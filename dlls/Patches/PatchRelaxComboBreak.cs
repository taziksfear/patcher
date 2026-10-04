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
    internal class PatchRelaxComboBreak
    {
        [HarmonyTargetMethod]
        private static MethodBase Target() => ILPatch.FindMethodBySignature(Patterns.PatchRelaxComboBreak_Target);

        [HarmonyTranspiler]
        private static IEnumerable<CodeInstruction> Transpiler(IEnumerable<CodeInstruction> instructions)
        {
            var codes = new List<CodeInstruction>(instructions);
            codes.RemoveAt(1558);
            codes.InsertRange(1559, new CodeInstruction[]
            {
                new CodeInstruction(OpCodes.Or),
                new CodeInstruction(OpCodes.Call,
                    typeof(PatchRelaxComboBreak)
                        .GetMethod(nameof(PatchRelax), BindingFlags.Public | BindingFlags.Static)),
                new CodeInstruction(OpCodes.And),
            });

            return codes.AsEnumerable();
        }

        [MethodImpl(MethodImplOptions.AggressiveInlining)]
        public static bool PatchRelax() => !OptionsUI.Options.Config.PatchRelax;
    }
}