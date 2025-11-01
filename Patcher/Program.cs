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
        private static readonly string TempDir = Path.Combine(Path.GetTempPath(), "osu_patcher_" + Guid.NewGuid().ToString()[..8]);

        public static void Main(string[] args)
        {
            try
            {
                // Диагностика встроенных ресурсов
                var assembly = Assembly.GetExecutingAssembly();
                var resources = assembly.GetManifestResourceNames();
                Console.WriteLine("Available embedded resources:");
                foreach (var resource in resources)
                {
                    Console.WriteLine($"  - {resource}");
                }

                // Создаем временную директорию
                Directory.CreateDirectory(TempDir);
                
                var osuPath = GetOsuPath();
                
                // Извлекаем DLL файлы во временную директорию
                var harmonyPath = ExtractEmbeddedResource("0Harmony.dll", TempDir);
                var patcherPath = ExtractEmbeddedResource("_patcher.dll", TempDir);

                Console.WriteLine($"Extracted DLLs to temporary directory: {TempDir}");
                Console.WriteLine($"Harmony: {File.Exists(harmonyPath)} ({new FileInfo(harmonyPath).Length} bytes)");
                Console.WriteLine($"Patcher: {File.Exists(patcherPath)} ({new FileInfo(patcherPath).Length} bytes)");
                
                // Запускаем osu!
                Console.WriteLine($"Starting osu! from: {osuPath}");
                var osuProc = Process.Start(new ProcessStartInfo
                {
                    FileName = osuPath,
                    Arguments = $"-devserver {Domain}",
                    UseShellExecute = false
                });
                
                if (osuProc == null)
                    throw new Exception("failed to start osu!");
                
                Console.WriteLine($"osu! started with PID: {osuProc.Id}");
                
                // Даем процессу больше времени для инициализации
                Console.WriteLine("Waiting for process to initialize...");
                osuProc.WaitForInputIdle();
                
                // Ждем подольше для полной инициализации
                for (int i = 0; i < 10; i++)
                {
                    Console.WriteLine($"Waiting... {i + 1}/10 seconds");
                    Thread.Sleep(1000);
                    
                    // Проверяем, что процесс еще жив
                    if (osuProc.HasExited)
                    {
                        Console.WriteLine("osu! process exited prematurely!");
                        return;
                    }
                }
                
                Console.WriteLine("Attempting injection...");
                
                // Инжектим с обработкой ошибок
                try
                {
                    Console.WriteLine($"Creating InjectableProcess for PID {osuProc.Id}...");
                    using (var proc = new InjectableProcess((uint)osuProc.Id))
                    {
                        Console.WriteLine($"InjectableProcess created successfully");
                        Console.WriteLine($"Injecting {patcherPath}...");
                        Console.WriteLine($"Type: _patcher.Main");
                        Console.WriteLine($"Method: Initialize");
                        
                        proc.Inject(patcherPath, "_patcher.Main", "Initialize");
                    }
                    Console.WriteLine("Injection completed successfully!");
                }
                catch (Exception injectEx)
                {
                    Console.WriteLine($"Injection failed with error: {injectEx.GetType().Name}");
                    Console.WriteLine($"Message: {injectEx.Message}");
                    Console.WriteLine($"Stack trace: {injectEx.StackTrace}");
                    
                    // Дополнительная диагностика
                    if (injectEx.InnerException != null)
                    {
                        Console.WriteLine($"Inner exception: {injectEx.InnerException.GetType().Name}");
                        Console.WriteLine($"Inner message: {injectEx.InnerException.Message}");
                    }
                    
                    throw;
                }
                
                // Даем время для работы инжектированного кода
                Console.WriteLine("Waiting for injected code to execute...");
                Thread.Sleep(3000);
                
                Console.WriteLine("Process completed successfully!");
            }
            catch (Exception e)
            {
                Console.Error.WriteLine($"Critical error: {e.GetType().Name}");
                Console.Error.WriteLine($"Message: {e.Message}");
                Console.Error.WriteLine($"Stack trace: {e.StackTrace}");
                Console.WriteLine("Press any key to exit...");
                Console.ReadKey();
            }
            finally
            {
                // Очищаем временные файлы
                CleanupTempFiles();
            }
        }
        
        private static string ExtractEmbeddedResource(string resourceName, string outputDirectory)
        {
            var outputPath = Path.Combine(outputDirectory, resourceName);
            
            // Получаем assembly и ищем ресурс
            var assembly = Assembly.GetExecutingAssembly();
            var resourceStream = assembly.GetManifestResourceStream($"{assembly.GetName().Name}.{resourceName}");
            
            if (resourceStream == null)
            {
                // Пробуем найти ресурс без namespace
                var resources = assembly.GetManifestResourceNames();
                foreach (var res in resources)
                {
                    if (res.EndsWith("." + resourceName) || res == resourceName)
                    {
                        resourceStream = assembly.GetManifestResourceStream(res);
                        break;
                    }
                }
                
                if (resourceStream == null)
                    throw new FileNotFoundException($"Embedded resource {resourceName} not found. Available resources: {string.Join(", ", resources)}");
            }
            
            using (resourceStream)
            using (var fileStream = File.Create(outputPath))
            {
                resourceStream.CopyTo(fileStream);
            }
            
            return outputPath;
        }
        
        private static void CleanupTempFiles()
        {
            try
            {
                if (Directory.Exists(TempDir))
                {
                    // Даем время для освобождения DLL файлов
                    Thread.Sleep(1000);
                    Directory.Delete(TempDir, true);
                    Console.WriteLine($"Cleaned up temporary directory: {TempDir}");
                }
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Warning: Could not clean up temporary files: {ex.Message}");
            }
        }
        
        private static string GetOsuPath()
        {
            // Без изменений
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

            string fullPath;
            if (inputPath.EndsWith("osu!.exe", StringComparison.OrdinalIgnoreCase))
            {
                fullPath = inputPath;
            }
            else
            {
                fullPath = Path.Combine(inputPath.TrimEnd('\\', '/'), "osu!.exe");
            }

            if (!File.Exists(fullPath))
            {
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