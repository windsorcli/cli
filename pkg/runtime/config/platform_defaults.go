package config

import "fmt"

// ApplyPlatformDefaults applies platform profile values using either force or fill-missing mode.
// In fill-missing mode, cluster.driver is set only when empty and cloud enabled flags are set
// only when the corresponding key is unset.
func ApplyPlatformDefaults(handler ConfigHandler, platformOverride string, onlyIfMissing bool) error {
	platform := platformOverride
	if platform == "" {
		platform = handler.GetString("platform")
	}
	if platform == "" {
		platform = handler.GetString("provider")
	}
	setBool := func(key string, value bool) error {
		if err := handler.Set(key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
		return nil
	}
	if onlyIfMissing {
		setBool = func(key string, value bool) error {
			if handler.Get(key) != nil {
				return nil
			}
			if err := handler.Set(key, value); err != nil {
				return fmt.Errorf("failed to set %s: %w", key, err)
			}
			return nil
		}
	}
	setString := func(key, value string) error {
		if err := handler.Set(key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
		return nil
	}
	if onlyIfMissing {
		setString = func(key, value string) error {
			if handler.GetString(key) != "" {
				return nil
			}
			if err := handler.Set(key, value); err != nil {
				return fmt.Errorf("failed to set %s: %w", key, err)
			}
			return nil
		}
	}

	if platform != "" {
		switch platform {
		case "docker", "incus", "metal", "omni":
			if err := setString("cluster.driver", "talos"); err != nil {
				return err
			}
		case "aws":
			if err := setString("cluster.driver", "eks"); err != nil {
				return err
			}
			if err := setBool("aws.enabled", true); err != nil {
				return err
			}
		case "azure":
			if err := setString("cluster.driver", "aks"); err != nil {
				return err
			}
			if err := setBool("azure.enabled", true); err != nil {
				return err
			}
		case "gcp":
			if err := setString("cluster.driver", "gke"); err != nil {
				return err
			}
			if err := setBool("gcp.enabled", true); err != nil {
				return err
			}
		}
	}

	return nil
}
