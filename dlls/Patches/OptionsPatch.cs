using System;
using System.Reflection;
using HarmonyLib;
using _patcher.Constants;
using _patcher.Helpers;
using _patcher.OptionsUI;

namespace _patcher.Patches
{
    [HarmonyPatch]
    internal class OptionsPatch
    {
        [HarmonyTargetMethod]
        private static MethodBase Target() => ILPatch.FindMethodBySignature(Patterns.PatchOptionsMenu_Target);

        [HarmonyPostfix]
        private static void Postfix(object __instance)
        {
            try
            {
                Options.InitializeOptions(__instance);
                Logger.Log("options menu patched");
            }
            catch (Exception ex)
            {
                Logger.Log(ex);
            }
        }
    }
}