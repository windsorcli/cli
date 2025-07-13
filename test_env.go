package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	// Set an environment variable
	os.Setenv("TEST_VAR", "test_value")
	
	// Check if it's visible in os.Environ()
	env := os.Environ()
	found := false
	for _, e := range env {
		if e == "TEST_VAR=test_value" {
			found = true
			break
		}
	}
	fmt.Printf("TEST_VAR found in os.Environ(): %v\n", found)
	
	// Create a command and set its environment
	cmd := exec.Command("env")
	cmd.Env = os.Environ()
	
	// Run the command and check if TEST_VAR is there
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Printf("TEST_VAR in command output: %v\n", string(output))
}
