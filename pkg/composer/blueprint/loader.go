package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/pkg/composer/artifact"
	"github.com/windsorcli/cli/pkg/runtime"
)

// BlueprintLoader holds individual blueprint state in-memory through the processing lifecycle.
// One BlueprintLoader is created per blueprint source (primary, each OCI source, user).
type BlueprintLoader interface {
	Load(sourceName, sourceURL string) error
	GetBlueprint() *blueprintv1alpha1.Blueprint
	GetFacets() []blueprintv1alpha1.Facet
	GetTemplateData() map[string][]byte
	GetSourceName() string
}

// =============================================================================
// Types
// =============================================================================

// BaseBlueprintLoader provides the default implementation of the BlueprintLoader interface.
type BaseBlueprintLoader struct {
	runtime         *runtime.Runtime
	artifactBuilder artifact.Artifact
	shims           *Shims

	sourceName    string
	sourceURL     string
	blueprint     *blueprintv1alpha1.Blueprint
	blueprintPath string
	facets        []blueprintv1alpha1.Facet
	templateData  map[string][]byte
}

// =============================================================================
// Constructor
// =============================================================================

// NewBlueprintLoader creates a new BlueprintLoader. The sourceName and sourceURL are provided
// when calling Load(), not during construction. The artifactBuilder is required for OCI sources
// but may be nil for local sources.
func NewBlueprintLoader(rt *runtime.Runtime, artifactBuilder artifact.Artifact) *BaseBlueprintLoader {
	if rt == nil {
		panic("runtime is required")
	}
	return &BaseBlueprintLoader{
		runtime:         rt,
		artifactBuilder: artifactBuilder,
		shims:           NewShims(),
		templateData:    make(map[string][]byte),
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Load loads the blueprint from the specified source. The sourceName identifies this loader
// (e.g., "user", "template", or a source name from the sources array). The sourceURL specifies
// an OCI artifact URL to pull; if empty, the loader will use local filesystem paths based on
// sourceName. For "user", it loads from config root. For other names, it loads from _template.
func (l *BaseBlueprintLoader) Load(sourceName, sourceURL string) error {
	l.sourceName = sourceName
	l.sourceURL = sourceURL

	if sourceURL != "" {
		if l.artifactBuilder == nil {
			return fmt.Errorf("artifact builder is required for OCI sources")
		}
		return l.loadFromOCI()
	}

	if sourceName == "user" {
		return l.loadUserBlueprint()
	}

	return l.loadFromLocalTemplate()
}

// GetBlueprint returns the loaded blueprint, which may be nil if loading failed or the source
// does not contain a blueprint. The blueprint is modified during facet processing as components
// from evaluated facets are appended to it.
func (l *BaseBlueprintLoader) GetBlueprint() *blueprintv1alpha1.Blueprint {
	return l.blueprint
}

// GetBlueprintPath returns the absolute path to the main blueprint file (e.g. blueprint.yaml)
// for this source. For the user blueprint this is the config root blueprint; for template/OCI
// sources it is empty. Used when resolving relative paths in expressions (e.g. yaml(), file()).
func (l *BaseBlueprintLoader) GetBlueprintPath() string {
	return l.blueprintPath
}

// GetFacets returns all Facet definitions loaded from this source's facets directory.
// Facets are YAML files in the facets/ subdirectory that define conditional terraform
// components and kustomizations. Returns an empty slice if no facets were found.
func (l *BaseBlueprintLoader) GetFacets() []blueprintv1alpha1.Facet {
	return l.facets
}

// GetTemplateData returns a map of relative file paths to their contents for all files collected
// from this source. This data is used when building OCI artifacts from local templates, allowing
// the complete template to be pushed to a registry.
func (l *BaseBlueprintLoader) GetTemplateData() map[string][]byte {
	return l.templateData
}

// GetSourceName returns the identifier for this loader, used in error messages and to track
// which source a blueprint came from during composition. Common values are "primary", "user",
// or the name specified in a blueprint's sources array.
func (l *BaseBlueprintLoader) GetSourceName() string {
	return l.sourceName
}

// =============================================================================
// Private Methods
// =============================================================================

// loadFromLocalTemplate reads blueprint data from the project's _template directory. It collects
// all template files into templateData, loads the schema.yaml into the ConfigHandler for validation,
// parses the blueprint.yaml, and loads any facet definitions from the facets subdirectory.
// Returns nil without error if the template directory doesn't exist.
func (l *BaseBlueprintLoader) loadFromLocalTemplate() error {
	templateRoot := l.runtime.TemplateRoot
	if templateRoot == "" {
		return nil
	}

	if _, err := l.shims.Stat(templateRoot); os.IsNotExist(err) {
		return nil
	}

	if err := l.collectTemplateData(templateRoot); err != nil {
		return fmt.Errorf("failed to collect template data: %w", err)
	}

	if err := l.loadSchema(); err != nil {
		return err
	}

	blueprintPath := filepath.Join(templateRoot, "blueprint.yaml")
	if err := l.loadBlueprintFromFile(blueprintPath); err != nil {
		return err
	}

	if err := l.loadFacetsFromDirectory(templateRoot); err != nil {
		return err
	}

	if err := l.loadFeaturesFromDirectory(templateRoot); err != nil {
		return err
	}

	if l.blueprint == nil {
		l.blueprint = DefaultBlueprint.DeepCopy()
	}

	return nil
}

// loadUserBlueprint reads the user's blueprint.yaml from the context's config root directory.
// Unlike template loading, user blueprints do not include facets or schema contributions -
// they only specify component selections and value overrides. Returns nil without error if
// no user blueprint exists.
func (l *BaseBlueprintLoader) loadUserBlueprint() error {
	configRoot := l.runtime.ConfigRoot
	if configRoot == "" {
		return nil
	}

	blueprintPath := filepath.Join(configRoot, "blueprint.yaml")
	if absPath, err := filepath.Abs(blueprintPath); err == nil {
		l.blueprintPath = absPath
	}
	if _, err := l.shims.Stat(blueprintPath); os.IsNotExist(err) {
		return nil
	}

	return l.loadBlueprintFromFile(blueprintPath)
}

// loadFromOCI pulls a blueprint artifact from an OCI registry and loads its contents. It uses
// the artifact builder to pull and cache the artifact, then collects template data from the
// extracted files. OCI artifacts may contain a _template subdirectory or have files directly
// in the root. Like local templates, it loads schema.yaml, blueprint.yaml, and facets. After
// loading, it injects the OCI source into the Sources array and sets the Source field on any
// components/kustomizations that don't already have one.
func (l *BaseBlueprintLoader) loadFromOCI() error {
	if l.artifactBuilder == nil {
		return fmt.Errorf("artifact builder not available for OCI source")
	}

	artifacts, err := l.artifactBuilder.Pull([]string{l.sourceURL})
	if err != nil {
		return fmt.Errorf("failed to pull OCI artifact: %w", err)
	}

	registry, repository, tag, err := l.artifactBuilder.ParseOCIRef(l.sourceURL)
	if err != nil {
		return fmt.Errorf("failed to parse OCI reference: %w", err)
	}

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	cacheDir, exists := artifacts[cacheKey]
	if !exists {
		return fmt.Errorf("failed to retrieve cache directory for %s", l.sourceURL)
	}

	templateDir := filepath.Join(cacheDir, "_template")
	if _, err := l.shims.Stat(templateDir); os.IsNotExist(err) {
		templateDir = cacheDir
	}

	if err := l.collectTemplateData(templateDir); err != nil {
		return fmt.Errorf("failed to collect template data from OCI: %w", err)
	}

	if err := l.loadSchema(); err != nil {
		return err
	}

	blueprintPath := filepath.Join(templateDir, "blueprint.yaml")
	if err := l.loadBlueprintFromFile(blueprintPath); err != nil {
		return err
	}

	if err := l.loadFacetsFromDirectory(templateDir); err != nil {
		return err
	}

	if err := l.loadFeaturesFromDirectory(templateDir); err != nil {
		return err
	}

	l.injectOCISource()

	return nil
}

// injectOCISource adds the OCI artifact as a source in the blueprint and sets the Source field
// on any terraform components and kustomizations that don't already have one. The source name
// uses the loader's sourceName (which was set when the loader was created based on the user's
// sources array) rather than extracting from the OCI URL. This ensures consistent matching
// between user blueprint sources and loaded blueprints during composition.
func (l *BaseBlueprintLoader) injectOCISource() {
	if l.blueprint == nil {
		return
	}

	ociInfo, err := artifact.ParseOCIReference(l.sourceURL)
	if err != nil || ociInfo == nil {
		return
	}

	trueVal := true
	ociSource := blueprintv1alpha1.Source{
		Name:    l.sourceName,
		Url:     ociInfo.URL,
		Install: &blueprintv1alpha1.BoolExpression{Value: &trueVal, IsExpr: false},
	}

	sourceExists := false
	for i, source := range l.blueprint.Sources {
		if source.Name == l.sourceName {
			l.blueprint.Sources[i] = ociSource
			sourceExists = true
			break
		}
	}
	if !sourceExists {
		l.blueprint.Sources = append(l.blueprint.Sources, ociSource)
	}

	for i := range l.blueprint.TerraformComponents {
		if l.blueprint.TerraformComponents[i].Source == "" {
			l.blueprint.TerraformComponents[i].Source = l.sourceName
		}
	}

	for i := range l.blueprint.Kustomizations {
		if l.blueprint.Kustomizations[i].Source == "" {
			l.blueprint.Kustomizations[i].Source = l.sourceName
		}
	}
}

// normalizeOCISourceRefs zeros Ref on sources whose OCI URL already includes the tag, so ref is
// not duplicated. Only the path (after the first slash) is checked for a tag; colons in the
// authority (e.g. localhost:5000) are not treated as tags.
func (l *BaseBlueprintLoader) normalizeOCISourceRefs(bp *blueprintv1alpha1.Blueprint) {
	if bp == nil {
		return
	}
	for i := range bp.Sources {
		s := &bp.Sources[i]
		if s.Url == "" || !strings.HasPrefix(s.Url, "oci://") {
			continue
		}
		afterScheme := s.Url[7:]
		firstSlash := strings.Index(afterScheme, "/")
		if firstSlash == -1 {
			continue
		}
		pathPart := afterScheme[firstSlash+1:]
		if strings.Contains(pathPart, ":") {
			s.Ref = blueprintv1alpha1.Reference{}
		}
	}
}

// loadBlueprintFromFile reads and unmarshals a blueprint.yaml file at the given path. The parsed
// blueprint is stored in the loader's blueprint field. Returns nil without error if the file
// does not exist, allowing callers to handle optional blueprints gracefully.
func (l *BaseBlueprintLoader) loadBlueprintFromFile(path string) error {
	if _, err := l.shims.Stat(path); os.IsNotExist(err) {
		return nil
	}

	data, err := l.shims.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read blueprint.yaml: %w", err)
	}

	var bp blueprintv1alpha1.Blueprint
	if err := l.shims.YamlUnmarshal(data, &bp); err != nil {
		return fmt.Errorf("failed to parse blueprint.yaml: %w", err)
	}

	l.normalizeOCISourceRefs(&bp)
	l.blueprint = &bp
	return nil
}

// loadFacetsFromDirectory scans the facets/ subdirectory for YAML files and parses each
// as a Facet definition. Facets define conditional terraform components and kustomizations
// that are included based on 'when' conditions during processing. Each facet's Path field is
// set to its source file location for debugging and error reporting.
func (l *BaseBlueprintLoader) loadFacetsFromDirectory(baseDir string) error {
	facetsDir := filepath.Join(baseDir, "facets")
	if _, err := l.shims.Stat(facetsDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := l.shims.ReadDir(facetsDir)
	if err != nil {
		return fmt.Errorf("failed to read facets directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		facetPath := filepath.Join(facetsDir, entry.Name())
		data, err := l.shims.ReadFile(facetPath)
		if err != nil {
			return fmt.Errorf("failed to read facet %s: %w", entry.Name(), err)
		}

		var facet blueprintv1alpha1.Facet
		if err := l.shims.YamlUnmarshal(data, &facet); err != nil {
			return fmt.Errorf("failed to parse facet %s: %w", entry.Name(), err)
		}

		facet.Path = facetPath
		l.facets = append(l.facets, facet)
	}

	return nil
}

// loadFeaturesFromDirectory scans the features/ subdirectory for YAML files and parses each
// as a Feature definition (for backwards compatibility). Features are converted to Facets
// internally. This maintains backwards compatibility with existing blueprints that use the
// old features/ directory and kind: Feature. Returns nil without error if the directory
// doesn't exist.
func (l *BaseBlueprintLoader) loadFeaturesFromDirectory(baseDir string) error {
	featuresDir := filepath.Join(baseDir, "features")
	if _, err := l.shims.Stat(featuresDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := l.shims.ReadDir(featuresDir)
	if err != nil {
		return fmt.Errorf("failed to read features directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		featurePath := filepath.Join(featuresDir, entry.Name())
		data, err := l.shims.ReadFile(featurePath)
		if err != nil {
			return fmt.Errorf("failed to read feature %s: %w", entry.Name(), err)
		}

		var facet blueprintv1alpha1.Facet
		if err := l.shims.YamlUnmarshal(data, &facet); err != nil {
			return fmt.Errorf("failed to parse feature %s: %w", entry.Name(), err)
		}

		if facet.Kind == "Feature" {
			facet.Kind = "Facet"
		}

		facet.Path = featurePath
		l.facets = append(l.facets, facet)
	}

	return nil
}

// loadSchema checks for a schema.yaml in the collected template data and loads it into the
// ConfigHandler for validation. Schemas from multiple sources are progressively merged by the
// ConfigHandler, allowing each source to contribute its own configuration definitions. Returns
// nil if no schema exists or ConfigHandler is unavailable.
func (l *BaseBlueprintLoader) loadSchema() error {
	schemaData, exists := l.templateData["schema.yaml"]
	if !exists {
		return nil
	}

	if l.runtime.ConfigHandler == nil {
		return nil
	}

	if err := l.runtime.ConfigHandler.LoadSchemaFromBytes(schemaData); err != nil {
		return fmt.Errorf("failed to load schema for source '%s': %w", l.sourceName, err)
	}

	return nil
}

// collectTemplateData recursively walks a directory tree and reads all file contents into the
// templateData map. File paths are stored relative to the base directory. This data is used
// for artifact building (pushing local templates to OCI) and for loading schema/blueprint files
// from OCI artifacts that have already been extracted to a cache directory.
func (l *BaseBlueprintLoader) collectTemplateData(dir string) error {
	return l.shims.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		data, err := l.shims.ReadFile(path)
		if err != nil {
			return err
		}

		l.templateData[relPath] = data
		return nil
	})
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ BlueprintLoader = (*BaseBlueprintLoader)(nil)
