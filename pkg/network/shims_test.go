package network

import "testing"

func TestUtils(t *testing.T) {
	t.Run("TestInterfaces", func(t *testing.T) {
		provider := &RealNetworkInterfaceProvider{}
		interfaces, err := provider.Interfaces()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(interfaces) == 0 {
			t.Fatalf("expected at least one interface, got %d", len(interfaces))
		}
	})

	t.Run("TestInterfaceAddrs", func(t *testing.T) {
		provider := &RealNetworkInterfaceProvider{}
		interfaces, err := provider.Interfaces()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		for _, iface := range interfaces {
			addrs, err := provider.InterfaceAddrs(iface)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			// Simply check if the function returns without error
			t.Logf("interface %s has %d addresses", iface.Name, len(addrs))
		}
	})
}
