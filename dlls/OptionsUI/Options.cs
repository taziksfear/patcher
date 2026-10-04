using System;
using System.Linq;
using System.Reflection;
using System.Reflection.Emit;
using _patcher.Constants;
using _patcher.Helpers;

namespace _patcher.OptionsUI
{
    internal static class Options
    {
        public static readonly Config Config = Config.Load();

        private static readonly OpCode[] AddElementPrefix = new[]
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
        };

        public static void InitializeOptions(object instance)
        {
            var patchRelax = new CheckBox(
                "Patch Relax/Autopilot",
                "Removes relax/autopilot limitation: miss counter, combo breaks and score saving work normally.",
                Config.PatchRelax,
                (sender, e) => Config.TogglePatchRelax());

            Array optionChildren = Element.CreateArray(patchRelax);
            var section = new Section("Patcher");
            section.SetChildren(optionChildren);

            Array sectionChildren = Element.CreateArray(section);
            var category = new Category(FontAwesome.Moon);
            category.SetChildren(sectionChildren);

            Add(instance, category);
        }

        private static void Add(object instance, Element element)
        {
            MethodInfo addElement = ILPatch.FindInstanceMethodBySignature(instance.GetType(), AddElementPrefix);
            if (addElement == null)
            {
                Logger.Log("AddElement not found on " + instance.GetType().FullName);
                return;
            }

            addElement.Invoke(instance, new object[] { element.Value });
        }
    }
}