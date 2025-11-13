using System;
using System.IO;
using System.Threading;
using System.Diagnostics;
using System.Reflection;
using System.Windows.Forms;
using System.Drawing;
using HoLLy.ManagedInjector;

namespace SimplePatch
{
    internal static class App
    {
        public static string cfg = Path.GetFullPath("path.txt");
        public static string tmp = Path.Combine(Path.GetTempPath(), "patch" + Guid.NewGuid().ToString()[..5]);

        [STAThread]
                static void Main()
        {
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);
            Application.Run(new Gui());
        }

        public static void work()
        {
            string dom = ask();
            if (dom == null) return;

            string p = find();
            if (string.IsNullOrEmpty(p)) return;

            Directory.CreateDirectory(tmp);

            save("0Harmony.dll", tmp);
            string dll = save("_patcher.dll", tmp);

            var proc = Process.Start(new ProcessStartInfo
            {
                FileName = p,
                Arguments = $"-devserver {dom}",
                UseShellExecute = false
            });
            
            proc.WaitForInputIdle();
            Thread.Sleep(8000);

            using (var i = new InjectableProcess((uint)proc.Id))
            {
                i.Inject(dll, "_patcher.Main", "Initialize");
            }
            
            MessageBox.Show("Готово!", "Инфо", MessageBoxButtons.OK, MessageBoxIcon.Information);

            clear();
        }

        public static string? ask()
        {
            using var f = new Form()
            {
                Text = "Сервер",
                Size = new Size(300, 150),
                FormBorderStyle = FormBorderStyle.FixedDialog,
                StartPosition = FormStartPosition.CenterScreen,
                MaximizeBox = false,
                MinimizeBox = false,
                BackColor = Color.FromArgb(30, 30, 30),
                ForeColor = Color.White
            };

            var l = new Label 
            { 
                Text = "Адрес сервера:", 
                Location = new Point(10, 10), 
                Size = new Size(260, 20),
                ForeColor = Color.White
            };

            var t = new TextBox 
            { 
                Text = "akatsuki.gg",
                Location = new Point(10, 35), 
                Size = new Size(260, 20),
                BackColor = Color.FromArgb(50, 50, 50),
                ForeColor = Color.White,
                BorderStyle = BorderStyle.FixedSingle
            };

            var b1 = new Button 
            { 
                Text = "Ок", 
                Location = new Point(110, 70), 
                                Size = new Size(70, 25),
                DialogResult = DialogResult.OK,
                BackColor = Color.FromArgb(60, 60, 60),
                ForeColor = Color.White,
                FlatStyle = FlatStyle.Flat
            };
            b1.FlatAppearance.BorderSize = 0;

            var b2 = new Button 
            { 
                Text = "Отмена", 
                Location = new Point(190, 70), 
                Size = new Size(70, 25),
                DialogResult = DialogResult.Cancel,
                BackColor = Color.FromArgb(60, 60, 60),
                ForeColor = Color.White,
                FlatStyle = FlatStyle.Flat
            };
            b2.FlatAppearance.BorderSize = 0;

            f.Controls.AddRange(new Control[] { l, t, b1, b2 });
            f.AcceptButton = b1;
            f.CancelButton = b2;

            if (f.ShowDialog() == DialogResult.OK)
            {
                return string.IsNullOrEmpty(t.Text) ? "akatsuki.gg" : t.Text;
            }

            return null;
        }

        public static string? find()
        {
                        if (File.Exists(cfg))
            {
                string s = File.ReadAllText(cfg);
                if (File.Exists(s)) return s;
            }

            using var d = new OpenFileDialog
            {
                Filter = "Exe файлы|*.exe",
                Title = "Где osu!.exe?",
                FileName = "osu!.exe"
            };

            if (d.ShowDialog() == DialogResult.OK)
            {
                File.WriteAllText(cfg, d.FileName);
                return d.FileName;
            }

            return null;
        }
        
        public static string save(string name, string dir)
        {
            string outpath = Path.Combine(dir, name);
            var asm = Assembly.GetExecutingAssembly();
            
            var stream = asm.GetManifestResourceStream($"{asm.GetName().Name}.{name}") 
                       ?? asm.GetManifestResourceStream(name);
            
            using (stream)
            using (var fs = File.Create(outpath))
            {
                stream.CopyTo(fs);
            }
            
            return outpath;
        }
        
        public static void clear()
        {
                        if (Directory.Exists(tmp))
            {
                Thread.Sleep(1000);
                Directory.Delete(tmp, true);
            }
        }
    }

    public class Gui : Form
    {
        Button btn;
        Label lbl;

        public Gui()
        {
            init();
        }

        void init()
        {
            Text = "патчер";
            Size = new Size(350, 180);
            StartPosition = FormStartPosition.CenterScreen;
            FormBorderStyle = FormBorderStyle.FixedDialog;
            MaximizeBox = false;
            MinimizeBox = false;
            BackColor = Color.FromArgb(30, 30, 30);
            ForeColor = Color.White;

            var l1 = new Label
            {
                Text = "патчер",
                Font = new Font("Arial", 16, FontStyle.Bold),
                Location = new Point(0, 20),
                Size = new Size(350, 30),
                TextAlign = ContentAlignment.MiddleCenter,
                ForeColor = Color.White
            };

            lbl = new Label
            {
                Text = "Нажми кнопку",
                Location = new Point(0, 60),
                Size = new Size(350, 20),
                TextAlign = ContentAlignment.MiddleCenter,
                ForeColor = Color.LightGray
            };

            btn = new Button
            {
                Text = "начать",
                Location = new Point(125, 90),
                Size = new Size(100, 30),
                BackColor = Color.FromArgb(60, 60, 60),
                ForeColor = Color.White,
                FlatStyle = FlatStyle.Flat
            };
            btn.FlatAppearance.BorderSize = 0;
            btn.Click += click;

            Controls.Add(l1);
            Controls.Add(lbl);
            Controls.Add(btn);
        }

        void click(object? s, EventArgs e)
        {
            btn.Enabled = false;
            lbl.Text = "запуск...";
            
            var t = new Thread(() =>
            {
                App.work();
                Invoke(new Action(() =>
                {
                    btn.Enabled = true;
                    lbl.Text = "запущено";
                }));
            });
            t.SetApartmentState(ApartmentState.STA);
            t.Start();
        }
    }
}
