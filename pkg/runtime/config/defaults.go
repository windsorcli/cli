// This file defines default configurations that supplement schema and facet defaults.
// Only values NOT already provided by schema.yaml defaults or facet config blocks belong here.

package config

import (
	"fmt"
	"net/netip"
	"strconv"

	cliconfig "github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/pkg/constants"
)

// DefaultConfig is the base configuration for non-dev contexts (platform "none").
var DefaultConfig = cliconfig.Context{
	Platform: "none",
}

// DefaultConfig_Dev provides defaults for all dev contexts (docker-desktop, colima, incus).
var DefaultConfig_Dev = cliconfig.Context{}

// DefaultTerraformBackendTypeForPlatform returns the canonical terraform state backend for a
// platform, or "" when the platform has no default backend (the caller falls back to "local").
// Kept in sync with the platform description in configuration.yaml.
func DefaultTerraformBackendTypeForPlatform(platform string) string {
	switch platform {
	case "aws":
		return "s3"
	case "azure":
		return "azurerm"
	case "gcp":
		return "gcs"
	case "metal", "docker", "incus", "hetzner", "hyperv", "vsphere", "proxmox":
		return "kubernetes"
	default:
		return ""
	}
}

// InitNodeDefaults returns the node entries that windsor init writes, keyed by config path.
// Only a Hyper-V context with both gateways on NodePort gets entries, because its NAT switch serves no DHCP.
// Control plane N gets cidrhost(network.cidr_block, 9+N), and worker N gets cidrhost(network.cidr_block, 19+N).
// A pool that already has nodes gets no entries. The worker count is 1 when it is not set.
func InitNodeDefaults(values map[string]any) map[string]any {
	defaults := map[string]any{}
	if platform, _ := getValueByPathFromMap(values, parsePath("platform")).(string); platform != "hyperv" {
		return defaults
	}
	if gatewayServiceType(values, "external") != "NodePort" || gatewayServiceType(values, "internal") != "NodePort" {
		return defaults
	}
	cidrBlock, _ := getValueByPathFromMap(values, parsePath("network.cidr_block")).(string)
	if cidrBlock == "" {
		cidrBlock = constants.DefaultNetworkCIDR
	}
	addNodePoolDefaults(defaults, values, cidrBlock, "controlplanes", "controlplane", 9)
	addNodePoolDefaults(defaults, values, cidrBlock, "workers", "worker", 19)
	return defaults
}

// gatewayServiceType returns the service type of a gateway.
// A gateway without its own type uses gateway.service_type, and LoadBalancer when neither is set.
func gatewayServiceType(values map[string]any, gateway string) string {
	if serviceType, ok := getValueByPathFromMap(values, parsePath("gateway."+gateway+".service_type")).(string); ok && serviceType != "" {
		return serviceType
	}
	if serviceType, ok := getValueByPathFromMap(values, parsePath("gateway.service_type")).(string); ok && serviceType != "" {
		return serviceType
	}
	return "LoadBalancer"
}

// addNodePoolDefaults adds one entry per position of a pool that has no nodes yet.
// Position N gets the host at offset+N in cidrBlock. A position outside cidrBlock gets no entry.
func addNodePoolDefaults(defaults, values map[string]any, cidrBlock, pool, prefix string, offset int) {
	if nodes, ok := getValueByPathFromMap(values, parsePath("cluster."+pool+".nodes")).(map[string]any); ok && len(nodes) > 0 {
		return
	}
	count, ok := intValue(getValueByPathFromMap(values, parsePath("cluster."+pool+".count")))
	if !ok {
		count = 1
	}
	for position := 1; position <= count; position++ {
		address, ok := cidrHost(cidrBlock, offset+position)
		if !ok {
			continue
		}
		defaults[fmt.Sprintf("cluster.%s.nodes.%s-%d", pool, prefix, position)] = map[string]any{
			"node":     address,
			"endpoint": address + ":50000",
		}
	}
}

// cidrHost returns host number hostnum of an IPv4 cidrBlock, or false when the host is outside it.
func cidrHost(cidrBlock string, hostnum int) (string, bool) {
	prefix, err := netip.ParsePrefix(cidrBlock)
	if err != nil || !prefix.Addr().Is4() || hostnum < 0 {
		return "", false
	}
	address := prefix.Masked().Addr()
	for range hostnum {
		address = address.Next()
		if !prefix.Contains(address) {
			return "", false
		}
	}
	return address.String(), true
}

// intValue converts a config value to an int. A --set value arrives as a string.
func intValue(value any) (int, bool) {
	if value == nil {
		return 0, false
	}
	n, err := strconv.Atoi(fmt.Sprint(value))
	return n, err == nil
}
