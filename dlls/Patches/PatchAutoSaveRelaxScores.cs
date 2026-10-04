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
    internal class PatchAutoSaveRelaxScores
    {
        [HarmonyTargetMethod]
        private static MethodBase Target() => ILPatch.FindMethodBySignature(Patterns.PatchAutoSaveRelaxScores_Target);

        [HarmonyTranspiler]
        private static IEnumerable<CodeInstruction> Transpiler(IEnumerable<CodeInstruction> instructions)
        {
            var codes = new List<CodeInstruction>(instructions);

            codes.InsertRange(33, new[]
            {
                new CodeInstruction(OpCodes.Call, typeof(PatchAutoSaveRelaxScores)
                    .GetMethod(nameof(PatchRelax), BindingFlags.Public | BindingFlags.Static)),
                new CodeInstruction(OpCodes.And),
            });

            codes.InsertRange(21, new[]
            {
                new CodeInstruction(OpCodes.Call, typeof(PatchAutoSaveRelaxScores)
                    .GetMethod(nameof(PatchRelax), BindingFlags.Public | BindingFlags.Static)),
                new CodeInstruction(OpCodes.And),
            });

            return codes.AsEnumerable();
        }

        [MethodImpl(MethodImplOptions.AggressiveInlining)]
        public static bool PatchRelax() => !OptionsUI.Options.Config.PatchRelax;
    }
}