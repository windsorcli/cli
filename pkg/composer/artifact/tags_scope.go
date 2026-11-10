// The scoped tag scan reads the artifact tags of the files that a composed blueprint uses.
// It provides ScanTagsScoped, which starts at the blueprint paths of one source.
// It follows the file entries of kustomization files and the local modules of Terraform files.
// It renders nothing and evaluates no Terraform.

package artifact

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/windsorcli/cli/pkg/runtime/terraform"
)

// =============================================================================
// Constants
// =============================================================================

// expressionMarker starts a blueprint expression. A path or a component name that holds it is not resolved.
const expressionMarker = "${"

// scopedWholeDirs are the directories of a source that the scoped scan always reads in full.
// The kustomize/crds directory is the CRD catalog. A cluster can use a catalog entry with no kustomization.
var scopedWholeDirs = []string{"contexts", "_template", "kustomize/crds"}

// kustomizationFileNames are the names of a Kustomize root file.
var kustomizationFileNames = []string{"kustomization.yaml", "kustomization.yml", "Kustomization"}

// =============================================================================
// Types
// =============================================================================

// TagScope lists the paths of one source that a composed blueprint uses.
type TagScope struct {
	Kustomizations []KustomizationScope
	Terraform      []TerraformScope
}

// KustomizationScope is one kustomization of a blueprint. Dir is relative to the source root, such as
// kustomize/telemetry/install. Each component is relative to Dir.
type KustomizationScope struct {
	Name       string
	Dir        string
	Components []string
}

// TerraformScope is one Terraform component of a blueprint. Root is the directory of the source that
// holds the Terraform components, and Path is relative to Root.
type TerraformScope struct {
	Name string
	Root string
	Path string
}

// kustomizationEntries holds the fields of a kustomization file that name other files.
type kustomizationEntries struct {
	Resources             []string            `yaml:"resources"`
	Components            []string            `yaml:"components"`
	Bases                 []string            `yaml:"bases"`
	PatchesStrategicMerge []string            `yaml:"patchesStrategicMerge"`
	Transformers          []string            `yaml:"transformers"`
	Generators            []string            `yaml:"generators"`
	Validators            []string            `yaml:"validators"`
	Patches               []kustomizationFile `yaml:"patches"`
	PatchesJSON6902       []kustomizationFile `yaml:"patchesJson6902"`
	Replacements          []kustomizationFile `yaml:"replacements"`
}

// kustomizationFile is an entry of a kustomization file that can name a file with path.
type kustomizationFile struct {
	Path string `yaml:"path"`
}

// scopedScan holds the state of one ScanTagsScoped call.
type scopedScan struct {
	fsys  fs.FS
	files map[string]bool
	seen  map[string]bool
	notes []string
}

// =============================================================================
// Public Methods
// =============================================================================

// ScanTagsScoped returns the artifact tags in the files of root that scope uses, and notes for the
// operator. It always reads the contexts, _template and kustomize/crds directories. File paths are
// relative to root.
//
// For a kustomization, it reads Dir and each component directory. It does not enter a subdirectory
// that has its own kustomization file, unless an entry reaches it. It follows local entries of the
// fields that name files, also ../ entries, and it reads a file that an entry names. A directory with
// no kustomization file gets a generated one from Flux, so the scan reads all of that directory.
//
// For a Terraform component, it reads the component directory and each local module. It does not
// enter a subdirectory that has .tf files, unless a module block reaches it.
//
// A name that holds an expression widens the scan and gives a note. A component name widens the scan
// to each directory under Dir. A kustomization path widens it to the kustomize directory, and a
// Terraform path to the Terraform root. A path that the source does not hold gives a note.
func ScanTagsScoped(root string, scope TagScope) ([]ArtifactTag, []string, error) {
	scan := &scopedScan{fsys: os.DirFS(root), files: make(map[string]bool), seen: make(map[string]bool)}
	for _, dir := range scopedWholeDirs {
		if err := scan.addTree(dir, noBoundary); err != nil {
			return nil, nil, err
		}
	}
	for _, k := range scope.Kustomizations {
		if err := scan.addKustomization(k); err != nil {
			return nil, nil, err
		}
	}
	for _, t := range scope.Terraform {
		if err := scan.addTerraform(t); err != nil {
			return nil, nil, err
		}
	}
	tags, err := scan.readTags()
	if err != nil {
		return nil, nil, err
	}
	return tags, scan.notes, nil
}

// =============================================================================
// Private Methods
// =============================================================================

// addKustomization adds the files of one kustomization and of each of its components.
func (s *scopedScan) addKustomization(k KustomizationScope) error {
	if strings.Contains(k.Dir, expressionMarker) {
		s.notef("Kustomization %q has the path %q, which the CLI cannot resolve, so the scan reads the whole kustomize directory.", k.Name, k.Dir)
		return s.addEveryKustomizeDir("kustomize")
	}
	dir := path.Clean(k.Dir)
	exists, err := s.isDir(dir)
	if err != nil {
		return err
	}
	if !exists {
		s.notef("Kustomization %q has the path %q, which is not in the source, so the scan reads no files for it.", k.Name, k.Dir)
		return nil
	}
	if err := s.addKustomizeDir(dir); err != nil {
		return err
	}
	for _, component := range k.Components {
		if component == "" {
			continue
		}
		if strings.Contains(component, expressionMarker) {
			s.notef("Kustomization %q has the component %q, which the CLI cannot resolve, so the scan reads each directory under %q.", k.Name, component, dir)
			if err := s.addEveryKustomizeDir(dir); err != nil {
				return err
			}
			continue
		}
		componentDir := path.Join(dir, component)
		exists, err := s.isDir(componentDir)
		if err != nil {
			return err
		}
		if !exists {
			s.notef("Kustomization %q has the component %q, which is not in the source, so the scan reads no files for it.", k.Name, component)
			continue
		}
		if err := s.addKustomizeDir(componentDir); err != nil {
			return err
		}
	}
	return nil
}

// addEveryKustomizeDir adds each file under dir, and follows the entries of each kustomization file under dir.
func (s *scopedScan) addEveryKustomizeDir(dir string) error {
	if err := s.addTree(dir, noBoundary); err != nil {
		return err
	}
	return fs.WalkDir(s.fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return ignoreNotExist(err)
		}
		if d.IsDir() && s.hasKustomization(p) {
			return s.addKustomizeDir(p)
		}
		return nil
	})
}

// addKustomizeDir adds the files of dir and follows the entries of its kustomization file. A directory
// with no kustomization file adds each file under it. A kustomization file that the scan cannot parse
// gives a note, and the note leaves out the content of the file.
func (s *scopedScan) addKustomizeDir(dir string) error {
	if s.seen[dir] {
		return nil
	}
	s.seen[dir] = true
	if err := s.addTree(dir, s.hasKustomization); err != nil {
		return err
	}
	file := s.kustomizationFile(dir)
	if file == "" {
		return s.addEveryKustomizeDir(dir)
	}
	content, err := fs.ReadFile(s.fsys, file)
	if err != nil {
		return err
	}
	var doc kustomizationEntries
	if err := yaml.Unmarshal(content, &doc); err != nil {
		s.notef("The scan cannot parse %q, so it does not follow the entries of that file: %s", file, yaml.FormatError(err, false, false))
		return nil
	}
	for _, entry := range doc.all() {
		if err := s.followEntry(dir, file, entry); err != nil {
			return err
		}
	}
	return nil
}

// followEntry adds the directory or the file that an entry of file names. A remote entry and an
// inline YAML entry add nothing. A local entry that the source does not hold gives a note.
func (s *scopedScan) followEntry(dir, file, entry string) error {
	if isRemoteKustomizeEntry(entry) || isInlineKustomizeEntry(entry) {
		return nil
	}
	target := path.Join(dir, entry)
	info, err := s.stat(target)
	if err != nil {
		return err
	}
	switch {
	case info == nil:
		s.notef("%q names %q, which is not in the source, so the scan reads no files for it.", file, entry)
	case info.IsDir():
		return s.addKustomizeDir(target)
	case hasTagExtension(target):
		s.files[target] = true
	}
	return nil
}

// addTerraform adds the files of one Terraform component and of its local modules.
func (s *scopedScan) addTerraform(t TerraformScope) error {
	if strings.Contains(t.Path, expressionMarker) {
		root := path.Clean(t.Root)
		exists, err := s.isDir(root)
		if err != nil {
			return err
		}
		if !exists {
			s.notef("Terraform component %q has the path %q, and the Terraform root %q is not in the source, so the scan reads no files for it.", t.Name, t.Path, t.Root)
			return nil
		}
		s.notef("Terraform component %q has the path %q, which the CLI cannot resolve, so the scan reads the whole %s directory.", t.Name, t.Path, t.Root)
		return s.addEveryTerraformDir(root)
	}
	dir := path.Join(t.Root, t.Path)
	exists, err := s.isDir(dir)
	if err != nil {
		return err
	}
	if !exists {
		s.notef("Terraform component %q has the path %q, which is not in the source, so the scan reads no files for it.", t.Name, t.Path)
		return nil
	}
	return s.addTerraformDir(dir)
}

// addEveryTerraformDir adds each file under dir, and follows the local modules of each Terraform directory under dir.
func (s *scopedScan) addEveryTerraformDir(dir string) error {
	if err := s.addTree(dir, noBoundary); err != nil {
		return err
	}
	return fs.WalkDir(s.fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return ignoreNotExist(err)
		}
		if d.IsDir() && s.hasTerraform(p) {
			return s.addTerraformDir(p)
		}
		return nil
	})
}

// addTerraformDir adds the files of dir and follows the local module blocks of its .tf files.
func (s *scopedScan) addTerraformDir(dir string) error {
	if s.seen[dir] {
		return nil
	}
	s.seen[dir] = true
	if err := s.addTree(dir, s.hasTerraform); err != nil {
		return err
	}
	entries, err := fs.ReadDir(s.fsys, dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}
		file := path.Join(dir, entry.Name())
		content, err := fs.ReadFile(s.fsys, file)
		if err != nil {
			return err
		}
		for _, source := range s.moduleSources(file, content) {
			if err := s.followModule(dir, file, source); err != nil {
				return err
			}
		}
	}
	return nil
}

// moduleSources returns the literal source of each module block in the content of a .tf file.
// A file that the scan cannot parse, or a source that is not a string, gives a note.
func (s *scopedScan) moduleSources(file string, content []byte) []string {
	modules, err := terraform.ParseModuleSources(file, content)
	if err != nil {
		s.notef("The scan cannot parse %q, so it does not follow the modules of that file: %v", file, err)
		return nil
	}
	var sources []string
	for _, module := range modules {
		if !module.Literal {
			s.notef("Module %q in %q has a source that is not a string, so the scan does not follow it.", module.Name, file)
			continue
		}
		sources = append(sources, module.Source)
	}
	return sources
}

// followModule adds the module directory that source names. A source that is not local adds nothing.
// A local source that the source tree does not hold gives a note.
func (s *scopedScan) followModule(dir, file, source string) error {
	if !strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "../") {
		return nil
	}
	target := path.Join(dir, source)
	info, err := s.stat(target)
	if err != nil {
		return err
	}
	if info == nil || !info.IsDir() {
		s.notef("%q names the module %q, which is not in the source, so the scan reads no files for it.", file, source)
		return nil
	}
	return s.addTerraformDir(target)
}

// addTree adds each tag file under dir. It does not enter a subdirectory of dir for which boundary
// returns true. A missing dir adds nothing.
func (s *scopedScan) addTree(dir string, boundary func(string) bool) error {
	return fs.WalkDir(s.fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return ignoreNotExist(err)
		}
		if d.IsDir() {
			if d.Name() == ".terraform" || d.Name() == ".git" {
				return fs.SkipDir
			}
			if p != dir && boundary(p) {
				return fs.SkipDir
			}
			return nil
		}
		if hasTagExtension(p) {
			s.files[p] = true
		}
		return nil
	})
}

// readTags returns the artifact tags of the added files, sorted by file and line.
func (s *scopedScan) readTags() ([]ArtifactTag, error) {
	files := make([]string, 0, len(s.files))
	for file := range s.files {
		files = append(files, file)
	}
	sort.Strings(files)
	var tags []ArtifactTag
	for _, file := range files {
		content, err := fs.ReadFile(s.fsys, file)
		if err != nil {
			return nil, err
		}
		found, err := ScanTags(file, content)
		if err != nil {
			return nil, err
		}
		tags = append(tags, found...)
	}
	return tags, nil
}

// kustomizationFile returns the path of the kustomization file of dir, or an empty string when it has none.
func (s *scopedScan) kustomizationFile(dir string) string {
	for _, name := range kustomizationFileNames {
		file := path.Join(dir, name)
		if info, err := s.stat(file); err == nil && info != nil && !info.IsDir() {
			return file
		}
	}
	return ""
}

// hasKustomization reports whether dir has a kustomization file.
func (s *scopedScan) hasKustomization(dir string) bool {
	return s.kustomizationFile(dir) != ""
}

// hasTerraform reports whether dir has a .tf file.
func (s *scopedScan) hasTerraform(dir string) bool {
	entries, err := fs.ReadDir(s.fsys, dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tf") {
			return true
		}
	}
	return false
}

// isDir reports whether p is a directory of the source.
func (s *scopedScan) isDir(p string) (bool, error) {
	info, err := s.stat(p)
	if err != nil {
		return false, err
	}
	return info != nil && info.IsDir(), nil
}

// stat returns the file information of p. It returns nil with no error when p is not in the source,
// also when p leaves the source root.
func (s *scopedScan) stat(p string) (fs.FileInfo, error) {
	if !fs.ValidPath(p) {
		return nil, nil
	}
	info, err := fs.Stat(s.fsys, p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return info, nil
}

// notef adds one note for the operator.
func (s *scopedScan) notef(format string, args ...any) {
	s.notes = append(s.notes, fmt.Sprintf(format, args...))
}

// =============================================================================
// Helpers
// =============================================================================

// all returns each entry of e that names a file or a directory.
func (e kustomizationEntries) all() []string {
	entries := slices.Concat(e.Resources, e.Components, e.Bases, e.PatchesStrategicMerge, e.Transformers, e.Generators, e.Validators)
	for _, file := range slices.Concat(e.Patches, e.PatchesJSON6902, e.Replacements) {
		if file.Path != "" {
			entries = append(entries, file.Path)
		}
	}
	return entries
}

// noBoundary is a boundary function that lets a walk enter each subdirectory.
func noBoundary(string) bool {
	return false
}

// ignoreNotExist returns nil for an error that says a file does not exist, and err for another error.
func ignoreNotExist(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// isInlineKustomizeEntry reports whether an entry holds YAML and does not name a file.
func isInlineKustomizeEntry(entry string) bool {
	return strings.Contains(entry, "\n") || strings.Contains(entry, ": ")
}

// isRemoteKustomizeEntry reports whether a resources or components entry names a remote target.
func isRemoteKustomizeEntry(entry string) bool {
	return strings.Contains(entry, "://") || strings.Contains(entry, "?ref=") || strings.HasPrefix(entry, "git@")
}
