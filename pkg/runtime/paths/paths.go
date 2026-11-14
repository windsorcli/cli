// The Paths package resolves the project source directories from the v1alpha2 root file.
// It provides one Resolver that returns the base, terraform, kustomize, and contexts directories as absolute paths.
// It validates paths.base, including the symbolic link check.
// The .windsor directory and windsor.yaml stay at the project root and are not part of the resolver.

package paths

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/internal/shims"
	"github.com/windsorcli/cli/internal/werror"
)

// =============================================================================
// Constants
// =============================================================================

const (
	// TerraformDirName is the name of the terraform source directory under base.
	TerraformDirName = "terraform"

	// KustomizeDirName is the name of the kustomize source directory under base.
	KustomizeDirName = "kustomize"

	// ContextsDirName is the name of the contexts directory under base.
	ContextsDirName = "contexts"

	// TemplateDirName is the name of the template directory inside the contexts directory.
	TemplateDirName = "_template"

	// WindsorDirName is the name of the CLI state directory at the project root.
	WindsorDirName = ".windsor"

	// RootFileName is the name of the root file at the project root.
	RootFileName = "windsor.yaml"
)

// =============================================================================
// Types
// =============================================================================

// Shims holds the system calls of the package.
type Shims struct {
	shims.Shims
	EvalSymlinks func(path string) (string, error)
}

// Resolver holds the absolute project source directories.
type Resolver struct {
	projectRoot string
	base        string
}

// =============================================================================
// Constructor
// =============================================================================

// NewShims returns Shims with the real implementations.
func NewShims() *Shims {
	return &Shims{Shims: *shims.NewShims(), EvalSymlinks: filepath.EvalSymlinks}
}

// Default returns the resolver of a project with no paths block.
func Default(projectRoot string) *Resolver {
	r, _ := New(projectRoot, nil, nil)
	return r
}

// New validates p and returns the resolver for projectRoot. A nil p gives the default layout.
// A nil shims skips the symbolic link check. It returns an error when p.Base breaks a rule.
func New(projectRoot string, p *v1alpha2.Paths, s *Shims) (*Resolver, error) {
	if p == nil {
		p = &v1alpha2.Paths{}
	}
	if err := validate(p); err != nil {
		return nil, err
	}
	base := filepath.FromSlash(p.Base)
	r := &Resolver{projectRoot: projectRoot}
	r.base = filepath.Join(projectRoot, base)
	if s != nil && s.EvalSymlinks != nil {
		if err := checkSymlinks(r, s); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Load reads the root file in projectRoot and returns its resolver. A missing root file, a v1alpha1
// root file, and a v1alpha2 root file with no paths block give the default layout. It returns an
// error when the file cannot be decoded or when the paths block breaks a rule.
func Load(projectRoot string, s *Shims) (*Resolver, error) {
	if s == nil {
		s = NewShims()
	}
	rootPath := filepath.Join(projectRoot, RootFileName)
	data, err := s.ReadFile(rootPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return New(projectRoot, nil, s)
		}
		return nil, werror.Wrap(err, "error reading root config file")
	}
	var root v1alpha2.Root
	if err := s.YamlUnmarshal(data, &root); err != nil {
		return nil, werror.Wrap(err, "error unmarshalling root config")
	}
	if root.Version != v1alpha2.Version {
		return New(projectRoot, nil, s)
	}
	r, err := New(projectRoot, root.Paths, s)
	if err != nil {
		return nil, werror.New(fmt.Sprintf("The paths block of %s is invalid. %s", rootPath, err), "Fix the paths block of the root file.", err)
	}
	return r, nil
}

// =============================================================================
// Public Methods
// =============================================================================

// ProjectRoot returns the project root.
func (r *Resolver) ProjectRoot() string { return r.projectRoot }

// Base returns the directory that holds the three source directories.
func (r *Resolver) Base() string { return r.base }

// Terraform returns the terraform source directory.
func (r *Resolver) Terraform() string { return filepath.Join(r.base, TerraformDirName) }

// Kustomize returns the kustomize source directory.
func (r *Resolver) Kustomize() string { return filepath.Join(r.base, KustomizeDirName) }

// Contexts returns the contexts directory.
func (r *Resolver) Contexts() string { return filepath.Join(r.base, ContextsDirName) }

// ContextDir returns the directory of the context called name.
func (r *Resolver) ContextDir(name string) string { return filepath.Join(r.Contexts(), name) }

// TemplateDir returns the template directory.
func (r *Resolver) TemplateDir() string { return filepath.Join(r.Contexts(), TemplateDirName) }

// RelSlash returns abs relative to the project root, with forward slashes. It returns abs
// unchanged when abs is outside the project root.
func (r *Resolver) RelSlash(abs string) string {
	rel, err := filepath.Rel(r.projectRoot, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return abs
	}
	return filepath.ToSlash(rel)
}

// RebaseKustomize maps a path under the standard kustomize directory to the same path under the
// configured kustomize directory, relative to the project root with forward slashes. A path
// outside the standard directory is returned unchanged.
func (r *Resolver) RebaseKustomize(path string) string {
	path = filepath.ToSlash(path)
	if path != KustomizeDirName && !strings.HasPrefix(path, KustomizeDirName+"/") {
		return path
	}
	return r.RelSlash(r.Kustomize()) + strings.TrimPrefix(path, KustomizeDirName)
}

// IsUnderTerraform reports whether p is the terraform directory or inside it.
func (r *Resolver) IsUnderTerraform(p string) bool { return within(r.Terraform(), p) }

// IsUnderTemplate reports whether p is the template directory or inside it.
func (r *Resolver) IsUnderTemplate(p string) bool { return within(r.TemplateDir(), p) }

// =============================================================================
// Private Methods
// =============================================================================

// validate rejects an empty, absolute, or .. base, a base inside .windsor, and a base that is the project root.
func validate(p *v1alpha2.Paths) error {
	if p.Base == "" {
		return nil
	}
	slashed := strings.ReplaceAll(p.Base, "\\", "/")
	if filepath.IsAbs(p.Base) || strings.HasPrefix(slashed, "/") || (len(slashed) > 1 && slashed[1] == ':') {
		return fmt.Errorf("paths.base is %q, and the path MUST be relative", p.Base)
	}
	for _, element := range strings.Split(slashed, "/") {
		if element == ".." {
			return fmt.Errorf("paths.base is %q, and the path MUST NOT contain '..'", p.Base)
		}
	}
	cleaned := cleanRel(p.Base)
	if cleaned == WindsorDirName || strings.HasPrefix(cleaned, WindsorDirName+"/") {
		return fmt.Errorf("paths.base is %q, and the path MUST NOT be inside %s", p.Base, WindsorDirName)
	}
	if cleaned == "." {
		return fmt.Errorf("paths.base resolves to the project root. Remove it, because the default already is the project root")
	}
	return nil
}

// checkSymlinks resolves symbolic links in base, and fails when base resolves outside the resolved
// project root. A base that does not exist is checked through its nearest existing parent.
func checkSymlinks(r *Resolver, s *Shims) error {
	root, err := s.EvalSymlinks(r.projectRoot)
	if err != nil {
		return nil
	}
	resolved, ok := resolveExisting(r.base, s)
	if ok && !within(root, resolved) {
		return fmt.Errorf("paths.base resolves through a symbolic link to %s, which is outside the project root", resolved)
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// cleanRel cleans a relative path and returns it with forward slashes.
func cleanRel(p string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(p, "\\", "/"))))
}

// within reports whether p equals dir or is inside it.
func within(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// resolveExisting resolves the nearest existing parent of path and appends the missing tail.
func resolveExisting(path string, s *Shims) (string, bool) {
	tail := ""
	current := path
	for {
		resolved, err := s.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(resolved, tail), true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		tail = filepath.Join(filepath.Base(current), tail)
		current = parent
	}
}
