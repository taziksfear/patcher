using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Text;

namespace _patcher
{
    internal sealed class Config
    {
        private const string DirName = "osuPatcher";
        private const string FileName = "config.ini";

        private readonly Dictionary<string, string> _values =
            new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);

        private readonly List<string> _keys = new List<string>();

        private Config(string filePath)
        {
            FilePath = filePath;
        }

        public bool PatchRelax { get; private set; } = true;

        public string FilePath { get; }

        private static string DefaultPath
        {
            get
            {
                string localAppData = Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);
                return string.IsNullOrWhiteSpace(localAppData)
                    ? Path.GetFullPath(FileName)
                    : Path.Combine(localAppData, DirName, FileName);
            }
        }

        public static Config Load()
        {
            var config = new Config(DefaultPath);
            if (!File.Exists(config.FilePath))
                return config;

            foreach (string line in File.ReadAllLines(config.FilePath))
            {
                int sep = line.IndexOf('=');
                if (sep <= 0)
                    continue;

                string key = line.Substring(0, sep).Trim();
                string value = line.Substring(sep + 1).Trim();
                if (key.Length == 0)
                    continue;

                if (!config._values.ContainsKey(key))
                    config._keys.Add(key);
                config._values[key] = value;
            }

            config.PatchRelax = config.ReadBool("PatchRelax", true);
            return config;
        }

        public void TogglePatchRelax()
        {
            PatchRelax = !PatchRelax;
            Write("PatchRelax", PatchRelax);
        }

        public bool ReadBool(string key, bool defaultValue)
        {
            return _values.TryGetValue(key, out string value) && bool.TryParse(value, out bool parsed)
                ? parsed
                : defaultValue;
        }

        private void Write(string key, object value)
        {
            if (!_values.ContainsKey(key))
                _keys.Add(key);

            _values[key] = value is IFormattable formattable
                ? formattable.ToString(null, CultureInfo.InvariantCulture)
                : value?.ToString() ?? string.Empty;

            try
            {
                string dir = Path.GetDirectoryName(FilePath);
                if (!string.IsNullOrEmpty(dir))
                    Directory.CreateDirectory(dir);

                using (var writer = new StreamWriter(FilePath, false, new UTF8Encoding(false)))
                {
                    foreach (string k in _keys)
                        writer.WriteLine($"{k}={_values[k]}");
                }
            }
            catch { }
        }
    }
}