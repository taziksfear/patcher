using System;
using System.Reflection;
using System.Reflection.Emit;
using _patcher.Constants;
using _patcher.Helpers;

namespace _patcher.OptionsUI
{
    internal enum FontAwesome
    {
        Moon = 0xf186,
    }

    internal class Category : Element
    {
        private static readonly ConstructorInfo BaseCategoryConstructor = ILPatch.FindConstructorBySignature(Patterns.Category_Constructor);

        public Category(FontAwesome icon)
            : base(CreateCategoryInstance(icon))
        {
        }

        private static object CreateCategoryInstance(FontAwesome icon)
        {
            // GetParameters() не включает `this`: [0]=OsuString title, [1]=icon enum
            var parameters = BaseCategoryConstructor.GetParameters();
            object title = Enum.ToObject(parameters[0].ParameterType, OsuConstants.PatcherCategory);
            object osuIcon = Enum.ToObject(parameters[1].ParameterType, (int)icon);
            return BaseCategoryConstructor.Invoke(new object[] { title, osuIcon });
        }
    }
}