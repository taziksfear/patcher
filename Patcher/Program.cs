using System;
using System.IO;
using System.Threading;
using System.Diagnostics;
using System.Reflection;
using HoLLy.ManagedInjector;

namespace patchershit
{
    internal class Program
    {
        private static readonly string ConfigPath = Path.GetFullPath("conf.db");
        private const string Domain = "akatsuki.gg";

        private static string GetCurrentDirectory()
        {
            return Path.GetDirectoryName(Assembly.GetExecutingAssembly().Location) ?? Directory.GetCurrentDirectory();
        }
        
        public static void Main(string[] args)
        {
            try
            {
                var osuPath = GetOsuPath();
                
                // path to dlls
                var currentDir = GetCurrentDirectory();
                var harmonyPath = Path.Combine(currentDir, "0Harmony.dll");
                var patcherPath = Path.Combine(currentDir, "_patcher.dll");

                Console.WriteLine($"Current directory: {currentDir}");
                Console.WriteLine($"Looking for Harmony: {harmonyPath}");
                Console.WriteLine($"Looking for Patcher: {patcherPath}");

                // is files exist fr?
                if (!File.Exists(harmonyPath))
                    throw new FileNotFoundException($"0Harmony.dll not found at: {harmonyPath}");
                
                if (!File.Exists(patcherPath))
                    throw new FileNotFoundException($"_patcher.dll not found at: {patcherPath}");

                Console.WriteLine("Found all required DLL files locally");
                
                // finally start osu!
                var osuProc = Process.Start(new ProcessStartInfo
                {
                    FileName = osuPath,
                    Arguments = $"-devserver {Domain}",
                    UseShellExecute = false
                });
                
                if (osuProc == null)
                    throw new Exception("failed to start osu!");
                
                Console.WriteLine($"osu! started with PID: {osuProc.Id}");
                
                osuProc.WaitForInputIdle();
                Thread.Sleep(5000);
                
                Console.WriteLine("Attempting injection...");
                
                // inject patcher
                using (var proc = new InjectableProcess((uint)osuProc.Id))
                    proc.Inject(patcherPath, "_patcher.Main", "Initialize");
                
                Console.WriteLine("Injection completed successfully!");
            }
            catch (Exception e)
            {
                Console.Error.WriteLine("Error: " + e.Message);
                Console.WriteLine("Press any key to exit...");
                Console.ReadKey();
            }
        }
        
        private static string GetOsuPath()
        {
            if (File.Exists(ConfigPath))
            {
                var savedPath = File.ReadAllText(ConfigPath).Trim();
                if (File.Exists(savedPath))
                    return savedPath;

                Console.WriteLine("saved osu! path not found, re-entering...");
            }

            Console.Write("enter path to osu! folder (ex: D:\\osu!): ");
            var inputPath = Console.ReadLine()?.Trim('"').Trim();

            if (string.IsNullOrWhiteSpace(inputPath))
                throw new FileNotFoundException("Path cannot be empty");

            // auto add \osu!.exe if not exist in user's path
            string fullPath;
            if (inputPath.EndsWith("osu!.exe", StringComparison.OrdinalIgnoreCase))
            {
                fullPath = inputPath;
            }
            else
            {
                // delete / if exist \osu!.exe
                fullPath = Path.Combine(inputPath.TrimEnd('\\', '/'), "osu!.exe");
            }

            if (!File.Exists(fullPath))
            {
                // try to find 
                var alternativePaths = new[]
                {
                    Path.Combine(inputPath, "osu!.exe"),
                    inputPath + "\\osu!.exe",
                    inputPath + "//osu!.exe"
                };

                foreach (var altPath in alternativePaths)
                {
                    if (File.Exists(altPath))
                    {
                        fullPath = altPath;
                        break;
                    }
                }
            }

            if (!File.Exists(fullPath))
                throw new FileNotFoundException("osu!.exe not found at: " + fullPath);

            Console.WriteLine($"Using osu! path: {fullPath}");
            File.WriteAllText(ConfigPath, fullPath);
            
            return fullPath;
        }
    }
}