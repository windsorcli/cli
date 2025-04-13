package blueprint

import (
	"testing"
)

func TestRealJsonnetVM_TLACode(t *testing.T) {
	vm := NewJsonnetVM()

	// Set up TLA code
	vm.TLACode("testKey", "'42'") // String needs to be quoted in Jsonnet

	// Test snippet that uses the TLA code
	snippet := `function(testKey) testKey`
	result, err := vm.EvaluateAnonymousSnippet("test.jsonnet", snippet)

	if err != nil {
		t.Errorf("Failed to evaluate snippet: %v", err)
	}

	expected := "\"42\"\n"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestRealJsonnetVM_ExtCode(t *testing.T) {
	vm := NewJsonnetVM()

	// Set up external code
	vm.ExtCode("config", `{value: 123}`)

	// Test snippet that uses the external code
	snippet := `std.extVar('config')`
	result, err := vm.EvaluateAnonymousSnippet("test.jsonnet", snippet)

	if err != nil {
		t.Errorf("Failed to evaluate snippet: %v", err)
	}

	expected := `{
   "value": 123
}
`
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestRealJsonnetVM_EvaluateAnonymousSnippet(t *testing.T) {
	vm := NewJsonnetVM()

	snippet := `{
		a: 1,
		b: 2,
		sum: self.a + self.b
	}`

	result, err := vm.EvaluateAnonymousSnippet("test.jsonnet", snippet)

	if err != nil {
		t.Errorf("Failed to evaluate snippet: %v", err)
	}

	expected := `{
   "a": 1,
   "b": 2,
   "sum": 3
}
`
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
