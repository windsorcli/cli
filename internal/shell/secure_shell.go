package shell

// Defines basic SSH connection parameters
type SSHConnectionParams struct {
	Host         string
	Port         int
	Username     string
	IdentityFile string
}

// SecureShell is the implementation of the Shell interface for Colima
type SecureShell struct {
	DefaultShell
	sshParams SSHConnectionParams
}

// NewSecureShell creates a new instance of SecureShell
func NewSecureShell(sshParams SSHConnectionParams) *SecureShell {
	return &SecureShell{
		DefaultShell: *NewDefaultShell(),
		sshParams:    sshParams,
	}
}

// PrintEnvVars prints the environment variables in a sorted order.
// If a custom PrintEnvVarsFn is provided, it will use that function instead.
func (d *SecureShell) PrintEnvVars(envVars map[string]string) {
	d.DefaultShell.PrintEnvVars(envVars)
}

// GetProjectRoot returns the project root directory.
// If a custom GetProjectRootFunc is provided, it will use that function instead.
func (d *SecureShell) GetProjectRoot() (string, error) {
	return d.DefaultShell.GetProjectRoot()
}

// Exec executes a command and returns its output as a string
func (d *SecureShell) Exec(verbose bool, message string, command string, args ...string) (string, error) {
	return d.DefaultShell.Exec(verbose, message, command, args...)
}

// Ensure secure shell is an instance of Shell
var _ Shell = (*SecureShell)(nil)
