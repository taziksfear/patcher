using System;
using System.Reflection;
using System.Reflection.Emit;
using _patcher.Constants;
using _patcher.Helpers;

namespace _patcher.OptionsUI
{
    internal class CheckBox : Element
    {
        private static ConstructorInfo _boolConstructor;

        private static readonly ConstructorInfo BaseCheckBox = ILPatch.FindConstructorBySignature(Patterns.CheckBox_Constructor);

        public CheckBox(string title, string tooltip, bool initial, EventHandler onChanged)
            : base(CreateCheckBoxInstance(title, tooltip, initial, onChanged))
        {
        }

        private static object CreateCheckBoxInstance(string title, string tooltip, bool initial, EventHandler onChanged)
        {
            if (_boolConstructor == null)
            {
                _boolConstructor = BaseCheckBox
                    .GetParameters()[2]
                    .ParameterType
                    .GetConstructor(new[] { typeof(bool) });
            }

            object bindableBool = _boolConstructor.Invoke(new object[] { initial });

            return BaseCheckBox.Invoke(new object[]
            {
                title,
                tooltip,
                bindableBool,
                onChanged,
            });
        }
    }
}