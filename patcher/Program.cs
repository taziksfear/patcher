using System;
using System.IO;
using System.Threading;
using System.Diagnostics;
using System.Reflection;
using System.Collections.Generic;
using HoLLy.ManagedInjector;

namespace osu_patcher
{
    internal class App
    {
        private static readonly string tmpdir =
            Path.Combine(Path.GetTempPath(), "osu_patcher_" + Guid.NewGuid().ToString().Substring(0, 8));

        // In single-file publish, AppDomain.BaseDirectory points at the extraction
        // temp dir, NOT the location of the exe on disk. ProcessPath is the only
        // reliable way to find the real folder. This was the core bug that broke
        // the previous patcher when run as a published single-file exe.
        private static readonly string exePath =
            Environment.ProcessPath ?? Process.GetCurrentProcess().MainModule!.FileName!;
        private static readonly string baseDir = Path.GetDirectoryName(exePath)!;

        private static string logPath = Path.Combine(baseDir, "patcher_log.txt");

        public static void Main(string[] args)
        {
            try { File.WriteAllText(logPath, $"logs: {DateTime.Now}\n"); } catch { }

            try
            {
                // --osu-dir / --server let the Go UI drive the patcher explicitly.
                // Without them we fall back to baseDir + server.txt, so the exe still
                // works when dropped standalone into an osu folder and double-clicked.
                var (cliArgs, osuDirOverride, srvOverride) = ParseCliOverrides(args);

                string resolvedOsuDir =
                    !string.IsNullOrEmpty(osuDirOverride) ? osuDirOverride : baseDir;
                logPath = Path.Combine(resolvedOsuDir, "patcher_log.txt");

                Log($"[ENV] OS: {Environment.OSVersion} | runtime: {Environment.Version}");
                Log($"[ENV] exePath: {exePath}");
                Log($"[ENV] baseDir: {baseDir} | osuDir: {resolvedOsuDir}");

                string srv = !string.IsNullOrEmpty(srvOverride)
                    ? srvOverride
                    : ReadFile(resolvedOsuDir, "server.txt", "bancho");
                srv = (srv ?? "").Trim().ToLower();

                Log($"[SELECTION] server='{srv}'");

                args = cliArgs;

                // ── routing ────────────────────────────────────────────────
                // The game is launched wherever it already lives; we never move or
                // rename it. bancho → plain start, private server → -devserver + inject.
                bool isBancho = string.IsNullOrEmpty(srv) || srv.Equals("bancho", StringComparison.OrdinalIgnoreCase);

                if (!TryResolveOsuExe(resolvedOsuDir, out string targetExe, out string targetDir))
                {
                    FailAndExit($"osu!.exe not found.\nSearched: {string.Join(", ", OsuExeCandidates(resolvedOsuDir))}");
                    return;
                }
                bool needsInject = !isBancho;
                Log(isBancho
                    ? $"[ROUTE] bancho → {targetExe} (no inject)"
                    : $"[ROUTE] '{srv}' → {targetExe} + inject + -devserver {srv}");

                // ── args ───────────────────────────────────────────────────
                var newArgs = new List<string>();
                foreach (var a in args)
                {
                    if (a.Equals("-devserver", StringComparison.OrdinalIgnoreCase)) continue;
                    newArgs.Add(a.Contains(" ") ? $"\"{a}\"" : a);
                }
                string finalArgs = string.Join(" ", newArgs);
                if (!isBancho)
                {
                    finalArgs += (finalArgs.Length > 0 ? " " : "") + $"-devserver {srv}";
                }
                Log($"[ARGS] {finalArgs}");

                // ── start ──────────────────────────────────────────────────
                // Injection needs a real process handle, so ShellExecute must be off
                // there. Without injection we let the shell start it.
                bool useShell = !needsInject;
                bool targetIs64 = IsExe64Bit(targetExe);
                Log($"[PROC] arch={(targetIs64 ? "64-bit" : "32-bit")} useShell={useShell}");

                var psi = new ProcessStartInfo
                {
                    FileName = targetExe,
                    Arguments = finalArgs,
                    UseShellExecute = useShell,
                    WorkingDirectory = targetDir,
                };

                Process osuProc;
                try { osuProc = Process.Start(psi); }
                catch (Exception ex)
                {
                    FailAndExit($"Process.Start threw: {ex.Message}\nexe={targetExe}");
                    return;
                }
                if (osuProc == null) { FailAndExit("Process.Start returned null"); return; }
                Log($"[PROC] pid {osuProc.Id} started");

                if (!needsInject)
                {
                    Log("[PROC] no injection needed");
                    return;
                }

                // ── inject ─────────────────────────────────────────────────
                Directory.CreateDirectory(tmpdir);
                Unpack("0Harmony.dll", tmpdir); // probed for next to _patcher.dll
                var patcherDll = Unpack("_patcher.dll", tmpdir);

                // 7s of 1s waits with HasExited checks. MainWindowHandle is unreliable
                // under wine; this matches the original osu_patcher cadence.
                Log("[INJECT] giving osu! 7s to initialise");
                for (int i = 0; i < 7; i++)
                {
                    Thread.Sleep(1000);
                    osuProc.Refresh();
                    if (osuProc.HasExited)
                    {
                        Log($"[INJECT] process exited early (code {osuProc.ExitCode})");
                        return;
                    }
                }

                try
                {
                    Log($"[INJECT] attempting inject into PID {osuProc.Id}");
                    using (var p = new InjectableProcess((uint)osuProc.Id))
                    {
                        p.Inject(patcherDll, "_patcher.Main", "Initialize");
                    }
                    Log(">>> INJECTED <<<");
                    Console.WriteLine(">>> INJECTED <<<");
                }
                catch (Exception ex)
                {
                    Log($"[INJECT] error: {ex}");
                    Console.WriteLine($"[!] inject error: {ex.Message}");
                }

                Thread.Sleep(3000);
            }
            catch (Exception e)
            {
                Log($"[FATAL] {e}");
                Console.WriteLine($"[!] crash: {e.Message}");
            }
            finally
            {
                CleanTmp();
            }
        }

        private static (string[] gameArgs, string osuDir, string server) ParseCliOverrides(string[] args)
        {
            var passthrough = new List<string>();
            string osuDir = null, server = null;
            for (int i = 0; i < args.Length; i++)
            {
                var a = args[i];
                if (a.Equals("--osu-dir", StringComparison.OrdinalIgnoreCase) && i + 1 < args.Length) { osuDir = args[++i]; continue; }
                if (a.Equals("--server",  StringComparison.OrdinalIgnoreCase) && i + 1 < args.Length) { server = args[++i]; continue; }
                passthrough.Add(a);
            }
            return (passthrough.ToArray(), osuDir, server);
        }

        private static string ReadFile(string dir, string name, string fallback)
        {
            string path = Path.Combine(dir, name);
            if (!File.Exists(path)) return fallback;
            try
            {
                var v = File.ReadAllText(path).Trim();
                return string.IsNullOrWhiteSpace(v) ? fallback : v;
            }
            catch { return fallback; }
        }

        // OsuExeCandidates yields the locations we'll check for the real osu!.exe.
        // real_osu/ comes first only because installs made by older versions of this
        // patcher still have the real game in there, with an impostor osu!.exe (an old
        // copy of this patcher) left at the root. Fresh installs have neither.
        private static IEnumerable<string> OsuExeCandidates(string root)
        {
            yield return Path.Combine(root, "real_osu", "osu!.exe");
            yield return Path.Combine(root, "osu!.exe");
        }

        private static bool TryResolveOsuExe(string root, out string exe, out string dir)
        {
            foreach (var c in OsuExeCandidates(root))
            {
                var full = Path.GetFullPath(c);
                if (File.Exists(full) && !PathsEqual(full, exePath))
                {
                    exe = full;
                    dir = Path.GetDirectoryName(full) ?? root;
                    return true;
                }
            }
            exe = ""; dir = "";
            return false;
        }

        private static bool PathsEqual(string a, string b)
        {
            try { return string.Equals(Path.GetFullPath(a), Path.GetFullPath(b), StringComparison.OrdinalIgnoreCase); }
            catch { return false; }
        }

        private static void FailAndExit(string message)
        {
            Log($"[FATAL] {message}");
            Console.WriteLine($"[!] {message}");
        }

        private static bool IsExe64Bit(string p)
        {
            try
            {
                using var fs = new FileStream(p, FileMode.Open, FileAccess.Read);
                using var br = new BinaryReader(fs);
                fs.Seek(0x3C, SeekOrigin.Begin);
                int peOffset = br.ReadInt32();
                fs.Seek(peOffset + 4, SeekOrigin.Begin);
                return br.ReadUInt16() == 0x8664;
            }
            catch { return false; }
        }

        private static void Log(string message)
        {
            try { File.AppendAllText(logPath, $"[{DateTime.Now:HH:mm:ss}] {message}\n"); } catch { }
        }

        private static string Unpack(string resName, string outDir)
        {
            var outPath = Path.Combine(outDir, resName);
            var asm = Assembly.GetExecutingAssembly();
            var stream = asm.GetManifestResourceStream($"{asm.GetName().Name}.{resName}");
            if (stream == null)
            {
                foreach (var r in asm.GetManifestResourceNames())
                {
                    if (r.EndsWith("." + resName) || r == resName)
                    {
                        stream = asm.GetManifestResourceStream(r);
                        break;
                    }
                }
            }
            if (stream == null) throw new FileNotFoundException($"resource {resName} missing");
            using (stream) using (var fs = File.Create(outPath)) { stream.CopyTo(fs); }
            return outPath;
        }

        private static void CleanTmp()
        {
            try
            {
                if (Directory.Exists(tmpdir))
                {
                    Thread.Sleep(1000);
                    Directory.Delete(tmpdir, true);
                }
            }
            catch { }
        }
    }
}
