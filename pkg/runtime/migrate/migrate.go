// The Migrate package converts a v1alpha1 project into the v1alpha2 layout.
// It provides Plan, which reads the v1alpha1 sources and reports the moves and the conflicts, and Apply, which writes them.
// It also provides LegacyFiles, which lists the v1alpha1 files that a project still holds.
// The package never touches the .windsor directory.

package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/windsorcli/cli/api/config/v1alpha2"
	"github.com/windsorcli/cli/internal/shims"
	"github.com/windsorcli/cli/internal/werror"
	"github.com/windsorcli/cli/pkg/runtime/config"
	"github.com/windsorcli/cli/pkg/runtime/paths"
)

// =============================================================================
// Types
// =============================================================================

// Conflict is a key that holds different values in a v1alpha1 source and in values.yaml.
type Conflict struct {
	Context     string
	KeyPath     string
	SourceValue any
	SourceFile  string
	ValuesValue any
	ValuesFile  string
}

// FileWrite is a file that Apply writes, with the planned content.
type FileWrite struct {
	Path string
	Data []byte
}

// Plan is the result of reading a project. It lists the files to write and delete and the
// conflicts that stop the migration. A plan with conflicts writes nothing.
type Plan struct {
	Writes    []FileWrite
	Deletes   []string
	Conflicts []Conflict
}

// Migrator reads and writes the files of a project through its shims.
type Migrator struct {
	shims *shims.Shims
}

// =============================================================================
// Constructor
// =============================================================================

// NewMigrator creates a Migrator. A nil argument selects the real file system.
func NewMigrator(s *shims.Shims) *Migrator {
	if s == nil {
		s = shims.NewShims()
	}
	return &Migrator{shims: s}
}

// =============================================================================
// Public Methods
// =============================================================================

// Plan reads the v1alpha1 sources of the project and compares them with each values.yaml.
// For each context it merges the root contexts.<name> map and then contexts/<name>/windsor.yaml.
// It maps provider to platform, and copies a root terraform.driver into every context.
// A key that only the sources hold is added to values.yaml. A key with a different value is a conflict.
// A root key other than version, environment, paths, contexts, and terraform.driver fails the plan.
func (m *Migrator) Plan(projectRoot string) (*Plan, error) {
	rootPath := filepath.Join(projectRoot, "windsor.yaml")
	root, hasRoot, err := m.readMap(rootPath)
	if err != nil {
		return nil, err
	}

	var driver any
	if hasRoot {
		if driver, err = rootDriver(rootPath, root); err != nil {
			return nil, err
		}
	}
	rootContexts, err := rootContextMap(rootPath, root)
	if err != nil {
		return nil, err
	}

	names, err := m.contextNames(projectRoot, rootContexts)
	if err != nil {
		return nil, err
	}

	plan := &Plan{}
	for _, name := range names {
		if err := m.planContext(projectRoot, rootPath, name, rootContexts[name], driver, plan); err != nil {
			return nil, err
		}
	}

	if hasRoot && len(plan.Conflicts) == 0 {
		if data, changed, err := rewriteRoot(root); err != nil {
			return nil, err
		} else if changed {
			plan.Writes = append(plan.Writes, FileWrite{Path: rootPath, Data: data})
		}
	}
	return plan, nil
}

// Apply writes the files of a plan and deletes the context windsor.yaml files. It writes the root
// file last, so that a failure leaves a project that migrate can read again. A plan with
// conflicts writes nothing and returns an error.
func (m *Migrator) Apply(plan *Plan) error {
	if len(plan.Conflicts) > 0 {
		return werror.New("The project has conflicts.", "Resolve the conflicts, then run 'windsor migrate' again.", nil)
	}
	for _, w := range plan.Writes {
		if err := m.shims.MkdirAll(filepath.Dir(w.Path), 0755); err != nil {
			return werror.Wrap(err, "error creating directory for %s", w.Path)
		}
		if err := m.shims.WriteFile(w.Path, w.Data, 0644); err != nil {
			return werror.Wrap(err, "error writing %s", w.Path)
		}
	}
	for _, path := range plan.Deletes {
		if err := m.shims.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return werror.Wrap(err, "error deleting %s", path)
		}
	}
	return nil
}

// LegacyFiles lists the v1alpha1 files of a project. They are the root windsor.yaml when its
// version is v1alpha1 or missing, and each contexts/<name>/windsor.yaml or windsor.yml.
// It returns nil when the project holds none.
func (m *Migrator) LegacyFiles(projectRoot string) []string {
	var files []string
	rootPath := filepath.Join(projectRoot, "windsor.yaml")
	if root, ok, err := m.readMap(rootPath); err == nil && ok {
		if version, has := root["version"]; !has || version == "v1alpha1" {
			files = append(files, rootPath)
		}
	}
	return append(files, m.contextFiles(projectRoot)...)
}

// Empty reports whether the plan has nothing to write or delete and no conflict.
func (p *Plan) Empty() bool {
	return len(p.Writes) == 0 && len(p.Deletes) == 0 && len(p.Conflicts) == 0
}

// Format returns the text that migrate prints for a plan. It lists each conflict with the key path,
// both values, and both file names. Without conflicts it lists the files to write and delete.
// A dry run words the list as a plan.
func (p *Plan) Format(dryRun bool) string {
	var b strings.Builder
	if len(p.Conflicts) > 0 {
		fmt.Fprintf(&b, "Found %d conflict(s). Windsor wrote nothing.\n", len(p.Conflicts))
		for _, c := range p.Conflicts {
			fmt.Fprintf(&b, "\nContext %s, key %s\n  %s: %s\n  %s: %s\n",
				c.Context, c.KeyPath, c.SourceFile, formatValue(c.SourceValue), c.ValuesFile, formatValue(c.ValuesValue))
		}
		b.WriteString("\nResolve each conflict in one of the two files, then run 'windsor migrate' again.\n")
		return b.String()
	}
	if p.Empty() {
		return "Nothing to migrate.\n"
	}
	if dryRun {
		b.WriteString("Dry run. Windsor would make these changes:\n")
	} else {
		b.WriteString("Windsor made these changes:\n")
	}
	for _, w := range p.Writes {
		fmt.Fprintf(&b, "  write   %s\n", w.Path)
	}
	for _, d := range p.Deletes {
		fmt.Fprintf(&b, "  delete  %s\n", d)
	}
	return b.String()
}

// =============================================================================
// Private Methods
// =============================================================================

// planContext adds the writes, deletes, and conflicts of one context to the plan.
func (m *Migrator) planContext(projectRoot, rootPath, name string, rootEntry map[string]any, driver any, plan *Plan) error {
	ctxFile, ctxFileFound := m.contextFilePath(projectRoot, name)
	var ctxValues map[string]any
	if ctxFileFound {
		var err error
		if ctxValues, _, err = m.readMap(ctxFile); err != nil {
			return err
		}
		plan.Deletes = append(plan.Deletes, ctxFile)
	}

	type layer struct {
		values map[string]any
		file   string
	}
	layers := []layer{}
	if driver != nil {
		layers = append(layers, layer{map[string]any{"terraform": map[string]any{"driver": driver}}, rootPath})
	}
	layers = append(layers, layer{rootEntry, rootPath}, layer{ctxValues, ctxFile})

	source := map[string]any{}
	origin := map[string]string{}
	for _, l := range layers {
		if l.values == nil {
			continue
		}
		source = deepMerge(source, l.values)
		recordOrigin("", l.values, l.file, origin)
	}
	mapProvider(source, origin)

	valuesPath := filepath.Join(m.contextsDir(projectRoot), name, "values.yaml")
	existing, valuesExist, err := m.readMap(valuesPath)
	if err != nil {
		return err
	}
	if existing == nil {
		existing = map[string]any{}
	}
	_, hadProvider := existing["provider"]
	mapProvider(existing, nil)

	result := deepMerge(map[string]any{}, existing)
	conflicts := compare("", name, source, existing, result, origin, valuesPath)
	plan.Conflicts = append(plan.Conflicts, conflicts...)
	if len(conflicts) > 0 {
		return nil
	}
	if !hadProvider && reflect.DeepEqual(result, existing) {
		return nil
	}
	if len(result) == 0 && !valuesExist {
		return nil
	}
	data, err := config.MarshalValues(result)
	if err != nil {
		return werror.Wrap(err, "error encoding %s", valuesPath)
	}
	plan.Writes = append(plan.Writes, FileWrite{Path: valuesPath, Data: data})
	return nil
}

// readMap reads a YAML file into a map. The boolean is false when the file does not exist.
func (m *Migrator) readMap(path string) (map[string]any, bool, error) {
	if _, err := m.shims.Stat(path); err != nil {
		return nil, false, nil
	}
	data, err := m.shims.ReadFile(path)
	if err != nil {
		return nil, false, werror.Wrap(err, "error reading %s", path)
	}
	var values map[string]any
	if err := m.shims.YamlUnmarshal(data, &values); err != nil {
		return nil, false, werror.Wrap(err, "error parsing %s", path)
	}
	return values, true, nil
}

// contextsDir returns the contexts directory of the project. A root file that gives no valid
// paths block gives the default directory.
func (m *Migrator) contextsDir(projectRoot string) string {
	resolver, err := paths.Load(projectRoot, &paths.Shims{Shims: *m.shims, EvalSymlinks: filepath.EvalSymlinks})
	if err != nil {
		resolver = paths.Default(projectRoot)
	}
	return resolver.Contexts()
}

// contextFilePath returns the windsor.yaml or windsor.yml of a context, when one exists.
func (m *Migrator) contextFilePath(projectRoot, name string) (string, bool) {
	for _, file := range []string{"windsor.yaml", "windsor.yml"} {
		path := filepath.Join(m.contextsDir(projectRoot), name, file)
		if _, err := m.shims.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

// contextFiles lists the context windsor.yaml and windsor.yml files in context name order.
func (m *Migrator) contextFiles(projectRoot string) []string {
	var files []string
	for _, name := range m.contextDirs(projectRoot) {
		if path, ok := m.contextFilePath(projectRoot, name); ok {
			files = append(files, path)
		}
	}
	return files
}

// contextDirs lists the directories under contexts/ that are not templates, in name order.
func (m *Migrator) contextDirs(projectRoot string) []string {
	entries, err := m.shims.ReadDir(m.contextsDir(projectRoot))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), "_") && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// contextNames returns the context directories and the root contexts keys, without duplicates.
func (m *Migrator) contextNames(projectRoot string, rootContexts map[string]map[string]any) ([]string, error) {
	set := map[string]bool{}
	for _, name := range m.contextDirs(projectRoot) {
		set[name] = true
	}
	for name := range rootContexts {
		set[name] = true
	}
	return slices.Sorted(maps.Keys(set)), nil
}

// =============================================================================
// Helpers
// =============================================================================

// rootDriver returns the root terraform.driver. It fails on any other root key that the
// migration cannot place.
func rootDriver(rootPath string, root map[string]any) (any, error) {
	for key := range root {
		switch key {
		case "version", "environment", "paths", "contexts", "terraform":
		default:
			return nil, unmovableKey(rootPath, key)
		}
	}
	terraform, ok := root["terraform"]
	if !ok {
		return nil, nil
	}
	tf, isMap := terraform.(map[string]any)
	if !isMap {
		return nil, unmovableKey(rootPath, "terraform")
	}
	for key := range tf {
		if key != "driver" {
			return nil, unmovableKey(rootPath, "terraform."+key)
		}
	}
	return tf["driver"], nil
}

// unmovableKey returns the error for a root key that the migration cannot place.
func unmovableKey(rootPath, key string) error {
	return werror.New(
		fmt.Sprintf("The root file %s has the key %q. The migration cannot place it.", rootPath, key),
		"Remove the key or move it into a context values.yaml, then run 'windsor migrate' again.", nil)
}

// rootContextMap returns the root contexts map with one map per context.
func rootContextMap(rootPath string, root map[string]any) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	raw, ok := root["contexts"]
	if !ok || raw == nil {
		return out, nil
	}
	contexts, isMap := raw.(map[string]any)
	if !isMap {
		return nil, unmovableKey(rootPath, "contexts")
	}
	for name, entry := range contexts {
		if entry == nil {
			out[name] = map[string]any{}
			continue
		}
		entryMap, isMap := entry.(map[string]any)
		if !isMap {
			return nil, unmovableKey(rootPath, "contexts."+name)
		}
		out[name] = entryMap
	}
	return out, nil
}

// rewriteRoot returns the root file at version v1alpha2 with no contexts or terraform key. It keeps
// environment and paths. The boolean is false when the file already has that content.
func rewriteRoot(root map[string]any) ([]byte, bool, error) {
	_, hasContexts := root["contexts"]
	_, hasTerraform := root["terraform"]
	if root["version"] == v1alpha2.Version && !hasContexts && !hasTerraform {
		return nil, false, nil
	}
	out := yaml.MapSlice{{Key: "version", Value: v1alpha2.Version}}
	for _, key := range []string{"environment", "paths"} {
		if value, ok := root[key]; ok {
			out = append(out, yaml.MapItem{Key: key, Value: value})
		}
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return nil, false, werror.Wrap(err, "error encoding the root file")
	}
	return data, true, nil
}

// mapProvider moves provider to platform in place. A non-empty platform wins. When origin is not
// nil, the platform key takes the origin of the key that supplied its value.
func mapProvider(values map[string]any, origin map[string]string) {
	provider, ok := values["provider"]
	if !ok {
		return
	}
	delete(values, "provider")
	if platform, _ := values["platform"].(string); platform == "" {
		values["platform"] = provider
		if origin != nil {
			origin["platform"] = origin["provider"]
		}
	}
}

// deepMerge returns base with overlay merged in. Overlay wins, maps merge by key, and other values
// replace. It copies the maps it returns and does not change its arguments.
func deepMerge(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		overlayMap, overlayIsMap := v.(map[string]any)
		baseMap, baseIsMap := out[k].(map[string]any)
		if overlayIsMap && baseIsMap {
			out[k] = deepMerge(baseMap, overlayMap)
			continue
		}
		if overlayIsMap {
			out[k] = deepMerge(map[string]any{}, overlayMap)
			continue
		}
		out[k] = v
	}
	return out
}

// recordOrigin records the file of every key path in values. A later call overwrites an earlier one.
func recordOrigin(prefix string, values map[string]any, file string, origin map[string]string) {
	for k, v := range values {
		path := prefix + k
		origin[path] = file
		if nested, ok := v.(map[string]any); ok {
			recordOrigin(path+".", nested, file, origin)
		}
	}
}

// compare walks the source keys against the existing values. It adds each missing key to the
// matching level of result and returns a conflict for each key with a different value. The result
// maps are copies of the existing maps.
func compare(prefix, contextName string, source, existing, result map[string]any, origin map[string]string, valuesFile string) []Conflict {
	var conflicts []Conflict
	for _, key := range slices.Sorted(maps.Keys(source)) {
		sv := source[key]
		path := prefix + key
		ev, found := existing[key]
		if !found {
			result[key] = sv
			continue
		}
		sm, sIsMap := sv.(map[string]any)
		em, eIsMap := ev.(map[string]any)
		if sIsMap && eIsMap {
			rm, _ := result[key].(map[string]any)
			conflicts = append(conflicts, compare(path+".", contextName, sm, em, rm, origin, valuesFile)...)
			continue
		}
		if !reflect.DeepEqual(sv, ev) {
			conflicts = append(conflicts, Conflict{
				Context: contextName, KeyPath: path,
				SourceValue: sv, SourceFile: origin[path],
				ValuesValue: ev, ValuesFile: valuesFile,
			})
		}
	}
	return conflicts
}

// formatValue renders a value for the conflict list.
func formatValue(v any) string {
	data, err := yaml.MarshalWithOptions(v, yaml.Flow(true))
	if err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSpace(string(data))
}
