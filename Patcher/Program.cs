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
        private static readonly string tmpdir = Path.Combine(Path.GetTempPath(), "osu_patcher_" + Guid.NewGuid().ToString()[..8]);

        public static void Main(string[] args)
        {
            try
            {
                string curdir = AppDomain.CurrentDomain.BaseDirectory;
                string gamedir = Path.Combine(curdir, "real_osu");
                string game_exe = Path.Combine(gamedir, "osu!.exe");

                if (!File.Exists(game_exe))
                {
                    Console.WriteLine($"[!] Error: can't find {game_exe}");
                    Console.ReadLine();
                    return;
                }

                string srv_file = Path.Combine(curdir, "server.txt");
                string srv = "";

                if (File.Exists(srv_file))
                {
                    srv = File.ReadAllText(srv_file).Trim().ToLower();
                    File.Delete(srv_file);
                }

                bool is_bancho = string.IsNullOrEmpty(srv) || srv == "bancho";

                var new_args = new List<string>();
                for (int i = 0; i < args.Length; i++)
                {
                    if (args[i].Equals("-devserver", StringComparison.OrdinalIgnoreCase))
                    {
                        i++;
                        continue;
                    }
                    
                    if (args[i].Contains(" "))
                        new_args.Add($"\"{args[i]}\"");
                    else
                        new_args.Add(args[i]);
                }

                string final_args = string.Join(" ", new_args);
                
                if (!is_bancho)
                {
                    final_args += string.IsNullOrEmpty(final_args) ? $"-devserver {srv}" : $" -devserver {srv}";
                    Console.WriteLine($"[INFO] Server: {srv}");
                }
                else
                {
                    Console.WriteLine("[INFO] Bancho selected. Vanilla mode.");
                }

                var osu_proc = Process.Start(new ProcessStartInfo
                {
                    FileName = game_exe,
                    Arguments = final_args,
                    UseShellExecute = false,
                    WorkingDirectory = gamedir
                });

                if (osu_proc == null) throw new Exception("Failed to start osu!");
                
                if (is_bancho) return; 
                Directory.CreateDirectory(tmpdir);
                var harm_dll = Unpack("0Harmony.dll", tmpdir);
                var patch_dll = Unpack("_patcher.dll", tmpdir);

                for (int i = 0; i < 7; i++)
                {
                    Thread.Sleep(1000);
                    osu_proc.Refresh();
                    if (osu_proc.HasExited) return;
                }
                try
                {
                    using (var p = new InjectableProcess((uint)osu_proc.Id))
                    {
                        p.Inject(patch_dll, "_patcher.Main", "Initialize");
                    }
                    Console.WriteLine(">>> INJECTED! <<<");
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"[!] Inject error: {ex.Message}");
                }
                
                Thread.Sleep(3000);
            }
            catch (Exception e)
            {
                Console.WriteLine($"[!] Crash: {e.Message}");
                Console.ReadLine();
            }
            finally
            {
                CleanTmp();
            }
        }
        private static string Unpack(string resName, string outDir)
        {
            var outPath = Path.Combine(outDir, resName);
            var asm = Assembly.GetExecutingAssembly();
            var stream = asm.GetManifestResourceStream($"{asm.GetName().Name}.{resName}");
            
            if (stream == null)
            {
                var all_res = asm.GetManifestResourceNames();
                foreach (var r in all_res)
                {
                    if (r.EndsWith("." + resName) || r == resName)
                    {
                        stream = asm.GetManifestResourceStream(r);
                        break;
                    }
                }
                if (stream == null) throw new FileNotFoundException($"Resource {resName} missing.");
            }
            
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