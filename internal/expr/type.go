package expr

// ValueType is a value type determined without evaluation. Both branches of a conditional are represented as a bitwise union.
type ValueType uint8

const (
	UnknownType ValueType = 1 << iota
	NumberType
	BoolType
	StringType
	NilType
	InvalidType
)

// TypeOfValue returns the scalar type before implicit conversion.
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

// ResultType looks up the types of references via resolve and returns the candidate result types without executing the expression.
// It never calls random or side-effecting functions for checking purposes.
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

// numericType reports whether a value can be converted to a number at run time (including booleans and nil).
func numericType(t ValueType) ValueType {
	if t & ^(NumberType|BoolType|NilType|UnknownType) != 0 {
		return InvalidType
	}
	if t&UnknownType != 0 {
		return UnknownType
	}
	return NumberType
}
