package windsor

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommand(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	rootCmd.AddCommand(cmd)

	if rootCmd.Use != "windsor" {
		t.Errorf("Expected root command Use to be 'windsor', got '%s'", rootCmd.Use)
	}

	if rootCmd.Short == "" {
		t.Error("Expected root command to have a short description")
	}

	if rootCmd.Long == "" {
		t.Error("Expected root command to have a long description")
	}
}
