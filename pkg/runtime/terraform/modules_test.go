package terraform_test

import (
	"reflect"
	"testing"

	"github.com/windsorcli/cli/pkg/runtime/terraform"
)

func TestParseModuleSources(t *testing.T) {
	t.Run("ReturnsTheSourceOfEachModuleBlock", func(t *testing.T) {
		// Given a file with a literal source, a variable source and a module with no source
		content := "module \"a\" {\n  source = \"./a\"\n}\nmodule \"b\" {\n  source = var.b\n}\nmodule \"c\" {\n}\n"

		// When the file is parsed
		got, err := terraform.ParseModuleSources("main.tf", []byte(content))

		// Then each module with a source is returned
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		want := []terraform.ModuleSource{{Name: "a", Source: "./a", Literal: true}, {Name: "b"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expected %+v, got %+v", want, got)
		}
	})

	t.Run("ReturnsAnErrorForContentThatDoesNotParse", func(t *testing.T) {
		// Given content that is not HCL
		content := "image: v1\n"

		// When the content is parsed
		_, err := terraform.ParseModuleSources("main.tf", []byte(content))

		// Then an error is returned
		if err == nil {
			t.Error("expected an error")
		}
	})
}
