package evaluator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestJsonnetHelper(t *testing.T) {
	t.Run("JsonnetFunctionWithValidPath", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		jsonnetFile := filepath.Join(tmpDir, "test.jsonnet")
		os.WriteFile(jsonnetFile, []byte(`{"result": "success"}`), 0644)

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		result, err := evaluator.Evaluate(`${jsonnet("test.jsonnet")}`, featurePath, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["result"] != "success" {
			t.Errorf("Expected result.result to be 'success', got %v", resultMap["result"])
		}
	})

	t.Run("JsonnetFunctionWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${jsonnet("path1", "path2")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("JsonnetFunctionWithNonStringArgument", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${jsonnet(42)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string argument, got nil")
		}
	})
}

func TestFileHelper(t *testing.T) {
	t.Run("FileFunctionWithValidPath", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("file content"), 0644)

		featurePath := filepath.Join(tmpDir, "feature.yaml")

		result, err := evaluator.Evaluate(`${file("test.txt")}`, featurePath, false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "file content" {
			t.Errorf("Expected result to be 'file content', got '%v'", result)
		}
	})

	t.Run("FileFunctionWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${file("path1", "path2")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("FileFunctionWithNonStringArgument", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${file(42)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string argument, got nil")
		}
	})
}

func TestSplitHelper(t *testing.T) {
	t.Run("SplitFunction", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${split("a,b,c", ",")}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be an array, got %T", result)
		}

		if len(resultArray) != 3 {
			t.Errorf("Expected result to have 3 elements, got %d", len(resultArray))
		}

		if resultArray[0] != "a" || resultArray[1] != "b" || resultArray[2] != "c" {
			t.Errorf("Expected result to be ['a', 'b', 'c'], got %v", resultArray)
		}
	})

	t.Run("SplitFunctionWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${split("a,b")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("SplitFunctionWithNonStringArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${split(42, ",")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string arguments, got nil")
		}
	})
}

func TestCidrHostHelper(t *testing.T) {
	t.Run("CidrHostWithValidInput", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrhost("10.5.0.0/16", 10)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "10.5.0.10" {
			t.Errorf("Expected result to be '10.5.0.10', got '%v'", result)
		}
	})

	t.Run("CidrHostWithHostnumZero", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrhost("192.168.1.0/24", 0)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "192.168.1.0" {
			t.Errorf("Expected result to be '192.168.1.0', got '%v'", result)
		}
	})

	t.Run("CidrHostWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("10.5.0.0/16")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("CidrHostWithNonStringPrefix", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost(42, 10)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string prefix, got nil")
		}
	})

	t.Run("CidrHostWithNonNumberHostnum", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("10.5.0.0/16", "10")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-number hostnum, got nil")
		}
	})

	t.Run("CidrHostWithInvalidCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("invalid", 10)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid CIDR, got nil")
		}
	})

	t.Run("CidrHostWithOutOfRangeHostnum", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("10.5.0.0/16", 65536)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for out of range hostnum, got nil")
		}
	})

	t.Run("CidrHostWithIPv6", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrhost("2001:db8::/96", 1)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !strings.Contains(result.(string), "2001:db8::") {
			t.Errorf("Expected result to contain '2001:db8::', got '%v'", result)
		}
	})

	t.Run("CidrHostWithIPv6Slash64", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrhost("2001:db8::/64", 0)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error for /64 network with hostnum 0, got: %v", err)
		}

		if !strings.Contains(result.(string), "2001:db8::") {
			t.Errorf("Expected result to contain '2001:db8::', got '%v'", result)
		}
	})

	t.Run("CidrHostWithIPv6OutOfRange", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("2001:db8::/128", 2)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for IPv6 out of range hostnum, got nil")
		}
	})

	t.Run("CidrHostWithHostnumExceedingMaxUint32", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("10.0.0.0/8", 4294967296)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for hostnum exceeding MaxUint32, got nil")
		}
		if !strings.Contains(err.Error(), "out of range for IPv4 address") {
			t.Errorf("Expected error message about out of range, got: %v", err)
		}
	})

	t.Run("CidrHostWithOverflowDetection", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrhost("255.255.255.0/24", 256)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for overflow, got nil")
		}
		if !strings.Contains(err.Error(), "causes overflow") {
			t.Errorf("Expected error message about overflow, got: %v", err)
		}
	})
}

func TestCidrSubnetHelper(t *testing.T) {
	t.Run("CidrSubnetWithValidInput", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnet("10.5.0.0/16", 8, 1)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "10.5.1.0/24" {
			t.Errorf("Expected result to be '10.5.1.0/24', got '%v'", result)
		}
	})

	t.Run("CidrSubnetWithNetnumZero", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnet("10.0.0.0/8", 8, 0)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "10.0.0.0/16" {
			t.Errorf("Expected result to be '10.0.0.0/16', got '%v'", result)
		}
	})

	t.Run("CidrSubnetWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("10.5.0.0/16", 8)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("CidrSubnetWithNonStringPrefix", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet(42, 8, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string prefix, got nil")
		}
	})

	t.Run("CidrSubnetWithNonNumberNewbits", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("10.5.0.0/16", "8", 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-number newbits, got nil")
		}
	})

	t.Run("CidrSubnetWithInvalidCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("invalid", 8, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid CIDR, got nil")
		}
	})

	t.Run("CidrSubnetWithInvalidNewbits", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("10.5.0.0/16", 17, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid newbits, got nil")
		}
	})

	t.Run("CidrSubnetWithOutOfRangeNetnum", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("10.5.0.0/16", 8, 256)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for out of range netnum, got nil")
		}
	})

	t.Run("CidrSubnetWithIPv6", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnet("2001:db8::/64", 16, 256)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if !strings.Contains(result.(string), "2001:db8") {
			t.Errorf("Expected result to contain '2001:db8', got '%v'", result)
		}
		if !strings.Contains(result.(string), "/80") {
			t.Errorf("Expected result to contain '/80', got '%v'", result)
		}
	})

	t.Run("CidrSubnetWithLargeNewbits", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnet("2001:db8::/32", 64, 0)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error for /32 network with newbits=64 and netnum=0, got: %v", err)
		}

		if !strings.Contains(result.(string), "2001:db8") {
			t.Errorf("Expected result to contain '2001:db8', got '%v'", result)
		}
		if !strings.Contains(result.(string), "/96") {
			t.Errorf("Expected result to contain '/96', got '%v'", result)
		}
	})

	t.Run("CidrSubnetWithNonCanonicalCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnet("10.5.3.7/16", 8, 1)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "10.5.1.0/24" {
			t.Errorf("Expected result to be '10.5.1.0/24' (using network base address), got '%v'", result)
		}
	})

	t.Run("CidrSubnetWithOffsetExceedingMaxUint32", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("0.0.0.0/0", 0, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for offset exceeding MaxUint32, got nil")
		}
		if !strings.Contains(err.Error(), "out of range for IPv4 address") {
			t.Errorf("Expected error message about out of range, got: %v", err)
		}
	})

	t.Run("CidrSubnetWithOverflowDetection", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnet("255.255.255.0/24", 0, 1)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for overflow, got nil")
		}
		if !strings.Contains(err.Error(), "causes overflow") {
			t.Errorf("Expected error message about overflow, got: %v", err)
		}
	})
}

func TestCidrSubnetsHelper(t *testing.T) {
	t.Run("CidrSubnetsWithValidInput", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnets("10.5.0.0/16", 8, 8)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be []any, got %T", result)
		}

		if len(resultArray) != 2 {
			t.Fatalf("Expected result to have 2 elements, got %d", len(resultArray))
		}

		if resultArray[0] != "10.5.0.0/24" {
			t.Errorf("Expected result[0] to be '10.5.0.0/24', got '%v'", resultArray[0])
		}

		if resultArray[1] != "10.5.1.0/24" {
			t.Errorf("Expected result[1] to be '10.5.1.0/24', got '%v'", resultArray[1])
		}
	})

	t.Run("CidrSubnetsWithSingleSubnet", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnets("10.0.0.0/8", 8)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be []any, got %T", result)
		}

		if len(resultArray) != 1 {
			t.Fatalf("Expected result to have 1 element, got %d", len(resultArray))
		}

		if resultArray[0] != "10.0.0.0/16" {
			t.Errorf("Expected result[0] to be '10.0.0.0/16', got '%v'", resultArray[0])
		}
	})

	t.Run("CidrSubnetsWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnets("10.5.0.0/16")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("CidrSubnetsWithNonStringPrefix", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnets(42, 8)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string prefix, got nil")
		}
	})

	t.Run("CidrSubnetsWithNonNumberNewbits", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnets("10.5.0.0/16", "8")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-number newbits, got nil")
		}
	})

	t.Run("CidrSubnetsWithInvalidCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrsubnets("invalid", 8)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid CIDR, got nil")
		}
	})

	t.Run("CidrSubnetsWithIPv6", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnets("2001:db8::/64", 16, 16)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be []any, got %T", result)
		}

		if len(resultArray) != 2 {
			t.Fatalf("Expected result to have 2 elements, got %d", len(resultArray))
		}

		if !strings.Contains(resultArray[0].(string), "2001:db8") {
			t.Errorf("Expected result[0] to contain '2001:db8', got '%v'", resultArray[0])
		}
		if !strings.Contains(resultArray[1].(string), "2001:db8") {
			t.Errorf("Expected result[1] to contain '2001:db8', got '%v'", resultArray[1])
		}
	})

	t.Run("CidrSubnetsWithDifferentSizes", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnets("10.0.0.0/8", 4, 8)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be []any, got %T", result)
		}

		if len(resultArray) != 2 {
			t.Fatalf("Expected result to have 2 elements, got %d", len(resultArray))
		}

		if resultArray[0] != "10.0.0.0/12" {
			t.Errorf("Expected result[0] to be '10.0.0.0/12', got '%v'", resultArray[0])
		}

		if resultArray[1] != "10.16.0.0/16" {
			t.Errorf("Expected result[1] to be '10.16.0.0/16', got '%v'", resultArray[1])
		}
	})

	t.Run("CidrSubnetsWithNonCanonicalCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrsubnets("10.5.3.7/16", 8, 8)}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		resultArray, ok := result.([]any)
		if !ok {
			t.Fatalf("Expected result to be []any, got %T", result)
		}

		if len(resultArray) != 2 {
			t.Fatalf("Expected result to have 2 elements, got %d", len(resultArray))
		}

		if resultArray[0] != "10.5.0.0/24" {
			t.Errorf("Expected result[0] to be '10.5.0.0/24' (using network base address), got '%v'", resultArray[0])
		}

		if resultArray[1] != "10.5.1.0/24" {
			t.Errorf("Expected result[1] to be '10.5.1.0/24' (using network base address), got '%v'", resultArray[1])
		}
	})
}

func TestCidrNetmaskHelper(t *testing.T) {
	t.Run("CidrNetmaskWithValidInput", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrnetmask("10.5.0.0/16")}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "255.255.0.0" {
			t.Errorf("Expected result to be '255.255.0.0', got '%v'", result)
		}
	})

	t.Run("CidrNetmaskWithSlash24", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrnetmask("192.168.1.0/24")}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "255.255.255.0" {
			t.Errorf("Expected result to be '255.255.255.0', got '%v'", result)
		}
	})

	t.Run("CidrNetmaskWithSlash8", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		result, err := evaluator.Evaluate(`${cidrnetmask("10.0.0.0/8")}`, "", false)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result != "255.0.0.0" {
			t.Errorf("Expected result to be '255.0.0.0', got '%v'", result)
		}
	})

	t.Run("CidrNetmaskWithInvalidArguments", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrnetmask("10.5.0.0/16", "extra")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid arguments, got nil")
		}
	})

	t.Run("CidrNetmaskWithNonStringPrefix", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrnetmask(42)}`, "", false)

		if err == nil {
			t.Fatal("Expected error for non-string prefix, got nil")
		}
	})

	t.Run("CidrNetmaskWithInvalidCIDR", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrnetmask("invalid")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for invalid CIDR, got nil")
		}
	})

	t.Run("CidrNetmaskWithIPv6", func(t *testing.T) {
		evaluator, _, _, _ := setupEvaluatorTest(t)

		_, err := evaluator.Evaluate(`${cidrnetmask("2001:db8::/32")}`, "", false)

		if err == nil {
			t.Fatal("Expected error for IPv6 CIDR, got nil")
		}
		if !strings.Contains(err.Error(), "IPv4") {
			t.Errorf("Expected error to mention 'IPv4', got: %v", err)
		}
	})
}
