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
                // --osu-dir / --server / --client let the Go UI drive the patcher
                // explicitly. Without them we fall back to baseDir + server.txt/client.txt
                // so the exe still works when dropped standalone into an osu folder.
                var (cliArgs, osuDirOverride, srvOverride, clientOverride) = ParseCliOverrides(args);

                string resolvedOsuDir =
                    !string.IsNullOrEmpty(osuDirOverride) ? osuDirOverride : baseDir;
                logPath = Path.Combine(resolvedOsuDir, "patcher_log.txt");

                Log($"[ENV] OS: {Environment.OSVersion} | runtime: {Environment.Version}");
                Log($"[ENV] exePath: {exePath}");
                Log($"[ENV] baseDir: {baseDir} | osuDir: {resolvedOsuDir}");

                string srv = !string.IsNullOrEmpty(srvOverride)
                    ? srvOverride
                    : ReadFile(resolvedOsuDir, "server.txt", "bancho");
                string client = !string.IsNullOrEmpty(clientOverride)
                    ? clientOverride
                    : ReadFile(resolvedOsuDir, "client.txt", "default");
                srv = (srv ?? "").Trim().ToLower();
                client = (client ?? "").Trim();

                Log($"[SELECTION] server='{srv}' client='{client}'");

                args = cliArgs;

                // ── routing ────────────────────────────────────────────────
                //   client = "default" → real_osu/osu!.exe   (+inject if server != bancho)
                //   client = "<name>"  → clients/<name>/osu!.exe (no inject; we just pass -devserver)
                //   server = "bancho"  → no -devserver, no inject
                bool isBancho = string.IsNullOrEmpty(srv) || srv.Equals("bancho", StringComparison.OrdinalIgnoreCase);
                bool isCustomClient = !string.IsNullOrEmpty(client) && !client.Equals("default", StringComparison.OrdinalIgnoreCase);

                // Implicit custom client: clients/<srv>/osu!.exe exists. Matches the
                // example/GO behaviour where the server name doubles as the client
                // folder when present. Only kicks in if the UI didn't pick a client.
                if (!isCustomClient && !isBancho)
                {
                    string implicitDir = Path.Combine(resolvedOsuDir, "clients", srv);
                    string implicitExe = Path.Combine(implicitDir, "osu!.exe");
                    if (Directory.Exists(implicitDir) && File.Exists(implicitExe))
                    {
                        client = srv;
                        isCustomClient = true;
                        Log($"[ROUTE] implicit custom client matched server name → {implicitExe}");
                    }
                }

                string targetExe;
                string targetDir;
                bool needsInject;

                if (isCustomClient)
                {
                    string customDir = Path.Combine(resolvedOsuDir, "clients", client);
                    string customExe = Path.Combine(customDir, "osu!.exe");
                    if (!File.Exists(customExe))
                    {
                        FailAndExit($"custom client '{client}' has no osu!.exe at:\n{customExe}");
                        return;
                    }
                    targetExe = customExe;
                    targetDir = customDir;
                    needsInject = false; // custom clients bake in their own networking
                    Log($"[ROUTE] custom client '{client}' → {customExe}");
                }
                else
                {
                    if (!TryResolveOsuExe(resolvedOsuDir, out targetExe, out targetDir))
                    {
                        FailAndExit($"osu!.exe not found.\nSearched: {string.Join(", ", OsuExeCandidates(resolvedOsuDir))}");
                        return;
                    }
                    needsInject = !isBancho;
                    Log(isBancho
                        ? $"[ROUTE] vanilla + bancho → {targetExe} (no inject)"
                        : $"[ROUTE] vanilla + '{srv}' → {targetExe} + inject + -devserver {srv}");
                }

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
                // For custom clients (often 64-bit) we use ShellExecute so the OS
                // handles bitness mismatches. For vanilla+inject we need a real
                // process handle, so ShellExecute must be off.
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

        private static (string[] gameArgs, string osuDir, string server, string client) ParseCliOverrides(string[] args)
        {
            var passthrough = new List<string>();
            string osuDir = null, server = null, client = null;
            for (int i = 0; i < args.Length; i++)
            {
                var a = args[i];
                if (a.Equals("--osu-dir", StringComparison.OrdinalIgnoreCase) && i + 1 < args.Length) { osuDir = args[++i]; continue; }
                if (a.Equals("--server",  StringComparison.OrdinalIgnoreCase) && i + 1 < args.Length) { server = args[++i]; continue; }
                if (a.Equals("--client",  StringComparison.OrdinalIgnoreCase) && i + 1 < args.Length) { client = args[++i]; continue; }
                passthrough.Add(a);
            }
            return (passthrough.ToArray(), osuDir, server, client);
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

        private static IEnumerable<string> OsuExeCandidates(string root)
        {
            yield return Path.Combine(root, "real_osu", "osu!.exe");
            yield return Path.Combine(root, "osu!.exe");
            yield return Path.Combine(root, "..", "osu!.exe");
        }

        private static bool TryResolveOsuExe(string root, out string exe, out string dir)
        {
            foreach (var c in OsuExeCandidates(root))
            {
                var full = Path.GetFullPath(c);
                // Avoid pointing at our own exe — happens when patcher is dropped as osu!.exe
                // at the osu root and real_osu/ hasn't been scaffolded yet.
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
