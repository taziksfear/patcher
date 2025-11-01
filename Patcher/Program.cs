using System;
using System.IO;
using System.Threading;
using System.Diagnostics;
using System.Reflection;
using System.Windows.Forms;
using System.Drawing;
using HoLLy.ManagedInjector;

namespace patchershit
{
    internal static class Program
    {
        private static readonly string ConfigPath = Path.GetFullPath("conf.db");
        private static readonly string DomainConfigPath = Path.GetFullPath("domain.conf");
        private const string DefaultDomain = "akatsuki.gg";
        private static readonly string TempDir = Path.Combine(Path.GetTempPath(), "osu_patcher_" + Guid.NewGuid().ToString()[..8]);

        [STAThread]
        static void Main()
        {
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);
            
            Application.Run(new MainForm());
        }

        public static void RunPatcher()
        {
            try
            {
                var domain = GetDomain();
                if (domain == null)
                {
                    ShowMessage("Domain selection cancelled.", "Cancelled");
                    return;
                }

                var osuPath = GetOsuPath();
                if (string.IsNullOrEmpty(osuPath))
                {
                    ShowMessage("Path selection cancelled.", "Cancelled");
                    return;
                }

                Directory.CreateDirectory(TempDir);

                ExtractEmbeddedResource("0Harmony.dll", TempDir);
                var patcherPath = ExtractEmbeddedResource("_patcher.dll", TempDir);

                var osuProc = Process.Start(new ProcessStartInfo
                {
                    FileName = osuPath,
                    Arguments = $"-devserver {domain}",
                    UseShellExecute = false
                });
                
                if (osuProc == null)
                    throw new Exception("Failed to start osu!");

                osuProc.WaitForInputIdle();
                Thread.Sleep(8000);

                using (var proc = new InjectableProcess((uint)osuProc.Id))
                    proc.Inject(patcherPath, "_patcher.Main", "Initialize");
                
                ShowMessage("Injection completed successfully!", "Success");
            }
            catch (Exception e)
            {
                ShowMessage($"Error: {e.Message}", "Error", MessageBoxIcon.Error);
            }
            finally
            {
                CleanupTempFiles();
            }
        }

        private static string? GetDomain()
        {
            if (File.Exists(DomainConfigPath))
            {
                var savedDomain = File.ReadAllText(DomainConfigPath).Trim();
                if (!string.IsNullOrEmpty(savedDomain))
                    return savedDomain;
            }

            using var form = new Form()
            {
                Text = "Server Domain",
                Size = new Size(350, 180),
                FormBorderStyle = FormBorderStyle.FixedDialog,
                StartPosition = FormStartPosition.CenterScreen,
                MaximizeBox = false,
                MinimizeBox = false
            };

            var label = new Label 
            { 
                Text = "Enter server domain:", 
                Location = new Point(20, 20), 
                Size = new Size(300, 20)
            };

            var textBox = new TextBox 
            { 
                Text = DefaultDomain,
                Location = new Point(20, 45), 
                Size = new Size(290, 20)
            };

            var okButton = new Button 
            { 
                Text = "OK", 
                Location = new Point(150, 80), 
                Size = new Size(75, 30),
                DialogResult = DialogResult.OK
            };

            var cancelButton = new Button 
            { 
                Text = "Cancel", 
                Location = new Point(235, 80), 
                Size = new Size(75, 30),
                DialogResult = DialogResult.Cancel
            };

            form.Controls.AddRange(new Control[] { label, textBox, okButton, cancelButton });
            form.AcceptButton = okButton;
            form.CancelButton = cancelButton;

            textBox.SelectAll();
            textBox.Focus();

            if (form.ShowDialog() == DialogResult.OK)
            {
                var domain = string.IsNullOrEmpty(textBox.Text.Trim()) ? DefaultDomain : textBox.Text.Trim();
                File.WriteAllText(DomainConfigPath, domain);
                return domain;
            }

            return null;
        }

        private static string? GetOsuPath()
        {
            if (File.Exists(ConfigPath))
            {
                var savedPath = File.ReadAllText(ConfigPath).Trim();
                if (File.Exists(savedPath))
                    return savedPath;
            }

            using var openFileDialog = new OpenFileDialog
            {
                Filter = "osu! executable (osu!.exe)|osu!.exe",
                Title = "Select osu!.exe",
                CheckFileExists = true,
                CheckPathExists = true,
                FileName = "osu!.exe"
            };

            return openFileDialog.ShowDialog() == DialogResult.OK ? openFileDialog.FileName : null;
        }
        
        private static string ExtractEmbeddedResource(string resourceName, string outputDirectory)
        {
            var outputPath = Path.Combine(outputDirectory, resourceName);
            var assembly = Assembly.GetExecutingAssembly();
            
            var resourceStream = assembly.GetManifestResourceStream($"{assembly.GetName().Name}.{resourceName}") 
                              ?? assembly.GetManifestResourceStream(resourceName);
            
            if (resourceStream == null)
                throw new FileNotFoundException($"Resource {resourceName} not found");
            
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
                    Thread.Sleep(1000);
                    Directory.Delete(TempDir, true);
                }
            }
            catch { /* ignored */ }
        }

        private static void ShowMessage(string message, string title, MessageBoxIcon icon = MessageBoxIcon.Information)
        {
            MessageBox.Show(message, title, MessageBoxButtons.OK, icon);
        }
    }

    public class MainForm : Form
    {
        private Button startButton;
        private Label statusLabel;

        public MainForm()
        {
            InitializeComponent();
        }

        private void InitializeComponent()
        {
            this.Text = "osu! Patcher";
            this.Size = new Size(400, 200);
            this.StartPosition = FormStartPosition.CenterScreen;
            this.FormBorderStyle = FormBorderStyle.FixedDialog;
            this.MaximizeBox = false;
            this.MinimizeBox = false;

            this.BackColor = Color.FromArgb(15, 15, 15);
            this.ForeColor = Color.White;

            var titleLabel = new Label
            {
                Text = "osu! Patcher",
                Font = new Font("Arial", 14, FontStyle.Bold),
                Location = new Point(20, 20),
                Size = new Size(350, 30),
                TextAlign = ContentAlignment.MiddleCenter,

                ForeColor = Color.White,
                BackColor = Color.Transparent
            };

            statusLabel = new Label
            {
                Text = "Click Start to begin patching",
                Location = new Point(20, 70),
                Size = new Size(350, 20),
                TextAlign = ContentAlignment.MiddleCenter,

                ForeColor = Color.White,
                BackColor = Color.Transparent
            };

            startButton = new Button
            {
                Text = "Start",
                Location = new Point(150, 110),
                Size = new Size(100, 30),
                Font = new Font("Arial", 10, FontStyle.Bold),

                BackColor = Color.Transparent,
                ForeColor = Color.White,
                FlatStyle = FlatStyle.Flat
            };
            startButton.Click += StartButton_Click;

            this.Controls.AddRange(new Control[] { titleLabel, statusLabel, startButton });
        }

        private void StartButton_Click(object? sender, EventArgs e)
        {
            startButton.Enabled = false;
            statusLabel.Text = "Starting...";

            var thread = new Thread(() =>
            {
                Program.RunPatcher();
                this.Invoke(new Action(() =>
                {
                    startButton.Enabled = true;
                    statusLabel.Text = "Process completed";
                }));
            });
            
            thread.SetApartmentState(ApartmentState.STA);
            thread.Start();
        }
    }
}