using System;
using System.Linq;
using HarmonyLib;
using _patcher.Helpers;

namespace _patcher
{
    public class Main
    {
        private static readonly Harmony Harmony = new Harmony("osu_patcher.ano");

        public static int Initialize(string st)
        {
            try
            {
                Logger.Log(">>> osu! patcher Initialize <<<");
                Logger.Log("server arg: " + (st ?? "null"));

                try
                {
                    string currentDir = System.IO.Path.GetDirectoryName(
                        typeof(Main).Assembly.Location);
                    Logger.Log("assembly dir: " + (currentDir ?? "null"));
                    Logger.Log("loaded assemblies: " +
                        string.Join(", ", AppDomain.CurrentDomain.GetAssemblies().Select(a => a.GetName().Name)));
                }
                catch { }

                Harmony.PatchAll(typeof(Main).Assembly);
                Logger.Log("PatchAll done");
            }
            catch (Exception e)
            {
                Logger.Log(e);
            }

            return 0;
        }
    }
}