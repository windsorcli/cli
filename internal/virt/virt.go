package virt

// VMInfo is a struct that holds the information about the VM
type VMInfo struct {
	Address string
	Arch    string
	CPUs    int
	Disk    int
	Memory  int
	Name    string
}

type ContainerInfo struct {
	Name    string
	Address string
	Labels  map[string]string
}

// VirtInterface defines methods for the virt operations
type VirtInterface interface {
	Up(verbose ...bool) error
	Down(verbose ...bool) error
	Delete(verbose ...bool) error
	PrintInfo() error
	WriteConfig() error
}

// VMInterface defines methods for VM operations
type VMInterface interface {
	GetVMInfo() (VMInfo, error)
}

// ContainerInterface defines methods for container operations
type ContainerInterface interface {
	GetContainerInfo() ([]ContainerInfo, error)
}
