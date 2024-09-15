package interfaces

// ConfigHandler defines the interface for handling configuration operations
type ConfigHandler interface {
    // LoadConfig loads the configuration from the specified path
    LoadConfig(path string) error

    // GetConfigValue retrieves the value for the specified key from the configuration
    GetConfigValue(key string) (string, error)

    // SetConfigValue sets the value for the specified key in the configuration
    SetConfigValue(key string, value string) error

    // SaveConfig saves the current configuration to the specified path
    SaveConfig(path string) error
}
