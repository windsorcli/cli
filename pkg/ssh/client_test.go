package ssh

import (
	"fmt"
	"os"
	"testing"
)

var privateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABFwAAAAdzc2gtcn
NhAAAAAwEAAQAAAQEAzRbWmvX0VNMiWpzIeo3ewv029doibmpXl1C+kB3IK2XqWqwyZi8J
pRqMJN9wye5hBP+lXyZfxl4d2BbFNc0Az0rjw5f+6i2vF4bD1EdYO0DBHWRxXC2ARVSEaf
1RCyKfnJbyUfRpRsewfdUMizOAUhJUPl+/RFUfFnXF0CwmzfYi7vVUYWhrnDnrfk2eZ71C
e+S6w3SeN7IA9Uj9IoqaTYmnJ7xfCcCfRXNHd8ykMj0KiEJXdJovnFK86sKoBqEMvVV7A9
D8sH87ZKLhjE8NP6X2TjD5lZe3sC65Adq9WLm6CHtI/kw+7KAxRnQTatoolWX/RUp9ge2g
Zn+DnNeOJwAAA8goZXJCKGVyQgAAAAdzc2gtcnNhAAABAQDNFtaa9fRU0yJanMh6jd7C/T
b12iJualeXUL6QHcgrZeparDJmLwmlGowk33DJ7mEE/6VfJl/GXh3YFsU1zQDPSuPDl/7q
La8XhsPUR1g7QMEdZHFcLYBFVIRp/VELIp+clvJR9GlGx7B91QyLM4BSElQ+X79EVR8Wdc
XQLCbN9iLu9VRhaGucOet+TZ5nvUJ75LrDdJ43sgD1SP0iippNiacnvF8JwJ9Fc0d3zKQy
PQqIQld0mi+cUrzqwqgGoQy9VXsD0PywfztkouGMTw0/pfZOMPmVl7ewLrkB2r1YuboIe0
j+TD7soDFGdBNq2iiVZf9FSn2B7aBmf4Oc144nAAAAAwEAAQAAAQB8Vs1Tc6xnRP49+3Hc
Q2j7xLLuiQp48MYb+hsemr/B9+8GfAGuS/RIAflXXZQvCPQPKMLlFgnY5TSozt1PifNkud
2uttcYuQu/crgFWh/XBKJQJJZJsVhkMCJ7c9YPrzUfpbBSGaE+BVEuaN1LA7VXjL9AdaIr
VoQbhNmiJTJ9iRZNqykqZypCPjJL2SLGmtIZx046ESMmxJjxZpxYQgUw7OnSrmhLeHjZxj
VmsNEC2X+mfWQ648jfwguSurqc+pC0ZcWf4xxe4HKD3+2m7EHWRhyyYkoTJexazseYFiy9
jaZj603aDPL0j6urNXAhUXlTqojAqwY2t66mOZHg3gthAAAAgBENmZNPkazwyv+UnCl5gZ
r8LXrMErYMyVLJ481Z0UFcnsmda9zkwZzDmoELzsDng9X5CeEtkmsDHh9ErTEWIw5c4eg/
2RIF0VdQ+CuIs7N7PcRdTDMAPSko5lBjplxk0TuJBo+x3gyVuFTXGPEkHzU8swZ1c8AbWM
txd0769PmKAAAAgQD9mrkWLph00qJxafWEjEOOgLHKTPieCxQcbkNo022hKQzGlcC/pMmV
5PdHjX7uG2VX7KpBM7TSpgE6ZORHWlXGEogrmgmdMTw8gtElqPaI28C2mMCHRqvgRgPYyC
GKdg9hLp3m7LaHe86W+kyGblyEgG/9jf+uAytQO1OH/eqf0QAAAIEAzwbLOIZm3IXsn8pE
H6hfP/2JAUUuHb+QZX2chxoKLrkRvplQRWFDAq/9nnPY8/n5gDyG+eIUn8XNlk+T88Zzle
Wn4fSEjQr1zKgjfGFb0u75fjPlb4j0FC4x8p1cDacss82k9OpZI64P3CpFIN4lkuJ0gy/Z
SRbYzac7Ad/IBHcAAAAQcnlhbnZhbmd1bmR5QE1hYwECAw==
-----END OPENSSH PRIVATE KEY-----`

func TestBaseClient_SetClientConfig(t *testing.T) {
	t.Run("SetUserConfig", func(t *testing.T) {
		client := &BaseClient{}
		config := &ClientConfig{
			User: "testuser",
		}
		client.SetClientConfig(config)
		if client.clientConfig != config {
			t.Fatalf("Expected clientConfig to be set")
		}
	})

	t.Run("SetEmptyConfig", func(t *testing.T) {
		client := &BaseClient{}
		config := &ClientConfig{}
		client.SetClientConfig(config)
		if client.clientConfig != config {
			t.Fatalf("Expected clientConfig to be set")
		}
	})

	t.Run("SetConfigFilePath", func(t *testing.T) {
		client := &BaseClient{}
		configPath := "/path/to/config"

		// Mock functions
		stat = func(_ string) (os.FileInfo, error) {
			return nil, nil // Simulate that the file exists
		}
		readFile = func(name string) ([]byte, error) {
			if name == configPath {
				return []byte(`
Host localhost
  User testuser
  IdentityFile /path/to/id_rsa
  Hostname localhost
  Port 22
`), nil
			}
			if name == "/path/to/id_rsa" {
				return []byte(privateKey), nil
			}
			return nil, os.ErrNotExist
		}

		err := client.SetClientConfigFile(configPath, "localhost")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if client.clientConfig == nil {
			t.Fatal("Expected clientConfig to be set")
		}
		if client.clientConfig.User != "testuser" {
			t.Fatalf("Expected User to be 'testuser', got %v", client.clientConfig.User)
		}
		if client.clientConfig.HostName != "localhost" {
			t.Fatalf("Expected HostName to be 'localhost', got %v", client.clientConfig.HostName)
		}
		if client.clientConfig.Port != "22" {
			t.Fatalf("Expected Port to be '22', got %v", client.clientConfig.Port)
		}
		if len(client.clientConfig.Auth) == 0 {
			t.Fatal("Expected Auth to be set")
		}
	})

	t.Run("ErrorReadingConfigFile", func(t *testing.T) {
		client := &BaseClient{}
		configPath := "/path/to/config"

		// Mock functions
		stat = func(_ string) (os.FileInfo, error) {
			return nil, nil // Simulate that the file exists
		}
		readFile = func(name string) ([]byte, error) {
			if name == configPath {
				return nil, fmt.Errorf("simulated read file error")
			}
			if name == "/path/to/id_rsa" {
				return []byte(privateKey), nil
			}
			return nil, os.ErrNotExist
		}

		err := client.SetClientConfigFile(configPath, "localhost")
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
		if client.clientConfig != nil {
			t.Fatal("Expected clientConfig to be nil")
		}
	})
}

func TestBaseClient_SetClientConfigFile(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		client := &BaseClient{}
		configStr := `
Host localhost
  User testuser
  IdentityFile /path/to/id_rsa
  Hostname localhost
  Port 22
`
		// Mock functions
		stat = func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist // Simulate that the file does not exist
		}
		readFile = func(name string) ([]byte, error) {
			if name == "/path/to/id_rsa" || name == "id_rsa" {
				return []byte(privateKey), nil
			}
			return nil, os.ErrNotExist
		}

		err := client.SetClientConfigFile(configStr, "localhost")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if client.clientConfig == nil {
			t.Fatal("Expected clientConfig to be set")
		}
	})

	t.Run("InvalidConfig", func(t *testing.T) {
		client := &BaseClient{}
		configStr := "invalid config"

		// Mock functions
		stat = func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		readFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		err := client.SetClientConfigFile(configStr, "localhost")
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
	})
}

func TestBaseClient_parseSSHConfig(t *testing.T) {
	t.Run("SuccessfulParse", func(t *testing.T) {
		configStr := `
Host localhost
  User testuser
  IdentityFile /path/to/id_rsa
  Hostname localhost
  Port 22
`
		// Mock functions
		readFile = func(name string) ([]byte, error) {
			if name == "/path/to/id_rsa" || name == "id_rsa" {
				return []byte(privateKey), nil
			}
			return nil, os.ErrNotExist
		}

		clientConfig, err := parseSSHConfig(configStr, "localhost")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if clientConfig == nil {
			t.Fatal("Expected clientConfig to be set")
		}
		if clientConfig.User != "testuser" {
			t.Fatalf("Expected User to be 'testuser', got %v", clientConfig.User)
		}
		if clientConfig.HostName != "localhost" {
			t.Fatalf("Expected HostName to be 'localhost', got %v", clientConfig.HostName)
		}
		if clientConfig.Port != "22" {
			t.Fatalf("Expected Port to be '22', got %v", clientConfig.Port)
		}
		if len(clientConfig.Auth) == 0 {
			t.Fatal("Expected Auth to be set")
		}
	})

	t.Run("FailedParse", func(t *testing.T) {
		_, err := parseSSHConfig("invalid config", "localhost")
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
	})

	t.Run("SingleField", func(t *testing.T) {
		configStr := `
Host localhost
  User
  IdentityFile /path/to/id_rsa
`
		// Mock functions
		readFile = func(name string) ([]byte, error) {
			if name == "/path/to/id_rsa" || name == "id_rsa" {
				return []byte(privateKey), nil
			}
			return nil, os.ErrNotExist
		}

		clientConfig, err := parseSSHConfig(configStr, "localhost")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if clientConfig == nil {
			t.Fatal("Expected clientConfig to be set")
		}
		if len(clientConfig.Auth) == 0 {
			t.Fatal("Expected Auth to be set")
		}
	})

	t.Run("FailedLoadSigner", func(t *testing.T) {
		configStr := `
Host localhost
  IdentityFile /invalid/path/to/id_rsa
`
		// Mock functions
		readFile = func(name string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		_, err := parseSSHConfig(configStr, "localhost")
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
	})
}

func TestBaseClient_LoadSigner(t *testing.T) {
	t.Run("SuccessfulLoad", func(t *testing.T) {
		// Mock functions
		readFile = func(name string) ([]byte, error) {
			if name == "/path/to/id_rsa" || name == "id_rsa" {
				return []byte(privateKey), nil
			}
			return nil, os.ErrNotExist
		}

		signer, err := loadSigner("/path/to/id_rsa")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if signer == nil {
			t.Fatal("Expected a valid signer, got nil")
		}
	})

	t.Run("FailedLoad", func(t *testing.T) {
		_, err := loadSigner("invalid/path")
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
	})

	t.Run("FailedParsePrivateKey", func(t *testing.T) {
		// Mock functions
		readFile = func(_ string) ([]byte, error) {
			return []byte("invalid private key content"), nil
		}

		_, err := loadSigner("/path/to/invalid_id_rsa")
		if err == nil {
			t.Fatal("Expected an error, got nil")
		}
	})
}