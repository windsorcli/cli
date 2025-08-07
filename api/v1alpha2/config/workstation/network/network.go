package workstation

// NetworkConfig represents the network configuration
type NetworkConfig struct {
	CIDRBlock       *string `yaml:"cidr_block,omitempty"`
	LoadBalancerIPs *struct {
		Start *string `yaml:"start,omitempty"`
		End   *string `yaml:"end,omitempty"`
	} `yaml:"loadbalancer_ips,omitempty"`
}

// Merge merges the non-nil fields from another NetworkConfig into this one.
func (nc *NetworkConfig) Merge(other *NetworkConfig) {
	if other != nil {
		if other.CIDRBlock != nil {
			nc.CIDRBlock = other.CIDRBlock
		}
		if other.LoadBalancerIPs != nil {
			if nc.LoadBalancerIPs == nil {
				nc.LoadBalancerIPs = &struct {
					Start *string `yaml:"start,omitempty"`
					End   *string `yaml:"end,omitempty"`
				}{}
			}
			if other.LoadBalancerIPs.Start != nil {
				nc.LoadBalancerIPs.Start = other.LoadBalancerIPs.Start
			}
			if other.LoadBalancerIPs.End != nil {
				nc.LoadBalancerIPs.End = other.LoadBalancerIPs.End
			}
		}
	}
}

// DeepCopy creates a deep copy of the NetworkConfig.
func (nc *NetworkConfig) DeepCopy() *NetworkConfig {
	if nc == nil {
		return nil
	}

	var cidrBlockCopy *string
	if nc.CIDRBlock != nil {
		cidrBlockValue := *nc.CIDRBlock
		cidrBlockCopy = &cidrBlockValue
	}

	var loadBalancerIPsCopy *struct {
		Start *string `yaml:"start,omitempty"`
		End   *string `yaml:"end,omitempty"`
	}
	if nc.LoadBalancerIPs != nil {
		loadBalancerIPsCopy = &struct {
			Start *string `yaml:"start,omitempty"`
			End   *string `yaml:"end,omitempty"`
		}{}

		if nc.LoadBalancerIPs.Start != nil {
			startValue := *nc.LoadBalancerIPs.Start
			loadBalancerIPsCopy.Start = &startValue
		}

		if nc.LoadBalancerIPs.End != nil {
			endValue := *nc.LoadBalancerIPs.End
			loadBalancerIPsCopy.End = &endValue
		}
	}

	return &NetworkConfig{
		CIDRBlock:       cidrBlockCopy,
		LoadBalancerIPs: loadBalancerIPsCopy,
	}
}
