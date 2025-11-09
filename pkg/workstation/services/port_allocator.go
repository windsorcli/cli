package services

// PortAllocator manages port allocation for services during network initialization.
// It tracks allocated ports to prevent conflicts.
type PortAllocator struct {
	allocatedPorts map[int]bool
}

// NewPortAllocator creates a new PortAllocator with initial state.
func NewPortAllocator() *PortAllocator {
	return &PortAllocator{
		allocatedPorts: make(map[int]bool),
	}
}

// NextAvailablePort finds the next available port starting from basePort. If basePort is already allocated,
// it increments until finding an available port. Returns the allocated port.
func (p *PortAllocator) NextAvailablePort(basePort int) int {
	port := basePort
	for p.allocatedPorts[port] {
		port++
	}
	p.allocatedPorts[port] = true
	return port
}
