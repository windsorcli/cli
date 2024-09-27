package cmd

import (
	"fmt"
	"os"

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
		data, error := decryptFile(file)

		fmt.Print(data)
		return error
	},
}

// decryptFile decrypts a file using the go.mozilla.org/sops/decrypt package
func decryptFile(filePath string) ([]byte, error) {
	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	// Decrypt the file using SOPS and return the decrypted content as a byte array
	plaintextBytes, err := decrypt.File(filePath, "yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt file: %w", err)
	}

	return plaintextBytes, nil
}

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "sops",
		Short: "Perform SOPS operations",
		Long:  `Perform various SOPS (Secrets OPerationS) operations like encryption and decryption.`,
	})

	sopsCmd := rootCmd.Commands()[len(rootCmd.Commands())-1]
	sopsCmd.AddCommand(sopsDecryptCmd)
}
