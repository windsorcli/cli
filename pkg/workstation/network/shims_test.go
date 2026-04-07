package network

import "testing"

func TestUtils(t *testing.T) {
	t.Run("TestInterfaces", func(t *testing.T) {
		// Given a real network interface provider
		provider := &RealNetworkInterfaceProvider{}

		// When getting network interfaces
		interfaces, err := provider.Interfaces()

		// Then no error should occur
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// And at least one interface should be returned
		if len(interfaces) == 0 {
			t.Fatalf("expected at least one interface, got %d", len(interfaces))
		}
	})

	t.Run("TestInterfaceAddrs", func(t *testing.T) {
		// Given a real network interface provider
		provider := &RealNetworkInterfaceProvider{}

		// And getting network interfaces
		interfaces, err := provider.Interfaces()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// When getting addresses for each interface
		for _, iface := range interfaces {
			// Then no error should occur
			addrs, err := provider.InterfaceAddrs(iface)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			// And the function should return without error
			t.Logf("interface %s has %d addresses", iface.Name, len(addrs))
		}
	})
}
