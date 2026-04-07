package v1alpha2

import (
	"fmt"
	"strings"
	"testing"
)

func TestLoadSchemas(t *testing.T) {
	t.Run("LoadsAllEmbeddedSchemas", func(t *testing.T) {
		count := 0
		schemaNames := make(map[string]bool)

		err := LoadSchemas(func(content []byte) error {
			count++
			contentStr := string(content)
			if strings.Contains(contentStr, "Providers Configuration Schema") {
				schemaNames["providers"] = true
			} else if strings.Contains(contentStr, "Secrets Configuration Schema") {
				schemaNames["secrets"] = true
			} else if strings.Contains(contentStr, "Terraform Configuration Schema") {
				schemaNames["terraform"] = true
			} else if strings.Contains(contentStr, "Workstation Configuration Schema") {
				schemaNames["workstation"] = true
			}
			return nil
		})

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if count != 4 {
			t.Errorf("Expected 4 schemas to be loaded, got %d", count)
		}

		if !schemaNames["providers"] {
			t.Error("Expected providers schema to be loaded")
		}
		if !schemaNames["secrets"] {
			t.Error("Expected secrets schema to be loaded")
		}
		if !schemaNames["terraform"] {
			t.Error("Expected terraform schema to be loaded")
		}
		if !schemaNames["workstation"] {
			t.Error("Expected workstation schema to be loaded")
		}
	})

	t.Run("ReturnsErrorWhenLoaderFails", func(t *testing.T) {
		callCount := 0
		err := LoadSchemas(func(content []byte) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("loader error")
			}
			return nil
		})

		if err == nil {
			t.Fatal("Expected error when loader fails")
		}

		if !strings.Contains(err.Error(), "loader error") {
			t.Errorf("Expected error to contain loader error message, got: %v", err)
		}
	})
}

