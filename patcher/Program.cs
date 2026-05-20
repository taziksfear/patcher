using System;
using System.IO;
using System.Threading;
using System.Diagnostics;
using System.Reflection;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using HoLLy.ManagedInjector;

namespace osu_patcher
{
    internal class App
    {
        private static readonly string tmpdir = Path.Combine(Path.GetTempPath(), "osu_patcher_" + Guid.NewGuid().ToString().Substring(0, 8));
        private static string log_path = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "patcher_log.txt");

        public static void Main(string[] args)
        {
            File.WriteAllText(log_path, $"logs: {DateTime.Now}\n");

            try
            {
                Log($"[ENV] OS: {Environment.OSVersion} | runtime: {Environment.Version}");
                
                string curdir = AppDomain.CurrentDomain.BaseDirectory;
                string gamedir = Path.Combine(curdir, "real_osu");
                string game_exe = Path.Combine(gamedir, "osu!.exe");

                string srv_file = Path.Combine(curdir, "server.txt");
                string srv = "bancho";

                if (File.Exists(srv_file))
                {
                    srv = File.ReadAllText(srv_file).Trim().ToLower();
                    File.Delete(srv_file);
                    Log($"[SERVER] Resolved server: {srv}");
                }

                string target_exe = game_exe;
                string target_dir = gamedir;
                bool needs_inject = false;

                string custom_dir = Path.Combine(curdir, "clients", srv);
                string custom_exe = Path.Combine(custom_dir, "osu!.exe");

                bool is_custom_client = Directory.Exists(custom_dir) && File.Exists(custom_exe);
                bool is_bancho = string.IsNullOrEmpty(srv) || srv == "bancho";

                if (is_custom_client)
                {
                    Log($"[ROUTE] Found custom client for '{srv}'. Using: {custom_exe}");
                    target_exe = custom_exe;
                    target_dir = custom_dir;
                    needs_inject = false; 
                }
                else
                {
                    Log($"[ROUTE] No custom client for '{srv}'. Using real_osu.");
                    
                    if (!File.Exists(game_exe))
                    {
                        Log($"[!] Error: Cannot find {game_exe}");
                        Console.WriteLine($"[!] Error: can't find {game_exe}");
                        Console.ReadLine();
                        return;
                    }

                    if (!is_bancho)
                    {
                        needs_inject = true;
                        Log($"[ROUTE] Injection required for private server: {srv}");
                    }
                    else
                    {
                        Log($"[ROUTE] Bancho selected. Vanilla mode, no injection.");
                    }
                }

                var new_args = new List<string>();
                foreach (var arg in args)
                {
                    if (arg.Equals("-devserver", StringComparison.OrdinalIgnoreCase)) continue;
                    new_args.Add(arg.Contains(" ") ? $"\"{arg}\"" : arg);
                }

                string final_args = string.Join(" ", new_args);
                
                if (needs_inject)
                {
                    final_args += (string.IsNullOrEmpty(final_args) ? "" : " ") + $"-devserver {srv}";
                    Log($"[ARGS] Appended devserver arg: -devserver {srv}");
                }

                bool targetIs64 = IsExe64Bit(target_exe);
                bool useShell = !needs_inject && targetIs64;

                Log($"[PROC] Target architecture: {(targetIs64 ? "64-bit" : "32-bit")}");

                var psi = new ProcessStartInfo
                {
                    FileName = target_exe,
                    Arguments = final_args,
                    UseShellExecute = useShell,
                    WorkingDirectory = target_dir
                };

                Log($"[PROC] Starting {target_exe}...");
                var osu_proc = Process.Start(psi);

                if (osu_proc == null) throw new Exception("Failed to start osu!");

                if (!needs_inject)
                {
                    Log("[PROC] Running in standalone client mode. Waiting for game to exit...");
                    osu_proc.WaitForExit();
                    return;
                }

                Log($"[INJECT] Unpacking DLLs to {tmpdir}...");
                Directory.CreateDirectory(tmpdir);
                var harm_dll = Unpack("0Harmony.dll", tmpdir);
                var patch_dll = Unpack("_patcher.dll", tmpdir);

                Log("[INJECT] Waiting for process to initialize...");
                for (int i = 0; i < 7; i++)
                {
                    Thread.Sleep(1000);
                    osu_proc.Refresh();
                    if (osu_proc.HasExited)
                    {
                        Log("[INJECT] Process exited before injection could occur.");
                        return;
                    }
                }

                try
                {
                    Log($"[INJECT] Attempting injection into PID {osu_proc.Id}...");
                    using (var p = new InjectableProcess((uint)osu_proc.Id))
                    {
                        p.Inject(patch_dll, "_patcher.Main", "Initialize");
                    }
                    Log(">>> INJECTED SUCCESSFULLY! <<<");
                    Console.WriteLine(">>> INJECTED! <<<");
                }
                catch (Exception ex)
                {
                    Log($"[!] Inject error: {ex.Message}");
                    Console.WriteLine($"[!] Inject error: {ex.Message}");
                }
                
                osu_proc.WaitForExit();
            }
            catch (Exception e)
            {
                Log($"[FATAL] Crash: {e.Message}");
                Console.WriteLine($"[!] Crash: {e.Message}");
                Console.ReadLine();
            }
            finally
            {
                CleanTmp();
            }
        }

        [DllImport("kernel32.dll", SetLastError = true)]
        private static extern bool IsWow64Process(IntPtr hProcess, out bool wow64Process);

        private static bool IsProcess64Bit(Process proc)
        {
            if (!Environment.Is64BitOperatingSystem) return false;
            try
            {
                if (!IsWow64Process(proc.Handle, out bool isWow64)) return false;
                return !isWow64;
            }
            catch (Exception)
            {
                return false;
            }
        }

        private static bool IsExe64Bit(string exePath)
        {
            try
            {
                using (var fs = new FileStream(exePath, FileMode.Open, FileAccess.Read))
                using (var br = new BinaryReader(fs))
                {
                    fs.Seek(0x3C, SeekOrigin.Begin);
                    int peOffset = br.ReadInt32();
                    fs.Seek(peOffset + 4, SeekOrigin.Begin);
                    ushort machine = br.ReadUInt16();
                    return machine == 0x8664;
                }
            }
            catch (Exception ex)
            {
                Log($"[ROUTE] Could not read PE header of {exePath}: {ex.Message}");
                return false;
            }
        }

        private static void Log(string message)
        {
            try
            {
                File.AppendAllText(log_path, $"[{DateTime.Now:HH:mm:ss}] {message}\n");
            }
            catch { }
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
            if (stream == null) throw new FileNotFoundException($"Resource {resName} missing.");
            
            using (stream) 
            using (var fs = File.Create(outPath)) 
            {
                stream.CopyTo(fs);
            }
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