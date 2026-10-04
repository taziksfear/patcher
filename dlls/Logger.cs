using System;
using System.IO;

namespace _patcher
{
    internal static class Logger
    {
        private static readonly string LogPath = Path.Combine(
            Path.GetTempPath(),
            "osu_patcher_runtime.log");

        public static void Log(string message)
        {
            try
            {
                File.AppendAllText(LogPath, $"[{DateTime.Now:HH:mm:ss}] {message}\n");
            }
            catch { }
        }

        public static void Log(Exception ex)
        {
            Log(ex?.ToString() ?? "null exception");
        }
    }
}