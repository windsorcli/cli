// The CIDR functions are the expression helpers for IP address and network calculations.
// It provides cidrhost(), cidrsubnet(), cidrsubnets(), and cidrnetmask().
// The functions follow the Terraform functions of the same name.
// Each function accepts an int or a float64 for its number arguments.

package evaluator

import (
	"fmt"
	"math"
	"net"

	"github.com/expr-lang/expr"
)

// =============================================================================
// Private Methods
// =============================================================================

// cidrHostFunction returns the cidrhost(prefix, hostnum) helper.
func (e *expressionEvaluator) cidrHostFunction() expr.Option {
	return expr.Function(
		"cidrhost",
		func(params ...any) (any, error) {
			if len(params) != 2 {
				return nil, fmt.Errorf("cidrhost() requires exactly 2 arguments, got %d", len(params))
			}
			prefix, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("cidrhost() first argument must be a string, got %T", params[0])
			}
			hostnum, ok := numberArg(params[1])
			if !ok {
				return nil, fmt.Errorf("cidrhost() second argument must be a number, got %T", params[1])
			}
			return e.evaluateCidrHostFunction(prefix, hostnum)
		},
		new(func(string, int) string),
	)
}

// cidrSubnetFunction returns the cidrsubnet(prefix, newbits, netnum) helper.
func (e *expressionEvaluator) cidrSubnetFunction() expr.Option {
	return expr.Function(
		"cidrsubnet",
		func(params ...any) (any, error) {
			if len(params) != 3 {
				return nil, fmt.Errorf("cidrsubnet() requires exactly 3 arguments, got %d", len(params))
			}
			prefix, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("cidrsubnet() first argument must be a string, got %T", params[0])
			}
			newbits, ok := numberArg(params[1])
			if !ok {
				return nil, fmt.Errorf("cidrsubnet() second argument must be a number, got %T", params[1])
			}
			netnum, ok := numberArg(params[2])
			if !ok {
				return nil, fmt.Errorf("cidrsubnet() third argument must be a number, got %T", params[2])
			}
			return e.evaluateCidrSubnetFunction(prefix, newbits, netnum)
		},
		new(func(string, int, int) string),
	)
}

// cidrSubnetsFunction returns the cidrsubnets(prefix, newbits...) helper.
func (e *expressionEvaluator) cidrSubnetsFunction() expr.Option {
	return expr.Function(
		"cidrsubnets",
		func(params ...any) (any, error) {
			if len(params) < 2 {
				return nil, fmt.Errorf("cidrsubnets() requires at least 2 arguments, got %d", len(params))
			}
			prefix, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("cidrsubnets() first argument must be a string, got %T", params[0])
			}
			newbits := make([]int, len(params)-1)
			for i := 1; i < len(params); i++ {
				nb, ok := numberArg(params[i])
				if !ok {
					return nil, fmt.Errorf("cidrsubnets() argument %d must be a number, got %T", i+1, params[i])
				}
				newbits[i-1] = nb
			}
			return e.evaluateCidrSubnetsFunction(prefix, newbits)
		},
		new(func(string, ...int) []any),
	)
}

// cidrNetmaskFunction returns the cidrnetmask(prefix) helper.
func (e *expressionEvaluator) cidrNetmaskFunction() expr.Option {
	return expr.Function(
		"cidrnetmask",
		func(params ...any) (any, error) {
			if len(params) != 1 {
				return nil, fmt.Errorf("cidrnetmask() requires exactly 1 argument, got %d", len(params))
			}
			prefix, ok := params[0].(string)
			if !ok {
				return nil, fmt.Errorf("cidrnetmask() argument must be a string, got %T", params[0])
			}
			return e.evaluateCidrNetmaskFunction(prefix)
		},
		new(func(string) string),
	)
}

// evaluateCidrHostFunction calculates the IP address for a specific host number within a given CIDR block.
// The prefix parameter must be a valid CIDR notation (e.g., "10.5.0.0/16"), and hostnum specifies the
// host number within that network. Returns the IP address as a string (e.g., "10.5.0.10" for hostnum 10
// in "10.5.0.0/16"), or an error if the prefix is invalid or the host number is out of range.
func (e *expressionEvaluator) evaluateCidrHostFunction(prefix string, hostnum int) (string, error) {
	_, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	ones, bits := ipnet.Mask.Size()
	hostBits := bits - ones

	if hostnum < 0 {
		return "", fmt.Errorf("host number %d is out of range (must be non-negative) for CIDR %s", hostnum, prefix)
	}

	ip := make(net.IP, len(ipnet.IP))
	copy(ip, ipnet.IP)

	if len(ip) == net.IPv4len {
		if hostnum > math.MaxUint32 {
			return "", fmt.Errorf("host number %d is out of range for IPv4 address", hostnum)
		}
		hostnumUint32 := uint32(hostnum)
		ipInt := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
		if ipInt > math.MaxUint32-hostnumUint32 {
			return "", fmt.Errorf("host number %d causes overflow for CIDR %s", hostnum, prefix)
		}
		if hostBits < 64 {
			maxHosts := 1 << hostBits
			if hostnum >= maxHosts {
				return "", fmt.Errorf("host number %d is out of range (0-%d) for CIDR %s", hostnum, maxHosts-1, prefix)
			}
		}
		ipInt += hostnumUint32
		ip[0] = byte(ipInt >> 24) // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
		ip[1] = byte(ipInt >> 16) // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
		ip[2] = byte(ipInt >> 8)  // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
		ip[3] = byte(ipInt)       // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
	} else {
		if hostnum < 0 {
			return "", fmt.Errorf("host number %d is out of range for IPv6 address", hostnum)
		}
		hostnum64 := uint64(hostnum)
		for i := len(ip) - 1; i >= 0 && hostnum64 > 0; i-- {
			val := uint64(ip[i]) + (hostnum64 & 0xff)
			hostnum64 >>= 8
			if val > 255 {
				hostnum64 += val >> 8
				val &= 0xff
			}
			ip[i] = byte(val)
		}
		if hostnum64 > 0 {
			return "", fmt.Errorf("host number %d is too large for CIDR %s", hostnum, prefix)
		}
	}

	if !ipnet.Contains(ip) {
		return "", fmt.Errorf("host number %d produces IP %s which is outside CIDR %s", hostnum, ip.String(), prefix)
	}

	return ip.String(), nil
}

// evaluateCidrSubnetFunction calculates a subnet address within a given CIDR block by adding new bits
// to the prefix and specifying the subnet number. The prefix parameter must be a valid CIDR notation,
// newbits specifies how many additional bits to add for subnetting, and netnum is the subnet number.
// Returns the new subnet CIDR as a string (e.g., "10.5.1.0/24" for cidrsubnet("10.5.0.0/16", 8, 1)),
// or an error if the prefix is invalid or the subnet parameters are out of range.
func (e *expressionEvaluator) evaluateCidrSubnetFunction(prefix string, newbits int, netnum int) (string, error) {
	_, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	ones, bits := ipnet.Mask.Size()
	if newbits < 0 || ones+newbits > bits {
		return "", fmt.Errorf("newbits %d is invalid for CIDR %s (prefix length %d, total bits %d)", newbits, prefix, ones, bits)
	}

	if netnum < 0 {
		return "", fmt.Errorf("netnum %d is out of range (must be non-negative) for newbits %d", netnum, newbits)
	}

	newPrefixLen := ones + newbits
	subnetSize := 1 << (bits - newPrefixLen)
	subnetIP := make(net.IP, len(ipnet.IP))
	copy(subnetIP, ipnet.IP)

	offset := netnum * subnetSize

	if len(subnetIP) == net.IPv4len {
		if offset > math.MaxUint32 {
			return "", fmt.Errorf("offset %d is out of range for IPv4 address", offset)
		}
		offsetUint32 := uint32(offset) // #nosec G115 -- bounds-checked above (offset <= math.MaxUint32)
		ipInt := uint32(subnetIP[0])<<24 | uint32(subnetIP[1])<<16 | uint32(subnetIP[2])<<8 | uint32(subnetIP[3])
		if ipInt > math.MaxUint32-offsetUint32 {
			return "", fmt.Errorf("offset %d causes overflow for CIDR %s", offset, prefix)
		}
		if newbits < 64 {
			maxSubnets := 1 << newbits
			if netnum >= maxSubnets {
				return "", fmt.Errorf("netnum %d is out of range (0-%d) for newbits %d", netnum, maxSubnets-1, newbits)
			}
		}
		ipInt += offsetUint32
		subnetIP[0] = byte(ipInt >> 24) // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
		subnetIP[1] = byte(ipInt >> 16) // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
		subnetIP[2] = byte(ipInt >> 8)  // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
		subnetIP[3] = byte(ipInt)       // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
	} else {
		if offset < 0 {
			return "", fmt.Errorf("offset %d is out of range for IPv6 address", offset)
		}
		offset64 := uint64(offset)
		for i := len(subnetIP) - 1; i >= 0 && offset64 > 0; i-- {
			val := uint64(subnetIP[i]) + (offset64 & 0xff)
			offset64 >>= 8
			if val > 255 {
				offset64 += val >> 8
				val &= 0xff
			}
			subnetIP[i] = byte(val)
		}
		if offset64 > 0 {
			return "", fmt.Errorf("offset %d is too large for CIDR %s", offset, prefix)
		}
	}

	mask := net.CIDRMask(newPrefixLen, bits)
	subnetIP.Mask(mask)

	return fmt.Sprintf("%s/%d", subnetIP.String(), newPrefixLen), nil
}

// evaluateCidrSubnetsFunction generates multiple subnets from a single CIDR block by specifying
// multiple newbits values. The prefix parameter must be a valid CIDR notation, and newbits is
// a slice of integers specifying how many additional bits to add for each subnet. Returns a
// slice of subnet CIDR strings (e.g., ["10.5.0.0/24", "10.5.1.0/24"] for cidrsubnets("10.5.0.0/16", 8, 8)),
// or an error if the prefix is invalid or any subnet parameters are out of range.
func (e *expressionEvaluator) evaluateCidrSubnetsFunction(prefix string, newbits []int) ([]any, error) {
	_, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	ones, bits := ipnet.Mask.Size()
	results := make([]any, len(newbits))
	baseIP := make(net.IP, len(ipnet.IP))
	copy(baseIP, ipnet.IP)
	cumulativeOffset := 0

	for i, nb := range newbits {
		if nb < 0 || ones+nb > bits {
			return nil, fmt.Errorf("newbits[%d]=%d is invalid for CIDR %s (prefix length %d, total bits %d)", i, nb, prefix, ones, bits)
		}

		newPrefixLen := ones + nb
		subnetSize := 1 << (bits - newPrefixLen)
		offset := cumulativeOffset

		subnetIP := make(net.IP, len(baseIP))
		copy(subnetIP, baseIP)

		if len(subnetIP) == net.IPv4len {
			if offset < 0 || offset > math.MaxUint32 {
				return nil, fmt.Errorf("offset %d is out of range for IPv4 address", offset)
			}
			offsetUint32 := uint32(offset) // #nosec G115 -- bounds-checked above (offset <= math.MaxUint32)
			ipInt := uint32(subnetIP[0])<<24 | uint32(subnetIP[1])<<16 | uint32(subnetIP[2])<<8 | uint32(subnetIP[3])
			if ipInt > math.MaxUint32-offsetUint32 {
				return nil, fmt.Errorf("offset %d causes overflow for CIDR %s", offset, prefix)
			}
			ipInt += offsetUint32
			subnetIP[0] = byte(ipInt >> 24) // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
			subnetIP[1] = byte(ipInt >> 16) // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
			subnetIP[2] = byte(ipInt >> 8)  // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
			subnetIP[3] = byte(ipInt)       // #nosec G115 -- uint32->byte bit extraction is safe (shift guarantees range)
		} else {
			if offset < 0 {
				return nil, fmt.Errorf("offset %d is out of range for IPv6 address", offset)
			}
			offset64 := uint64(offset)
			for j := len(subnetIP) - 1; j >= 0 && offset64 > 0; j-- {
				val := uint64(subnetIP[j]) + (offset64 & 0xff)
				offset64 >>= 8
				if val > 255 {
					offset64 += val >> 8
					val &= 0xff
				}
				subnetIP[j] = byte(val)
			}
			if offset64 > 0 {
				return nil, fmt.Errorf("offset %d is too large for CIDR %s", offset, prefix)
			}
		}

		mask := net.CIDRMask(newPrefixLen, bits)
		subnetIP.Mask(mask)

		results[i] = fmt.Sprintf("%s/%d", subnetIP.String(), newPrefixLen)

		cumulativeOffset += subnetSize
	}

	return results, nil
}

// evaluateCidrNetmaskFunction converts a CIDR prefix to a subnet mask in dotted-decimal notation.
// The prefix parameter must be a valid CIDR notation (e.g., "10.5.0.0/16"). Returns the subnet
// mask as a string (e.g., "255.255.0.0" for "/16"), or an error if the prefix is invalid.
func (e *expressionEvaluator) evaluateCidrNetmaskFunction(prefix string) (string, error) {
	_, ipnet, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR prefix: %w", err)
	}

	mask := ipnet.Mask
	if len(mask) == net.IPv4len {
		return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3]), nil
	}

	return "", fmt.Errorf("CIDR prefix must be IPv4, got IPv6")
}

// =============================================================================
// Helpers
// =============================================================================

// numberArg converts an int or float64 expression argument to an int. It reports false for any other type.
func numberArg(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
