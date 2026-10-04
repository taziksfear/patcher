using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using System.Reflection.Emit;
using System.Runtime.CompilerServices;
using HarmonyLib;
using _patcher.Constants;
using _patcher.Helpers;

namespace _patcher.Patches
{
    [HarmonyPatch]
    internal class PatchRelaxMiss
    {
        [HarmonyTargetMethod]
        private static MethodBase Target() => ILPatch.FindMethodBySignature(Patterns.PatchRelaxMiss_Target);

        [HarmonyTranspiler]
        private static IEnumerable<CodeInstruction> Transpiler(IEnumerable<CodeInstruction> instructions)
        {
            var codes = new List<CodeInstruction>(instructions);
            codes.RemoveAt(668);
            codes.Insert(668, new CodeInstruction(
                OpCodes.Call,
                typeof(PatchRelaxMiss)
                    .GetMethod(nameof(PatchRelax), BindingFlags.Public | BindingFlags.Static)
            ));
            codes.RemoveRange(670, 2);

            return codes.AsEnumerable();
        }

        [MethodImpl(MethodImplOptions.AggressiveInlining)]
        public static bool PatchRelax() => !OptionsUI.Options.Config.PatchRelax;
    }
}