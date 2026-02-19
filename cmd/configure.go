package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/windsorcli/cli/pkg/project"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure Windsor resources",
	Long:  "Configure Windsor resources such as workstation host/guest networking and DNS.",
}

var configureNetworkDnsAddress string

var configureNetworkCmd = &cobra.Command{
	Use:          "network",
	Short:        "Configure workstation host/guest networking and DNS",
	Long:         "Run from project root after the workstation Terraform component is applied. Use --dns-address to set the DNS service address; otherwise DNS is not configured.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var opts []*project.Project
		if overridesVal := cmd.Context().Value(projectOverridesKey); overridesVal != nil {
			if p, ok := overridesVal.(*project.Project); ok {
				opts = []*project.Project{p}
			}
		}

		proj := project.NewProject("", opts...)
		proj.Runtime.Shell.SetVerbosity(verbose)

		if err := proj.Runtime.Shell.CheckTrustedDirectory(); err != nil {
			return fmt.Errorf("not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve")
		}

		if err := proj.Configure(nil); err != nil {
			return err
		}

		if err := proj.Runtime.ConfigHandler.ValidateContextValues(); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}

		proj.EnsureWorkstation()
		if proj.Workstation == nil {
			fmt.Fprintln(os.Stderr, "workstation disabled")
			return nil
		}
		if err := proj.Workstation.Prepare(); err != nil {
			return err
		}
		dnsAddr, _ := cmd.Flags().GetString("dns-address")
		if proj.Workstation.NetworkManager == nil {
			fmt.Fprintln(os.Stderr, "network: n/a")
			return nil
		}
		return proj.Workstation.ConfigureNetwork(dnsAddr)
	},
}

func init() {
	configureNetworkCmd.Flags().StringVar(&configureNetworkDnsAddress, "dns-address", "", "DNS service address (e.g. from Terraform workstation output)")
	configureCmd.AddCommand(configureNetworkCmd)
	rootCmd.AddCommand(configureCmd)
}
