using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using System.Reflection.Emit;

namespace _patcher.Helpers
{
    internal static class ILPatch
    {
        // Модуль игры osu!, который мы патчим (загружен в процесс).
        private static readonly Module OsuModule;

        static ILPatch()
        {
            try
            {
                Assembly osuAssembly = AppDomain.CurrentDomain
                    .GetAssemblies()
                    .FirstOrDefault(a => a.GetName().Name == "osu!");
                OsuModule = osuAssembly?.GetModules().FirstOrDefault();
            }
            catch
            {
                OsuModule = null;
            }
        }

        /// <summary>Ищет метод, содержащий заданную последовательность опкодов.</summary>
        public static MethodInfo FindMethodBySignature(OpCode[] signature)
        {
            if (signature == null || signature.Length == 0 || OsuModule == null)
                return null;

            return OsuModule.GetTypes()
                .SelectMany(t => t.GetRuntimeMethods())
                .FirstOrDefault(m => MatchesSignature(m, signature));
        }

        /// <summary>
        /// Ищет инстанс-метод на конкретном типе, чей IL начинается с заданной
        /// сигнатуры. Надёжнее паттерна по модулю целиком (который может дать
        /// ложные срабатывания на этом клиенте).
        /// </summary>
        public static MethodInfo FindInstanceMethodBySignature(Type declaringType, OpCode[] signaturePrefix)
        {
            if (declaringType == null || signaturePrefix == null || signaturePrefix.Length == 0)
                return null;

            return declaringType.GetRuntimeMethods()
                .FirstOrDefault(m => !m.IsStatic && StartsWithSignature(m, signaturePrefix));
        }

        private static bool StartsWithSignature(MethodBase method, OpCode[] signature)
        {
            var body = method.GetMethodBody()?.GetILAsByteArray();
            if (body == null)
                return false;

            int index = 0;
            foreach (OpCode opcode in new ILReader(body).GetOpCodes())
            {
                if (opcode == signature[index])
                    index++;
                else
                    return false;

                if (index == signature.Length)
                    return true;
            }

            return false;
        }

        /// <summary>Ищет конструктор, содержащий заданную последовательность опкодов.</summary>
        public static ConstructorInfo FindConstructorBySignature(OpCode[] signature)
        {
            if (signature == null || signature.Length == 0 || OsuModule == null)
                return null;

            return OsuModule.GetTypes()
                .SelectMany(t => t.GetConstructors(BindingFlags.Instance | BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic))
                .FirstOrDefault(c => MatchesSignature(c, signature));
        }

        private static bool MatchesSignature(MethodBase method, OpCode[] signature)
        {
            if (method == null || signature == null || signature.Length == 0)
                return false;

            var body = method.GetMethodBody()?.GetILAsByteArray();
            if (body == null)
                return false;

            int index = 0;
            foreach (OpCode opcode in new ILReader(body).GetOpCodes())
            {
                if (opcode == signature[index])
                    index++;
                else
                    index = opcode == signature[0] ? 1 : 0;

                if (index == signature.Length)
                    return true;
            }

            return false;
        }
    }
}