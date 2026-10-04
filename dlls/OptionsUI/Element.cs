using System;
using System.Linq;
using System.Reflection;
using System.Reflection.Emit;
using _patcher.Constants;
using _patcher.Helpers;

namespace _patcher.OptionsUI
{
    internal class Element
    {
        internal object Value { get; set; }

        protected Element(object value)
        {
            Value = value;
        }

        private static readonly MethodBase BaseSetChildren = ILPatch.FindMethodBySignature(Patterns.Element_SetChildren);

        public static Array CreateArray(params Element[] elements)
        {
            Array array = Array.CreateInstance(
                elements.First().Value.GetType().BaseType,
                elements.Length);

            for (int i = 0; i < elements.Length; i++)
                array.SetValue(elements[i].Value, i);

            return array;
        }

        public void SetChildren(Array children)
            => BaseSetChildren.Invoke(Value, new object[] { children });
    }
}