using System;
using System.Collections.Generic;
using System.Reflection;
using System.Reflection.Emit;

namespace _patcher.Helpers
{
    internal sealed class ILReader
    {
        private static readonly Dictionary<(byte prefix, byte code), OpCode> Opcodes;

        private readonly byte[] _il;
        private int _pos;

        static ILReader()
        {
            Opcodes = new Dictionary<(byte prefix, byte code), OpCode>();
            foreach (FieldInfo field in typeof(OpCodes).GetFields(BindingFlags.Public | BindingFlags.Static))
            {
                var opcode = (OpCode)field.GetValue(null);
                if (opcode.OpCodeType == OpCodeType.Nternal)
                    continue;

                byte prefix = opcode.Size == 1 ? (byte)0 : (byte)0xFE;
                byte code = (byte)(opcode.Value & 0xFF);
                Opcodes[(prefix, code)] = opcode;
            }
        }

        public ILReader(byte[] ilBytes)
        {
            _il = ilBytes ?? throw new ArgumentNullException(nameof(ilBytes));
        }

        public IEnumerable<OpCode> GetOpCodes()
        {
            while (_pos < _il.Length)
            {
                byte current = _il[_pos++];
                byte prefix = current == 0xFE ? (byte)0xFE : (byte)0;
                byte code = prefix == 0xFE ? _il[_pos++] : current;

                OpCode op = Opcodes[(prefix, code)];

                switch (op.OperandType)
                {
                    case OperandType.InlineSwitch:
                        _pos += 1 + (_il[_pos]
                            | (_il[_pos + 1] << 8)
                            | (_il[_pos + 2] << 16)
                            | (_il[_pos + 3] << 24)) * 4;
                        break;
                    case OperandType.InlineI8:
                    case OperandType.InlineR:
                        _pos += 8;
                        break;
                    case OperandType.InlineNone:
                        break;
                    case OperandType.ShortInlineBrTarget:
                    case OperandType.ShortInlineI:
                    case OperandType.ShortInlineVar:
                        _pos += 1;
                        break;
                    case OperandType.InlineVar:
                        _pos += 2;
                        break;
                    case OperandType.InlineBrTarget:
                    case OperandType.InlineField:
                    case OperandType.InlineI:
                    case OperandType.InlineMethod:
                    case OperandType.InlineSig:
                    case OperandType.InlineString:
                    case OperandType.InlineTok:
                    case OperandType.InlineType:
                    case OperandType.ShortInlineR:
                        _pos += 4;
                        break;
                    default:
                        throw new NotSupportedException($"Используется неподдерживаемый тип операнда: {op.OperandType}");
                }

                yield return op;
            }
        }
    }
}