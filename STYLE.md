# Windsor CLI Code Style Guide

## Package Structure

A typical package should contain:
1. Main implementation file (`{name}.go`)
2. Test file (`{name}_test.go`)
3. Mock implementation (`mock_{name}.go`)
4. Test mocks (`mock_{name}_test.go`)
5. Shims file (`shims.go`) for testable system calls

## File Organization

### Main Implementation File
1. Package declaration and imports
2. Interface definition with detailed documentation
3. Base struct definition
4. Class header comment block
5. Section headers using `// =============================================================================`
6. Methods organized by visibility (public/private)

### Test File
1. Test setup section with Mocks struct and SetupOptions
2. Global setupMocks function
3. Test functions using t.Run for BDD style
4. Local setup functions within each test
5. BDD style comments (Given/When/Then)

### Mock Implementation
1. Mock struct with function fields
2. Constructor
3. Interface implementation methods
4. Interface compliance check

### Shims
1. System call variables that can be overridden in tests
2. Minimal implementation, just variable declarations

## Documentation Style

### Class Headers
Every package-level file MUST begin with a class header in the following exact format:
```go
// The [ClassName] is a [brief description]
// It provides [detailed explanation]
// [role in application]
// [key features/capabilities]
```

Each line MUST start with "// " and MUST NOT be empty. The first line MUST start with "The [ClassName] is a".
The second line MUST start with "It provides". The third and fourth lines describe role and features.

Example:
```go
// The ConfigHandler is a core component that manages configuration state and context.
// It provides a unified interface for loading, saving, and accessing configuration data,
// The ConfigHandler acts as the central configuration orchestrator for the application,
// coordinating context switching, secret management, and configuration persistence.
```

### Section Headers
Section headers MUST follow this exact format with no variations:
```go
// =============================================================================
// [SECTION NAME]
// =============================================================================
```

The following are the ONLY allowed section names, in this exact order when present:

For implementation files (*.go, mock_*.go):
1. Constants
2. Types
3. Interfaces
4. Constructor
5. Public Methods
6. Private Methods
7. Helpers

For test files (*_test.go ONLY):
1. Test Setup
2. Test Constructor
3. Test Public Methods
4. Test Private Methods
5. Test Helpers

IMPORTANT:
- Only include section headers for sections that contain code
- Empty sections MUST be omitted entirely
- The equals signs must be exactly 77 characters long
- There must be exactly one blank line before and after each section header
- Sections must appear in the order specified above when present

Example for implementation file with only types and constructor:
```go
// =============================================================================
// Types
// =============================================================================

type Config struct {
    // fields
}

// =============================================================================
// Constructor
// =============================================================================

func NewConfig() *Config {
    return &Config{}
}
```

Example for test file with only setup and methods:
```go
// =============================================================================
// Test Setup
// =============================================================================

type Mocks struct {
    // fields
}

// =============================================================================
// Test Methods
// =============================================================================

func TestConfig_Load(t *testing.T) {
    // test implementation
}
```

### Method Documentation
- Brief description at the top of each method
- No inline comments within method bodies
- Focus on what and why, not how

Example from config_handler.go:
```go
// Initialize sets up the config handler by resolving and storing the shell dependency.
func (c *BaseConfigHandler) Initialize() error {
    // Implementation...
}
```

## Testing Patterns

### Test Structure

Tests should follow a BDD (Behavior-Driven Development) style with Given/When/Then comments:

```go
t.Run("Scenario", func(t *testing.T) {
    // Given [context]
    mocks, obj := setup(t)
    
    // When [action]
    err := obj.DoSomething()
    
    // Then [result]
    if err != nil {
        t.Errorf("Expected success, got error: %v", err)
    }
})
```

### Mock Setup

The base setupMocks function provides common mock behaviors. To extend it for specific test needs:

```go
// setupDockerMocks extends the base setupMocks function with Docker-specific mock behaviors.
// It sets up mock configurations for Docker commands, container inspection, and service configuration.
func setupDockerMocks(t *testing.T, opts ...*SetupOptions) (*Mocks, string) {
    t.Helper()

    // Store original directory and create temp dir
    origDir, err := os.Getwd()
    if err != nil {
        t.Fatalf("Failed to get working directory: %v", err)
    }

    tmpDir := t.TempDir()
    if err := os.Chdir(tmpDir); err != nil {
        t.Fatalf("Failed to change to temp directory: %v", err)
    }

    // Set project root environment variable
    os.Setenv("WINDSOR_PROJECT_ROOT", tmpDir)

    // Get base mocks
    mocks := setupMocks(t, opts...)

    // Add Docker-specific mock behaviors
    mocks.MockConfigHandler.GetBoolFunc = func(key string, defaultValue ...bool) bool {
        if key == "docker.enabled" {
            return true
        }
        return false
    }

    // Register cleanup to restore original state
    t.Cleanup(func() {
        os.Unsetenv("WINDSOR_PROJECT_ROOT")
        if err := os.Chdir(origDir); err != nil {
            t.Logf("Warning: Failed to change back to original directory: %v", err)
        }
    })

    return mocks, tmpDir
}
```

Key points for extending setupMocks:
1. Call the base setupMocks function first
2. Add domain-specific mock behaviors
3. Handle cleanup of any additional resources
4. Return both mocks and any additional context needed

### Baseline Configuration
1. Use ConfigStr in SetupOptions to set baseline configuration
2. Default config should be minimal but functional
3. Test-specific config should extend default config
4. Config should be in YAML format
5. Config should be loaded after handler initialization

Example:
```go
// Default config in setupMocks
defaultConfigStr := `
contexts:
  mock-context:
    dns:
      domain: mock.domain.com
    network:
      cidr_block: 10.0.0.0/24`

// Test-specific config
testConfig := `
contexts:
  mock-context:
    dns:
      enabled: true
      address: 10.0.0.53
    docker:
      enabled: true
      compose_file: docker-compose.yml`

mocks := setupMocks(t, &SetupOptions{
    ConfigStr: testConfig,
})
```

### BDD Style
```go
t.Run("Scenario", func(t *testing.T) {
    // Given [context]
    mocks, obj := setup(t)
    
    // When [action]
    err := obj.DoSomething()
    
    // Then [result]
    if err != nil {
        t.Errorf("Expected success, got error: %v", err)
    }
})
```

## Code Organization

### Interface Definition
1. Clear, focused interface
2. Detailed documentation
3. Minimal method set

Example from config_handler.go:
```go
type ConfigHandler interface {
    Initialize() error
    LoadConfig(path string) error
    LoadConfigString(content string) error
    GetString(key string, defaultValue ...string) string
    GetInt(key string, defaultValue ...int) int
    GetBool(key string, defaultValue ...bool) bool
    GetStringSlice(key string, defaultValue ...[]string) []string
    GetStringMap(key string, defaultValue ...map[string]string) map[string]string
    Set(key string, value any) error
    SetContextValue(key string, value any) error
    Get(key string) any
    SaveConfig(path string, overwrite ...bool) error
    SetDefault(context v1alpha1.Context) error
    GetConfig() *v1alpha1.Context
    GetContext() string
    SetContext(context string) error
    GetConfigRoot() (string, error)
    Clean() error
    IsLoaded() bool
}
```

### Base Implementation
1. Struct with dependencies
2. Constructor function
3. Interface implementation
4. Clear separation of concerns

Example from config_handler.go:
```go
// BaseConfigHandler is a base implementation of the ConfigHandler interface
type BaseConfigHandler struct {
    injector         di.Injector
    shell            shell.Shell
    config           v1alpha1.Config
    context          string
    secretsProviders []secrets.SecretsProvider
    loaded           bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewBaseConfigHandler creates a new BaseConfigHandler instance
func NewBaseConfigHandler(injector di.Injector) *BaseConfigHandler {
    return &BaseConfigHandler{injector: injector}
}
```

### Mock Implementation
1. Function fields for each interface method
2. Default implementations
3. Interface compliance check
4. Minimal implementation
5. No internal comments - mock implementations should be self-documenting through their structure

Example from mock_config_handler.go:
```go
// MockConfigHandler is a mock implementation of the ConfigHandler interface
type MockConfigHandler struct {
    InitializeFunc       func() error
    LoadConfigFunc       func(path string) error
    LoadConfigStringFunc func(content string) error
    IsLoadedFunc         func() bool
    // ... other function fields
}

// =============================================================================
// Constructor
// =============================================================================

// NewMockConfigHandler is a constructor for MockConfigHandler
func NewMockConfigHandler() *MockConfigHandler {
    return &MockConfigHandler{}
}

// =============================================================================
// Public Methods
// =============================================================================

// Initialize calls the mock InitializeFunc if set, otherwise returns nil
func (m *MockConfigHandler) Initialize() error {
    if m.InitializeFunc != nil {
        return m.InitializeFunc()
    }
    return nil
}

// Ensure MockConfigHandler implements ConfigHandler
var _ ConfigHandler = (*MockConfigHandler)(nil)
```

### Shims
1. Create a `shims.go` file in each package that needs system call abstraction
2. Define a `Shims` struct with function fields for each system call
3. Implement a `NewShims()` constructor that provides default implementations
4. Use the Shims struct in your implementation for dependency injection
5. Override specific functions in tests as needed

Example from shims.go:
```go
package virt

import (
    "encoding/json"
    "io"
    "os"
    "runtime"

    "github.com/goccy/go-yaml"
    "github.com/shirou/gopsutil/mem"
)

// The shims package is a system call abstraction layer
// It provides mockable wrappers around system and runtime functions
// It serves as a testing aid by allowing system calls to be intercepted
// It enables dependency injection and test isolation for system-level operations

// =============================================================================
// Types
// =============================================================================

// YAMLEncoder is an interface for encoding YAML data.
type YAMLEncoder interface {
    Encode(v any) error
    Close() error
}

// Shims provides mockable wrappers around system and runtime functions
type Shims struct {
    Setenv         func(key, value string) error
    UnmarshalJSON  func(data []byte, v any) error
    UserHomeDir    func() (string, error)
    MkdirAll       func(path string, perm os.FileMode) error
    WriteFile      func(name string, data []byte, perm os.FileMode) error
    Rename         func(oldpath, newpath string) error
    GOARCH         func() string
    NumCPU         func() int
    VirtualMemory  func() (*mem.VirtualMemoryStat, error)
    MarshalYAML    func(v any) ([]byte, error)
    NewYAMLEncoder func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder
}

// =============================================================================
// Helpers
// =============================================================================

// NewShims creates a new Shims instance with default implementations
func NewShims() *Shims {
    return &Shims{
        Setenv:        os.Setenv,
        UnmarshalJSON: json.Unmarshal,
        UserHomeDir:   os.UserHomeDir,
        MkdirAll:      os.MkdirAll,
        WriteFile:     os.WriteFile,
        Rename:        os.Rename,
        GOARCH:        func() string { return runtime.GOARCH },
        NumCPU:        func() int { return runtime.NumCPU() },
        VirtualMemory: mem.VirtualMemory,
        MarshalYAML:   yaml.Marshal,
        NewYAMLEncoder: func(w io.Writer, opts ...yaml.EncodeOption) YAMLEncoder {
            return yaml.NewEncoder(w, opts...)
        },
    }
}
```

## Best Practices

1. Use shims for system calls
2. Keep methods focused and small
3. Document at the package and type level
4. Use BDD style in tests
5. Maintain clear separation between interface and implementation
6. Use dependency injection
7. Keep test setup clean and reusable
8. Use section headers for code organization
9. Document public APIs thoroughly
10. Keep implementation details private 
