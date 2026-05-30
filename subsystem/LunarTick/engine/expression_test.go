package engine

import (
	"testing"
)

func TestParseSimpleString(t *testing.T) {
	val := EvalExpression("'hello world'", nil, nil)
	if val != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", val)
	}
}

func TestParseVariableRef(t *testing.T) {
	vs := NewVarStore()
	vs.Set("name", "LunarTick")

	val := EvalExpression("Welcome #name", vs, nil)
	if val != "Welcome LunarTick" {
		t.Fatalf("expected 'Welcome LunarTick', got '%s'", val)
	}
}

func TestParseUndefinedVariable(t *testing.T) {
	vs := NewVarStore()

	val := EvalExpression("Hello #undefined", vs, nil)
	if val != "Hello " {
		t.Fatalf("expected 'Hello ', got '%s'", val)
	}
}

func TestParsePointerExists(t *testing.T) {
	pr := NewPointerRegistry()
	pr.Define("test")

	val := EvalExpression("*test", nil, pr)
	if val != "true" {
		t.Fatalf("expected 'true', got '%s'", val)
	}
}

func TestParsePointerNotExists(t *testing.T) {
	pr := NewPointerRegistry()

	val := EvalExpression("*nonexistent", nil, pr)
	if val != "false" {
		t.Fatalf("expected 'false', got '%s'", val)
	}
}

func TestParseMixedExpression(t *testing.T) {
	vs := NewVarStore()
	vs.Set("a", "hello")
	vs.Set("b", "world")

	val := EvalExpression("#a + ' ' + #b", vs, nil)
	if val != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", val)
	}
}

func TestEvalConditionEquals(t *testing.T) {
	if !EvalCondition("'abc' = 'abc'", nil, nil) {
		t.Fatal("expected true for equal strings")
	}
	if EvalCondition("'abc' = 'xyz'", nil, nil) {
		t.Fatal("expected false for different strings")
	}
}

func TestEvalConditionNotEquals(t *testing.T) {
	if !EvalCondition("'abc' != 'xyz'", nil, nil) {
		t.Fatal("expected true for different strings")
	}
}

func TestEvalConditionComparison(t *testing.T) {
	if !EvalCondition("'b' > 'a'", nil, nil) {
		t.Fatal("expected 'b' > 'a'")
	}
	if !EvalCondition("'a' < 'b'", nil, nil) {
		t.Fatal("expected 'a' < 'b'")
	}
}

func TestEvalConditionLogicAnd(t *testing.T) {
	vs := NewVarStore()
	vs.Set("x", "1")
	vs.Set("y", "1")

	if !EvalCondition("#x = '1' & #y = '1'", vs, nil) {
		t.Fatal("expected true for AND of true conditions")
	}
}

func TestEvalConditionLogicOr(t *testing.T) {
	vs := NewVarStore()
	vs.Set("x", "0")

	if !EvalCondition("#x = '1' | #x = '0'", vs, nil) {
		t.Fatal("expected true for OR with one true")
	}
}

func TestEvalConditionNegation(t *testing.T) {
	if !EvalCondition("!'false'", nil, nil) {
		t.Fatal("expected true for NOT false")
	}
	if EvalCondition("!'true'", nil, nil) {
		t.Fatal("expected false for NOT true")
	}
}

func TestEvalConditionEmpty(t *testing.T) {
	if EvalCondition("", nil, nil) {
		t.Fatal("expected false for empty")
	}
	if EvalCondition("false", nil, nil) {
		t.Fatal("expected false for 'false' literal")
	}
	if !EvalCondition("true", nil, nil) {
		t.Fatal("expected true for 'true' literal")
	}
}

func TestEvalConditionNonEmpty(t *testing.T) {
	if !EvalCondition("anything", nil, nil) {
		t.Fatal("expected true for non-empty non-false")
	}
}

func TestEvalArithmetic(t *testing.T) {
	tests := []struct {
		expr     string
		expected float64
	}{
		{"1+2", 3},
		{"5-3", 2},
		{"4*3", 12},
		{"10/2", 5},
		{"7%3", 1},
		{"(2+3)*4", 20},
		{"int(3.7)", 3},
		{"10/0", 0},
	}

	for _, tt := range tests {
		result := evaluateArithmetic(tt.expr)
		if result != tt.expected {
			t.Errorf("evaluateArithmetic(%s) = %f, want %f", tt.expr, result, tt.expected)
		}
	}
}
