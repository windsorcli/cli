package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/getsops/sops/v3/decrypt"
	"github.com/spf13/cobra"
)

var sopsDecryptCmd = &cobra.Command{
	Use:   "decrypt <file>",
	Short: "Decrypt a file using SOPS",
	Long:  `Decrypt a file that was previously encrypted using SOPS.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		file := args[0]
		data, err := decryptFile(file)
		if err != nil {
			return err
		}

		fmt.Print(string(data))
		return nil
	},
}

var sopsEncryptCmd = &cobra.Command{
	Use:   "encrypt <file>",
	Short: "Encrypt a file using SOPS",
	Long:  `Encrypt a file to YAML format using SOPS.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		file := args[0]
		err := encryptFile(file)
		if err != nil {
			return err
		}

		fmt.Printf("File successfully encrypted: %s\n", file)
		return nil
	},
}

// decryptFile decrypts a file using the SOPS package
func decryptFile(filePath string) ([]byte, error) {
	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	// Decrypt the file using SOPS
	plaintextBytes, err := decrypt.File(filePath, "yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt file: %w", err)
	}

	return plaintextBytes, nil
}

// encryptFile encrypts a file using the SOPS CLI
func encryptFile(filePath string) error {
	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", filePath)
	}

	// Prepare the SOPS CLI command for encryption
	cmd := exec.Command("sops", "--encrypt", "--in-place", filePath)

	// Run the command and capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to encrypt file: %s, %s", err, string(output))
	}

	return nil
}

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "sops",
		Short: "Perform SOPS operations",
		Long:  `Perform various SOPS (Secrets OPerationS) operations like encryption and decryption.`,
	})

	sopsCmd := rootCmd.Commands()[len(rootCmd.Commands())-1]
	sopsCmd.AddCommand(sopsDecryptCmd)
	sopsCmd.AddCommand(sopsEncryptCmd)
}
