using System.Reflection.Emit;

namespace _patcher.Constants
{
    internal static class Patterns
    {
        // OptionsMenu.AddElement
        public static readonly OpCode[] Options_AddElement = new[]
        {
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Ldarg_1,
            OpCodes.Callvirt,
            OpCodes.Ldarg_1,
            OpCodes.Callvirt,
            OpCodes.Brfalse_S,
            OpCodes.Ldarg_1,
            OpCodes.Callvirt,
            OpCodes.Callvirt,
            OpCodes.Stloc_0,
            OpCodes.Br_S,
            OpCodes.Ldloc_0,
            OpCodes.Callvirt,
            OpCodes.Stloc_1,
            OpCodes.Ldloc_1,
            OpCodes.Ldarg_0,
            OpCodes.Ldftn,
            OpCodes.Newobj,
            OpCodes.Callvirt,
            OpCodes.Ldarg_0,
            OpCodes.Ldloc_1,
            OpCodes.Call,
            OpCodes.Ldloc_0,
            OpCodes.Callvirt,
            OpCodes.Brtrue_S,
            OpCodes.Leave_S,
            OpCodes.Ldloc_0,
            OpCodes.Brfalse_S,
            OpCodes.Ldloc_0,
            OpCodes.Callvirt,
            OpCodes.Endfinally,
            OpCodes.Ldarg_1,
            OpCodes.Isinst,
            OpCodes.Stloc_2,
            OpCodes.Ldloc_2,
            OpCodes.Brfalse_S,
            OpCodes.Ldloc_2,
            OpCodes.Ldarg_0,
            OpCodes.Ldftn,
            OpCodes.Newobj,
            OpCodes.Newobj,
            OpCodes.Stloc_3,
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Ldloc_3,
            OpCodes.Callvirt,
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Ldloc_3,
            OpCodes.Ldfld,
            OpCodes.Callvirt,
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Ldarg_1,
            OpCodes.Ldfld,
            OpCodes.Callvirt,
            OpCodes.Ret,
        };

        // OptionsMenu ctor (PatchOptionsMenu.Target)
        public static readonly OpCode[] PatchOptionsMenu_Target = new[]
        {
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Ldc_I4_S,
            OpCodes.Call,
            OpCodes.Callvirt,
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Ldc_I4_S,
            OpCodes.Call,
            OpCodes.Callvirt,
        };

        // Category ctor
        public static readonly OpCode[] Category_Constructor = new[]
        {
            OpCodes.Ldarg_0,
            OpCodes.Ldarga_S,
            OpCodes.Constrained,
            OpCodes.Callvirt,
            OpCodes.Call,
            OpCodes.Ldarg_0,
            OpCodes.Ldarga_S,
            OpCodes.Constrained,
            OpCodes.Callvirt,
            OpCodes.Call
        };

        // Section ctor
        public static readonly OpCode[] Section_Constructor = new[]
        {
            OpCodes.Ldarg_0,
            OpCodes.Ldarg_1,
            OpCodes.Callvirt,
            OpCodes.Ldc_R4,
            OpCodes.Ldc_R4,
            OpCodes.Ldc_R4,
            OpCodes.Newobj,
            OpCodes.Ldc_R4,
            OpCodes.Ldc_I4_1,
        };

        // Element.SetChildren
        public static readonly OpCode[] Element_SetChildren = new[]
        {
            OpCodes.Ldarg_0,
            OpCodes.Ldarg_1,
            OpCodes.Stfld,
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Brfalse_S,
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Callvirt,
            OpCodes.Stloc_0,
            OpCodes.Br_S,
            OpCodes.Ldloc_0,
            OpCodes.Callvirt,
            OpCodes.Stloc_1,
            OpCodes.Ldloc_1,
            OpCodes.Ldarg_0,
            OpCodes.Stfld,
            OpCodes.Ldarg_0,
            OpCodes.Call,
            OpCodes.Brfalse_S,
            OpCodes.Ldloc_1,
            OpCodes.Ldc_I4_1,
            OpCodes.Callvirt,
            OpCodes.Ldloc_0,
            OpCodes.Callvirt,
            OpCodes.Brtrue_S,
            OpCodes.Leave_S,
            OpCodes.Ldloc_0,
            OpCodes.Brfalse_S,
            OpCodes.Ldloc_0,
            OpCodes.Callvirt,
            OpCodes.Endfinally,
        };

        // CheckBox ctor
        public static readonly OpCode[] CheckBox_Constructor = new[]
        {
            OpCodes.Ldarg_0,
            OpCodes.Ldarg_3,
            OpCodes.Stfld,
            OpCodes.Ldarg_3,
            OpCodes.Brfalse_S,
            OpCodes.Ldarg_3,
            OpCodes.Ldarg_0,
            OpCodes.Ldftn,
            OpCodes.Newobj
        };

        // HitObjectManager.Hit → relax miss
        public static readonly OpCode[] PatchRelaxMiss_Target = new[]
        {
            OpCodes.Ldarg_1,
            OpCodes.Ldfld,
            OpCodes.Brfalse_S,
            OpCodes.Ldc_I4_0,
            OpCodes.Ret,
            OpCodes.Ldarg_0,
            OpCodes.Ldarg_1,
            OpCodes.Stfld,
            OpCodes.Ldarg_1,
            OpCodes.Callvirt,
            OpCodes.Stloc_0,
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Ldarg_1,
            OpCodes.Callvirt,
            OpCodes.Stloc_S,
            OpCodes.Ldarg_0,
            OpCodes.Ldloc_S,
            OpCodes.Ldc_I4_0,
            OpCodes.Blt_S,
            OpCodes.Ldloc_S,
            OpCodes.Br_S,
            OpCodes.Ldloc_S,
            OpCodes.Not
        };

        // Ranking.loadLocalUserScore → auto-save relax scores
        public static readonly OpCode[] PatchAutoSaveRelaxScores_Target = new[]
        {
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Ldfld,
            OpCodes.Brfalse,
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Brtrue,
            OpCodes.Ldsfld,
            OpCodes.Ldfld,
            OpCodes.Call,
            OpCodes.Ldc_I4,
            OpCodes.Stloc_2,
            OpCodes.Stloc_1,
            OpCodes.Ldloc_1,
            OpCodes.Ldloc_2,
            OpCodes.And,
            OpCodes.Ldc_I4_0,
            OpCodes.Cgt,
            OpCodes.Brtrue,
            OpCodes.Ldsfld,
            OpCodes.Ldfld,
            OpCodes.Call,
            OpCodes.Ldc_I4,
            OpCodes.Stloc_2,
            OpCodes.Stloc_1,
            OpCodes.Ldloc_1,
            OpCodes.Ldloc_2,
            OpCodes.And,
            OpCodes.Ldc_I4_0,
            OpCodes.Cgt,
            OpCodes.Brtrue
        };

        // Ruleset.IncreaseScoreHit → combo break
        public static readonly OpCode[] PatchRelaxComboBreak_Target = new[]
        {
            OpCodes.Ldarg_0,
            OpCodes.Ldfld,
            OpCodes.Callvirt,
            OpCodes.Ldc_I4_S,
            OpCodes.Ble_S,
            OpCodes.Ldsfld,
            OpCodes.Brtrue_S,
            OpCodes.Ldsfld,
            OpCodes.Brtrue_S
        };

        // NotificationManager.ShowMessageMassive
        public static readonly OpCode[] NotificationManager_ShowMessageMassive = new[]
        {
            OpCodes.Ldarg_1,
            OpCodes.Stfld,
            OpCodes.Ldloc_0,
            OpCodes.Ldarg_2,
            OpCodes.Stfld,
            OpCodes.Ldloc_0,
            OpCodes.Ldfld,
            OpCodes.Brtrue_S,
            OpCodes.Ret,
        };
    }
}