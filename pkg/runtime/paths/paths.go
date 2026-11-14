// The Paths package resolves the project source directories from the v1alpha2 root file.
// It provides one Resolver that returns the base, terraform, kustomize, and contexts directories as absolute paths.
// It validates the paths block of the root file, including the symbolic link check.
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
	// DefaultTerraformDir is the default name of the terraform source directory.
	DefaultTerraformDir = "terraform"

	// DefaultKustomizeDir is the default name of the kustomize source directory.
	DefaultKustomizeDir = "kustomize"

	// DefaultContextsDir is the default name of the contexts directory.
	DefaultContextsDir = "contexts"

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
	terraform   string
	kustomize   string
	contexts    string
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
// A nil shims skips the symbolic link check. It returns an error for the first rule that p breaks.
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
	r.terraform = filepath.Join(r.base, filepath.FromSlash(valueOr(p.Terraform, DefaultTerraformDir)))
	r.kustomize = filepath.Join(r.base, filepath.FromSlash(valueOr(p.Kustomize, DefaultKustomizeDir)))
	r.contexts = filepath.Join(r.base, filepath.FromSlash(valueOr(p.Contexts, DefaultContextsDir)))
	if err := checkOverlap(r); err != nil {
		return nil, err
	}
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
func (r *Resolver) Terraform() string { return r.terraform }

// Kustomize returns the kustomize source directory.
func (r *Resolver) Kustomize() string { return r.kustomize }

// Contexts returns the contexts directory.
func (r *Resolver) Contexts() string { return r.contexts }

// ContextDir returns the directory of the context called name.
func (r *Resolver) ContextDir(name string) string { return filepath.Join(r.contexts, name) }

// TemplateDir returns the template directory.
func (r *Resolver) TemplateDir() string { return filepath.Join(r.contexts, TemplateDirName) }

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
	if path != DefaultKustomizeDir && !strings.HasPrefix(path, DefaultKustomizeDir+"/") {
		return path
	}
	return r.RelSlash(r.kustomize) + strings.TrimPrefix(path, DefaultKustomizeDir)
}

// IsUnderTerraform reports whether p is the terraform directory or inside it.
func (r *Resolver) IsUnderTerraform(p string) bool { return within(r.terraform, p) }

// IsUnderTemplate reports whether p is the template directory or inside it.
func (r *Resolver) IsUnderTemplate(p string) bool { return within(r.TemplateDir(), p) }

// =============================================================================
// Private Methods
// =============================================================================

// validate applies the rules that need no file system and no other key.
func validate(p *v1alpha2.Paths) error {
	for _, key := range []struct{ name, value string }{
		{"base", p.Base}, {"terraform", p.Terraform}, {"kustomize", p.Kustomize}, {"contexts", p.Contexts},
	} {
		if key.value == "" {
			continue
		}
		if err := validateValue(key.name, key.value); err != nil {
			return err
		}
	}
	if p.Base != "" && cleanRel(p.Base) == "." {
		return fmt.Errorf("paths.base resolves to the project root. Remove it, because the default already is the project root")
	}
	for _, key := range []struct{ name, value string }{
		{"terraform", p.Terraform}, {"kustomize", p.Kustomize}, {"contexts", p.Contexts},
	} {
		if key.value != "" && cleanRel(key.value) == "." {
			return fmt.Errorf("paths.%s resolves to paths.base itself. Name a directory inside it", key.name)
		}
	}
	return nil
}

// validateValue rejects an absolute value, a value with a .. element, and a value inside .windsor.
func validateValue(name, value string) error {
	slashed := strings.ReplaceAll(value, "\\", "/")
	if filepath.IsAbs(value) || strings.HasPrefix(slashed, "/") || (len(slashed) > 1 && slashed[1] == ':') {
		return fmt.Errorf("paths.%s is %q, and the path MUST be relative", name, value)
	}
	for _, element := range strings.Split(slashed, "/") {
		if element == ".." {
			return fmt.Errorf("paths.%s is %q, and the path MUST NOT contain '..'", name, value)
		}
	}
	if cleanRel(value) == WindsorDirName || strings.HasPrefix(cleanRel(value), WindsorDirName+"/") {
		return fmt.Errorf("paths.%s is %q, and the path MUST NOT be inside %s", name, value, WindsorDirName)
	}
	return nil
}

// checkOverlap rejects a source directory that equals or contains another, and any directory
// that sits inside .windsor once base is applied.
func checkOverlap(r *Resolver) error {
	dirs := []struct{ name, path string }{
		{"terraform", r.terraform}, {"kustomize", r.kustomize}, {"contexts", r.contexts},
	}
	windsor := filepath.Join(r.projectRoot, WindsorDirName)
	for i, a := range dirs {
		if within(windsor, a.path) {
			return fmt.Errorf("paths.%s resolves inside %s, and the path MUST NOT", a.name, WindsorDirName)
		}
		for _, b := range dirs[i+1:] {
			if within(a.path, b.path) || within(b.path, a.path) {
				return fmt.Errorf("paths.%s and paths.%s overlap, and the directories MUST NOT contain each other", a.name, b.name)
			}
		}
	}
	if within(windsor, r.base) {
		return fmt.Errorf("paths.base resolves inside %s, and the path MUST NOT", WindsorDirName)
	}
	return nil
}

// checkSymlinks resolves symbolic links in base and the three directories, and fails when one
// resolves outside the resolved project root. A directory that does not exist is checked through
// its nearest existing parent.
func checkSymlinks(r *Resolver, s *Shims) error {
	root, err := s.EvalSymlinks(r.projectRoot)
	if err != nil {
		return nil
	}
	for name, path := range map[string]string{
		"base": r.base, "terraform": r.terraform, "kustomize": r.kustomize, "contexts": r.contexts,
	} {
		resolved, ok := resolveExisting(path, s)
		if ok && !within(root, resolved) {
			return fmt.Errorf("paths.%s resolves through a symbolic link to %s, which is outside the project root", name, resolved)
		}
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

// valueOr returns value, or fallback when value is empty.
func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

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
