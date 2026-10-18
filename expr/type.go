package expr

// ValueType は評価せずに判定する値の型。条件式の両枝はビットの和で表す。
type ValueType uint8

const (
	UnknownType ValueType = 1 << iota
	NumberType
	BoolType
	StringType
	NilType
	InvalidType
)

// TypeOfValue は暗黙変換前のスカラーの型を返す。
func TypeOfValue(v any) ValueType {
	switch v.(type) {
	case bool:
		return BoolType
	case string:
		return StringType
	case nil:
		return NilType
	}
	if _, ok := ToNumber(v); ok {
		return NumberType
	}
	return UnknownType
}

// ResultType は参照の型を resolve で引き、式を実行せず結果の候補型を返す。
// 乱数や副作用のある関数を検査のために呼ばない。
func (e *Expr) ResultType(resolve func(Ref) ValueType) ValueType {
	if e == nil {
		return NilType
	}
	var infer func(node) ValueType
	infer = func(n node) ValueType {
		if r, _, ok := chain(n); ok {
			return resolve(r)
		}
		switch x := n.(type) {
		case *lit:
			return TypeOfValue(x.v)
		case *not, *logic:
			return BoolType
		case *neg:
			return numericType(infer(x.x))
		case *cond:
			return infer(x.a) | infer(x.b)
		case *binop:
			switch x.op {
			case "==", "!=", "<", "<=", ">", ">=":
				return BoolType
			case "+":
				l, r := infer(x.l), infer(x.r)
				if l == StringType || r == StringType {
					return StringType
				}
				if (l|r)&UnknownType != 0 {
					return UnknownType
				}
				if (l|r)&StringType != 0 {
					return StringType | NumberType
				}
			}
			return numericType(infer(x.l) | infer(x.r))
		case *call:
			if id, ok := x.fn.(*ident); ok {
				switch id.name {
				case "str":
					return StringType
				case "has", "chance":
					return BoolType
				case "min", "max", "abs", "floor", "ceil", "round", "int", "sqrt", "pow", "clamp", "len", "rand", "pages":
					return NumberType
				}
			}
		}
		return UnknownType
	}
	return infer(e.root)
}

// numericType は実行時の数への変換（真偽と nil を含む）が可能かを調べる。
func numericType(t ValueType) ValueType {
	if t & ^(NumberType|BoolType|NilType|UnknownType) != 0 {
		return InvalidType
	}
	if t&UnknownType != 0 {
		return UnknownType
	}
	return NumberType
}
