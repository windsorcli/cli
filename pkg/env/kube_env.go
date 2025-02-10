package env

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/windsorcli/cli/pkg/di"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeEnvPrinter is a struct that simulates a Kubernetes environment for testing purposes.
type KubeEnvPrinter struct {
	BaseEnvPrinter
}

// NewKubeEnv initializes a new kubeEnv instance using the provided dependency injector.
func NewKubeEnvPrinter(injector di.Injector) *KubeEnvPrinter {
	return &KubeEnvPrinter{
		BaseEnvPrinter: BaseEnvPrinter{
			injector: injector,
		},
	}
}

// GetEnvVars constructs a map of Kubernetes environment variables by setting
// KUBECONFIG and KUBE_CONFIG_PATH based on the configuration root directory.
// It checks for a project-specific volume directory and returns current variables
// if it doesn't exist. If it does, it ensures each PVC directory has a corresponding
// "PV_" environment variable, returning the map if all are accounted for.
func (e *KubeEnvPrinter) GetEnvVars() (map[string]string, error) {
	envVars := make(map[string]string)
	configRoot, err := e.configHandler.GetConfigRoot()
	if err != nil {
		return nil, fmt.Errorf("error retrieving configuration root directory: %w", err)
	}
	kubeConfigPath := filepath.Join(configRoot, ".kube", "config")
	envVars["KUBECONFIG"] = kubeConfigPath
	envVars["KUBE_CONFIG_PATH"] = kubeConfigPath

	projectRoot := os.Getenv("WINDSOR_PROJECT_ROOT")
	volumeDir := filepath.Join(projectRoot, ".volumes")

	if _, err := stat(volumeDir); os.IsNotExist(err) {
		return envVars, nil
	}

	volumeDirs, err := readDir(volumeDir)
	if err != nil {
		return nil, fmt.Errorf("error reading volume directories: %w", err)
	}

	existingEnvVars := make(map[string]string)
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "PV_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				existingEnvVars[parts[0]] = parts[1]
				envVars[parts[0]] = parts[1] // Include existing PV environment variables
			}
		}
	}

	allVolumesAccounted := true
	for _, dir := range volumeDirs {
		if strings.HasPrefix(dir.Name(), "pvc-") {
			found := false
			for _, envVarValue := range existingEnvVars {
				if strings.HasSuffix(dir.Name(), filepath.Base(envVarValue)) {
					found = true
					break
				}
			}
			if !found {
				allVolumesAccounted = false
				break
			}
		}
	}

	if allVolumesAccounted {
		return envVars, nil
	}

	pvcs, _ := queryPersistentVolumeClaims(kubeConfigPath) // ignores error

	if pvcs != nil && pvcs.Items != nil {
		for _, dir := range volumeDirs {
			if strings.HasPrefix(dir.Name(), "pvc-") {
				for _, pvc := range pvcs.Items {
					if strings.HasSuffix(dir.Name(), string(pvc.UID)) {
						envVarName := fmt.Sprintf("PV_%s_%s", sanitizeEnvVar(pvc.Namespace), sanitizeEnvVar(pvc.Name))
						if _, exists := existingEnvVars[envVarName]; !exists {
							envVars[envVarName] = filepath.Join(volumeDir, dir.Name())
						}
						break
					}
				}
			}
		}
	}

	return envVars, nil
}

// Print prints the environment variables for the Kube environment.
func (e *KubeEnvPrinter) Print() error {
	envVars, err := e.GetEnvVars()
	if err != nil {
		// Return the error if GetEnvVars fails
		return fmt.Errorf("error getting environment variables: %w", err)
	}

	// Call the Print method of the embedded BaseEnvPrinter struct with the retrieved environment variables
	return e.BaseEnvPrinter.Print(envVars)
}

// Ensure kubeEnv implements the EnvPrinter interface
var _ EnvPrinter = (*KubeEnvPrinter)(nil)

// sanitizeEnvVar converts a string to uppercase, trims whitespace, and replaces invalid characters with underscores.
func sanitizeEnvVar(input string) string {
	trimmed := strings.TrimSpace(input)
	upper := strings.ToUpper(trimmed)
	re := regexp.MustCompile(`[^A-Z0-9_]`)
	sanitized := re.ReplaceAllString(upper, "_")
	return strings.Trim(sanitized, "_")
}

// queryPersistentVolumeClaims retrieves a list of PersistentVolumeClaims (PVCs) from the Kubernetes cluster.
// It returns a list of PVCs and an error if there is any issue in building the Kubernetes configuration
var queryPersistentVolumeClaims = func(kubeConfigPath string) (*corev1.PersistentVolumeClaimList, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	pvcs, err := clientset.CoreV1().PersistentVolumeClaims("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return pvcs, nil
}
