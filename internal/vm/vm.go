package vm

import "encoding/json"

var jsonUnmarshal = json.Unmarshal

// VMInfo holds the information about the virtual machine
type VMInfo struct {
	Address string
	Arch    string
	CPUs    int
	Disk    float64
	Memory  float64
	Name    string
	Runtime string
	Status  string
}

// VMInterface defines methods for VM operations
type VMInterface interface {
	Up(verbose ...bool) error
	Down(verbose ...bool) error
	Delete(verbose ...bool) error
	Info() (interface{}, error)
}
