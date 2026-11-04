// The Terraform scan is the Terraform step of the Lister.
// It provides the charts and Kubernetes objects that each enabled Terraform component declares.
// It parses each module with hcl/v2 and evaluates its resource blocks from the component inputs
// and the variable defaults. A resource attribute, a data source, or a module output is unknown.

package inventory

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	"helm.sh/helm/v4/pkg/strvals"
	sigsyaml "sigs.k8s.io/yaml"

	blueprintv1alpha1 "github.com/windsorcli/cli/api/v1alpha1"
	"github.com/windsorcli/cli/internal/werror"
	blueprintcomposer "github.com/windsorcli/cli/pkg/composer/blueprint"
)

// =============================================================================
// Constants
// =============================================================================

// maxModuleDepth limits how deep the scan follows local module sources.
const maxModuleDepth = 10

// workloadResources are the Kubernetes provider resources whose pod template names images.
var workloadResources = map[string]bool{
	"kubernetes_pod": true, "kubernetes_pod_v1": true,
	"kubernetes_deployment": true, "kubernetes_deployment_v1": true,
	"kubernetes_daemonset": true, "kubernetes_daemon_set_v1": true,
	"kubernetes_stateful_set": true, "kubernetes_stateful_set_v1": true,
	"kubernetes_replication_controller": true, "kubernetes_replication_controller_v1": true,
	"kubernetes_job": true, "kubernetes_job_v1": true,
	"kubernetes_cron_job": true, "kubernetes_cron_job_v1": true,
}

// =============================================================================
// Types
// =============================================================================

// tfModule is the parsed content of one Terraform module directory.
type tfModule struct {
	dir       string
	variables map[string]hcl.Expression
	locals    map[string]hcl.Expression
	resources []tfBlock
	modules   []tfBlock
}

// tfBlock is one resource or module block and the file that holds it, relative to the source root.
// key is the instance key of a block with count or for_each, such as [0] or ["a"].
type tfBlock struct {
	kind string
	name string
	key  string
	file string
	body *hclsyntax.Body
}

// tfInstance is one instance of a block and the context that evaluates it.
type tfInstance struct {
	block tfBlock
	ctx   *hcl.EvalContext
}

// tfScan holds the state of the scan of one Terraform component.
type tfScan struct {
	lister    *BaseLister
	component string
	root      string
	objects   []renderedObject
	result    *Result
	rendered  []renderedObject
	entries   []Entry
}

// =============================================================================
// Private Methods
// =============================================================================

// listFromTerraform returns the objects and chart entries that the enabled Terraform components of
// blueprint declare. Each helm_release chart renders with objects as its cluster context. A
// component from a Git or remote module source gives a note, because the scan reads only local
// and OCI cache directories.
func (l *BaseLister) listFromTerraform(blueprint *blueprintv1alpha1.Blueprint, objects []renderedObject, result *Result) ([]renderedObject, []Entry, error) {
	var rendered []renderedObject
	var entries []Entry
	for _, c := range blueprint.TerraformComponents {
		if c.Enabled != nil && !c.Enabled.IsEnabled() {
			continue
		}
		root, ok, err := l.componentRoot(blueprint, c)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			result.Notes = append(result.Notes, fmt.Sprintf("Terraform component %q comes from %s, so the list has no artifacts from it.", c.GetID(), c.Source))
			continue
		}
		dir := filepath.Join(root, pathPrefix(blueprint, c), c.Path)
		if _, err := l.shims.Stat(dir); err != nil {
			return nil, nil, werror.New(
				fmt.Sprintf("module directory not found for terraform component %q at %s", c.GetID(), dir),
				"Run windsor init to pull the blueprint sources.", nil)
		}
		scan := &tfScan{lister: l, component: c.GetID(), root: root, objects: objects, result: result}
		if err := scan.module(dir, inputValues(c.Inputs), 0); err != nil {
			return nil, nil, err
		}
		rendered = append(rendered, scan.rendered...)
		entries = append(entries, scan.entries...)
	}
	return rendered, entries, nil
}

// pathPrefix returns the pathPrefix of the source of c, or terraform when the source sets none.
func pathPrefix(blueprint *blueprintv1alpha1.Blueprint, c blueprintv1alpha1.TerraformComponent) string {
	name := c.Source
	if name == "" || name == "template" {
		name = blueprint.Metadata.Name
	}
	for _, source := range blueprint.Sources {
		if source.Name == name && source.PathPrefix != "" {
			return source.PathPrefix
		}
	}
	return "terraform"
}

// componentRoot returns the source root of c. It returns false for a module source that is a URL
// or a named source that is not an OCI artifact.
func (l *BaseLister) componentRoot(blueprint *blueprintv1alpha1.Blueprint, c blueprintv1alpha1.TerraformComponent) (string, bool, error) {
	if strings.Contains(c.Source, "://") || strings.Contains(c.Source, "::") {
		return "", false, nil
	}
	if c.Source != "" && c.Source != "template" && c.Source != blueprint.Metadata.Name {
		for _, source := range blueprint.Sources {
			if source.Name == c.Source && !strings.HasPrefix(source.Url, "oci://") {
				return "", false, nil
			}
		}
	}
	root, err := l.sourceRoot(blueprint, c.Source, "terraform component "+c.GetID())
	return root, err == nil, err
}

// module parses dir, evaluates its locals from inputs and the variable defaults, and reads each
// instance of each resource. It follows each module block with a local source, up to
// maxModuleDepth levels.
func (s *tfScan) module(dir string, inputs map[string]cty.Value, depth int) error {
	mod, err := s.parse(dir)
	if err != nil {
		return err
	}
	ctx := s.evalContext(mod, inputs)

	for _, resource := range mod.resources {
		for _, inst := range instances(resource, ctx) {
			var err error
			switch {
			case resource.kind == "helm_release":
				err = s.helmRelease(inst.block, inst.ctx)
			case resource.kind == "kubernetes_manifest":
				s.manifest(inst.block, inst.ctx)
			case workloadResources[resource.kind]:
				s.workload(inst.block, inst.ctx)
			}
			if err != nil {
				return err
			}
		}
	}

	for _, block := range mod.modules {
		source, _ := evalString(block.body, "source", ctx)
		if !strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "../") {
			s.result.Notes = append(s.result.Notes, fmt.Sprintf("module.%s in %s has a remote source, so the list has no artifacts from it.", block.name, block.file))
			continue
		}
		if depth >= maxModuleDepth {
			return fmt.Errorf("module.%s in %s is more than %d levels deep", block.name, block.file, maxModuleDepth)
		}
		for _, inst := range instances(block, ctx) {
			childInputs := make(map[string]cty.Value)
			for name, attr := range block.body.Attributes {
				switch name {
				case "source", "version", "count", "for_each", "providers", "depends_on":
					continue
				}
				childInputs[name] = evalValue(attr.Expr, inst.ctx)
			}
			if err := s.module(filepath.Join(dir, source), childInputs, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// parse reads every .tf file in dir and returns its variables, locals, resources and modules.
func (s *tfScan) parse(dir string) (*tfModule, error) {
	entries, err := s.lister.shims.ReadDir(dir)
	if err != nil {
		return nil, werror.Wrap(err, "failed to read module directory %s", dir)
	}
	mod := &tfModule{dir: dir, variables: make(map[string]hcl.Expression), locals: make(map[string]hcl.Expression)}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := s.lister.shims.ReadFile(path)
		if err != nil {
			return nil, werror.Wrap(err, "failed to read %s", path)
		}
		file, diags := hclsyntax.ParseConfig(data, path, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return nil, werror.Wrap(diags, "failed to parse %s", path)
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			rel = path
		}
		body, _ := file.Body.(*hclsyntax.Body)
		for _, block := range body.Blocks {
			switch {
			case block.Type == "variable" && len(block.Labels) == 1:
				var def hcl.Expression
				if attr, ok := block.Body.Attributes["default"]; ok {
					def = attr.Expr
				}
				mod.variables[block.Labels[0]] = def
			case block.Type == "locals":
				for name, attr := range block.Body.Attributes {
					mod.locals[name] = attr.Expr
				}
			case block.Type == "resource" && len(block.Labels) == 2:
				mod.resources = append(mod.resources, tfBlock{kind: block.Labels[0], name: block.Labels[1], file: filepath.ToSlash(rel), body: block.Body})
			case block.Type == "module" && len(block.Labels) == 1:
				mod.modules = append(mod.modules, tfBlock{kind: "module", name: block.Labels[0], file: filepath.ToSlash(rel), body: block.Body})
			}
		}
	}
	sort.SliceStable(mod.resources, func(i, j int) bool {
		return mod.resources[i].file+mod.resources[i].kind+mod.resources[i].name < mod.resources[j].file+mod.resources[j].kind+mod.resources[j].name
	})
	sort.SliceStable(mod.modules, func(i, j int) bool { return mod.modules[i].name < mod.modules[j].name })
	return mod, nil
}

// evalContext returns the evaluation context of mod. Each variable takes its input, else its
// default, else an unknown value. Locals resolve over repeated passes, so a local can use another
// local. A local that does not evaluate is unknown. Resource, data and module references are unknown.
func (s *tfScan) evalContext(mod *tfModule, inputs map[string]cty.Value) *hcl.EvalContext {
	vars := make(map[string]cty.Value, len(mod.variables))
	for name, def := range mod.variables {
		switch {
		case inputs[name] != cty.NilVal:
			vars[name] = inputs[name]
		case def != nil:
			vars[name] = evalValue(def, &hcl.EvalContext{Functions: terraformFunctions()})
		default:
			vars[name] = cty.DynamicVal
		}
	}

	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"var":       objectOf(vars),
			"path":      cty.ObjectVal(map[string]cty.Value{"module": cty.StringVal(mod.dir), "root": cty.StringVal(mod.dir), "cwd": cty.StringVal(mod.dir)}),
			"terraform": cty.ObjectVal(map[string]cty.Value{"workspace": cty.StringVal("default")}),
			"data":      cty.DynamicVal,
			"module":    cty.DynamicVal,
			"each":      cty.DynamicVal,
			"count":     cty.DynamicVal,
			"self":      cty.DynamicVal,
		},
		Functions: terraformFunctions(),
	}
	for _, block := range mod.resources {
		ctx.Variables[block.kind] = cty.DynamicVal
	}

	locals := make(map[string]cty.Value, len(mod.locals))
	for name := range mod.locals {
		locals[name] = cty.DynamicVal
	}
	for pass := 0; pass <= len(mod.locals); pass++ {
		ctx.Variables["local"] = objectOf(locals)
		next := make(map[string]cty.Value, len(mod.locals))
		for name, expr := range mod.locals {
			next[name] = evalValue(expr, ctx)
		}
		locals = next
	}
	ctx.Variables["local"] = objectOf(locals)
	return ctx
}

// helmRelease adds the chart of block and renders it with its values and set entries. An unknown
// repository, chart or version is an error, and so is one that does not evaluate. A values document that is not known gives a note, and
// the chart renders with its defaults for it.
func (s *tfScan) helmRelease(block tfBlock, ctx *hcl.EvalContext) error {
	id := block.id()
	for _, name := range []string{"repository", "chart", "version"} {
		if attr, ok := block.body.Attributes[name]; ok {
			if _, diags := attr.Expr.Value(ctx); diags.HasErrors() {
				return werror.Wrap(diags, "failed to evaluate the %s of %s in %s", name, id, block.file)
			}
		}
	}
	repository, repoKnown := evalString(block.body, "repository", ctx)
	chartName, chartKnown := evalString(block.body, "chart", ctx)
	version, versionKnown := evalString(block.body, "version", ctx)
	if !repoKnown || !chartKnown || !versionKnown {
		return werror.New(
			fmt.Sprintf("the chart of %s in %s uses a value that is not known before apply", id, block.file),
			"Give the variable a value that does not come from a Terraform output.", nil)
	}

	chart, ok := terraformChart(repository, chartName, version)
	if !ok {
		s.result.Notes = append(s.result.Notes, fmt.Sprintf("%s in %s uses chart %q from no remote repository, so the list leaves it out.", id, block.file, chartName))
		return nil
	}
	chart.Source = Source{Object: id, File: block.file}
	s.entries = append(s.entries, chart)
	if !isExactVersion(chart.Version) {
		s.result.Notes = append(s.result.Notes, fmt.Sprintf("%s in %s has no exact chart version, so the list has no images from chart %s.", id, block.file, chart.Upstream()))
		return nil
	}

	values, unknown, err := releaseValuesFromHCL(block.body, ctx)
	if err != nil {
		return werror.Wrap(err, "failed to read the values of %s in %s", id, block.file)
	}
	if unknown {
		s.result.Notes = append(s.result.Notes, fmt.Sprintf("%s in %s has values that are not known before apply, so chart %s renders with its defaults for them.", id, block.file, chart.Upstream()))
	}

	name, _ := evalString(block.body, "name", ctx)
	namespace, _ := evalString(block.body, "namespace", ctx)
	if namespace == "" {
		namespace = "default"
	}
	hr := map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec":       map[string]any{"releaseName": name, "values": values},
	}
	raw := make([]map[string]any, len(s.objects))
	for i, o := range s.objects {
		raw[i] = o.object
	}
	out, err := s.lister.chartRenderer.Render(ChartRequest{Chart: chart, HelmRelease: hr, Objects: raw})
	if err != nil {
		return werror.Wrap(err, "failed to render %s in %s", id, block.file)
	}
	for _, o := range out {
		s.rendered = append(s.rendered, renderedObject{object: o, parent: id, file: block.file})
	}
	return nil
}

// manifest adds the object of a kubernetes_manifest. An unknown value in it becomes the deferred marker.
func (s *tfScan) manifest(block tfBlock, ctx *hcl.EvalContext) {
	attr, ok := block.body.Attributes["manifest"]
	if !ok {
		return
	}
	obj, _ := ctyToGo(evalValue(attr.Expr, ctx), deferredMarker).(map[string]any)
	if obj == nil {
		return
	}
	s.rendered = append(s.rendered, renderedObject{object: obj, parent: block.id(), file: block.file})
}

// workload adds a Pod that holds the image of each container block in a Kubernetes workload
// resource. An unknown image becomes the deferred marker.
func (s *tfScan) workload(block tfBlock, ctx *hcl.EvalContext) {
	var containers []any
	var walk func(body *hclsyntax.Body)
	walk = func(body *hclsyntax.Body) {
		for _, b := range body.Blocks {
			switch b.Type {
			case "container", "init_container", "ephemeral_container":
				if image, known := evalString(b.Body, "image", ctx); !known {
					containers = append(containers, map[string]any{"image": deferredMarker})
				} else if image != "" {
					containers = append(containers, map[string]any{"image": image})
				}
			default:
				walk(b.Body)
			}
		}
	}
	walk(block.body)
	if len(containers) == 0 {
		return
	}
	pod := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": block.id()},
		"spec":       map[string]any{"containers": containers},
	}
	s.rendered = append(s.rendered, renderedObject{object: pod, parent: block.id(), file: block.file})
}

// =============================================================================
// Helpers
// =============================================================================

// terraformChart returns the chart entry of a helm_release. An oci:// repository or chart gives an
// OCI chart, and an http(s) repository gives an HTTP chart. It returns false for a local chart.
func terraformChart(repository, chart, version string) (Entry, bool) {
	entry := Entry{Kind: KindChart, Version: version}
	switch {
	case strings.HasPrefix(repository, "oci://"):
		entry.Transport = TransportOCI
		entry.Reference = strings.TrimSuffix(strings.TrimPrefix(repository, "oci://"), "/") + "/" + chart
		entry.Name = chart
	case repository == "" && strings.HasPrefix(chart, "oci://"):
		entry.Transport = TransportOCI
		entry.Reference = strings.TrimPrefix(chart, "oci://")
		entry.Name = path.Base(entry.Reference)
	case strings.HasPrefix(repository, "http://") || strings.HasPrefix(repository, "https://"):
		entry.Transport = TransportHTTP
		entry.Reference = repository
		entry.Name = chart
	default:
		return Entry{}, false
	}
	return entry, true
}

// releaseValuesFromHCL merges the values documents of a helm_release in order, then applies each
// set and set_sensitive entry, as the Helm provider does. An unknown set value becomes the deferred
// marker. It reports true when a values document is not known. A values document or set entry
// that does not parse is an error.
func releaseValuesFromHCL(body *hclsyntax.Body, ctx *hcl.EvalContext) (map[string]any, bool, error) {
	values := make(map[string]any)
	unknown := false
	if attr, ok := body.Attributes["values"]; ok {
		list := evalValue(attr.Expr, ctx)
		if !list.IsKnown() {
			return values, true, nil
		}
		if !list.IsNull() && list.CanIterateElements() {
			for it := list.ElementIterator(); it.Next(); {
				_, doc := it.Element()
				if !doc.IsWhollyKnown() || doc.IsNull() || !doc.Type().Equals(cty.String) {
					unknown = unknown || !doc.IsWhollyKnown()
					continue
				}
				var m map[string]any
				if err := sigsyaml.Unmarshal([]byte(doc.AsString()), &m); err != nil {
					return nil, false, werror.Wrap(err, "failed to read a values document")
				}
				values = mergeValues(values, m)
			}
		}
	}

	var sets []map[string]any
	if attr, ok := body.Attributes["set"]; ok {
		if list, ok := ctyToGo(evalValue(attr.Expr, ctx), deferredMarker).([]any); ok {
			for _, item := range list {
				if m, ok := item.(map[string]any); ok {
					sets = append(sets, m)
				}
			}
		}
	}
	if attr, ok := body.Attributes["set_sensitive"]; ok {
		if list, ok := ctyToGo(evalValue(attr.Expr, ctx), deferredMarker).([]any); ok {
			for _, item := range list {
				if m, ok := item.(map[string]any); ok {
					sets = append(sets, m)
				}
			}
		}
	}
	for _, b := range body.Blocks {
		if b.Type != "set" && b.Type != "set_sensitive" {
			continue
		}
		m := make(map[string]any)
		for name, attr := range b.Body.Attributes {
			m[name] = ctyToGo(evalValue(attr.Expr, ctx), deferredMarker)
		}
		sets = append(sets, m)
	}
	for _, set := range sets {
		name, _ := set["name"].(string)
		if name == "" || set["value"] == nil {
			continue
		}
		value := fmt.Sprint(set["value"])
		var err error
		switch t, _ := set["type"].(string); t {
		case "string":
			err = strvals.ParseIntoString(name+"="+value, values)
		case "literal":
			err = strvals.ParseLiteralInto(name+"="+value, values)
		default:
			err = strvals.ParseInto(name+"="+value, values)
		}
		if err != nil {
			return nil, false, werror.Wrap(err, "failed to read set entry %q", name)
		}
	}
	return values, unknown, nil
}

// mergeValues merges src into dst. A map merges key by key, and any other src value replaces dst.
func mergeValues(dst, src map[string]any) map[string]any {
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			if dv, ok := dst[k].(map[string]any); ok {
				dst[k] = mergeValues(dv, sv)
				continue
			}
		}
		dst[k] = v
	}
	return dst
}

// id returns the address of b, such as helm_release.cilium or helm_release.app["a"].
func (b tfBlock) id() string {
	return b.kind + "." + b.name + b.key
}

// instances returns one instance of block for each count index or for_each element, with count
// or each set in its context. A known count of 0 or an empty for_each gives no instance. An unknown
// count or for_each gives one instance whose count or each is unknown.
func instances(block tfBlock, ctx *hcl.EvalContext) []tfInstance {
	if attr, ok := block.body.Attributes["count"]; ok {
		v := evalValue(attr.Expr, ctx)
		if !v.IsKnown() || v.IsNull() || v.Type() != cty.Number {
			return []tfInstance{{block: block, ctx: ctx}}
		}
		n, _ := v.AsBigFloat().Int64()
		out := make([]tfInstance, 0, n)
		for i := int64(0); i < n; i++ {
			child := ctx.NewChild()
			child.Variables = map[string]cty.Value{"count": cty.ObjectVal(map[string]cty.Value{"index": cty.NumberIntVal(i)})}
			b := block
			b.key = fmt.Sprintf("[%d]", i)
			out = append(out, tfInstance{block: b, ctx: child})
		}
		return out
	}
	if attr, ok := block.body.Attributes["for_each"]; ok {
		v := evalValue(attr.Expr, ctx)
		if !v.IsWhollyKnown() || v.IsNull() || !v.CanIterateElements() {
			return []tfInstance{{block: block, ctx: ctx}}
		}
		var out []tfInstance
		for it := v.ElementIterator(); it.Next(); {
			k, e := it.Element()
			if v.Type().IsSetType() {
				k = e
			}
			if k.Type() != cty.String {
				continue
			}
			child := ctx.NewChild()
			child.Variables = map[string]cty.Value{"each": cty.ObjectVal(map[string]cty.Value{"key": k, "value": e})}
			b := block
			b.key = fmt.Sprintf("[%q]", k.AsString())
			out = append(out, tfInstance{block: b, ctx: child})
		}
		return out
	}
	return []tfInstance{{block: block, ctx: ctx}}
}

// evalValue returns the value of expr in ctx, or an unknown value when it does not evaluate.
func evalValue(expr hcl.Expression, ctx *hcl.EvalContext) cty.Value {
	v, diags := expr.Value(ctx)
	if diags.HasErrors() {
		return cty.DynamicVal
	}
	return v
}

// evalString returns the string value of the attribute name of body. It returns "" and true when
// the attribute is absent or null, and false when its value is not known.
func evalString(body *hclsyntax.Body, name string, ctx *hcl.EvalContext) (string, bool) {
	attr, ok := body.Attributes[name]
	if !ok {
		return "", true
	}
	v := evalValue(attr.Expr, ctx)
	if !v.IsWhollyKnown() {
		return "", false
	}
	if v.IsNull() {
		return "", true
	}
	s, ok := ctyToGo(v, nil).(string)
	if !ok {
		return fmt.Sprint(ctyToGo(v, nil)), true
	}
	if strings.Contains(s, deferredMarker) {
		return "", false
	}
	return s, true
}

// inputValues converts the inputs of a Terraform component to cty values. A string that holds the
// deferred placeholder becomes an unknown value. A null input is left out, as Windsor leaves it out
// of the tfvars file, so the variable takes its default.
func inputValues(inputs map[string]any) map[string]cty.Value {
	out := make(map[string]cty.Value, len(inputs))
	for k, v := range inputs {
		if v == nil {
			continue
		}
		out[k] = goToCty(v)
	}
	return out
}

// goToCty converts a decoded YAML value to a cty value.
func goToCty(v any) cty.Value {
	switch t := v.(type) {
	case nil:
		return cty.NullVal(cty.DynamicPseudoType)
	case string:
		if strings.Contains(t, blueprintcomposer.DeferredPlaceholder) {
			return cty.DynamicVal
		}
		return cty.StringVal(t)
	case bool:
		return cty.BoolVal(t)
	case int:
		return cty.NumberIntVal(int64(t))
	case int64:
		return cty.NumberIntVal(t)
	case uint64:
		return cty.NumberUIntVal(t)
	case float64:
		return cty.NumberFloatVal(t)
	case []any:
		if len(t) == 0 {
			return cty.EmptyTupleVal
		}
		elems := make([]cty.Value, len(t))
		for i, e := range t {
			elems[i] = goToCty(e)
		}
		return cty.TupleVal(elems)
	case map[string]any:
		attrs := make(map[string]cty.Value, len(t))
		for k, e := range t {
			attrs[k] = goToCty(e)
		}
		return objectOf(attrs)
	default:
		return cty.StringVal(fmt.Sprint(t))
	}
}

// ctyToGo converts a cty value to a YAML-style Go value. An unknown value becomes unknownAs.
func ctyToGo(v cty.Value, unknownAs any) any {
	if !v.IsKnown() {
		return unknownAs
	}
	if v.IsNull() {
		return nil
	}
	t := v.Type()
	switch {
	case t == cty.String:
		return v.AsString()
	case t == cty.Bool:
		return v.True()
	case t == cty.Number:
		bf := v.AsBigFloat()
		if i, acc := bf.Int64(); acc == big.Exact {
			return i
		}
		f, _ := bf.Float64()
		return f
	case t.IsObjectType() || t.IsMapType():
		out := make(map[string]any)
		for it := v.ElementIterator(); it.Next(); {
			k, e := it.Element()
			out[k.AsString()] = ctyToGo(e, unknownAs)
		}
		return out
	case t.IsListType() || t.IsTupleType() || t.IsSetType():
		out := make([]any, 0, v.LengthInt())
		for it := v.ElementIterator(); it.Next(); {
			_, e := it.Element()
			out = append(out, ctyToGo(e, unknownAs))
		}
		return out
	}
	return nil
}

// objectOf returns an object value of attrs, or an empty object.
func objectOf(attrs map[string]cty.Value) cty.Value {
	if len(attrs) == 0 {
		return cty.EmptyObjectVal
	}
	return cty.ObjectVal(attrs)
}

// terraformFunctions returns the Terraform functions that a static evaluation can run. A function
// that the map lacks, such as a file function, makes its expression unknown.
func terraformFunctions() map[string]function.Function {
	return map[string]function.Function{
		"abs": stdlib.AbsoluteFunc, "ceil": stdlib.CeilFunc, "floor": stdlib.FloorFunc,
		"log": stdlib.LogFunc, "max": stdlib.MaxFunc, "min": stdlib.MinFunc, "pow": stdlib.PowFunc,
		"signum": stdlib.SignumFunc, "parseint": stdlib.ParseIntFunc,
		"chomp": stdlib.ChompFunc, "format": stdlib.FormatFunc, "formatlist": stdlib.FormatListFunc,
		"indent": stdlib.IndentFunc, "join": stdlib.JoinFunc, "lower": stdlib.LowerFunc,
		"upper": stdlib.UpperFunc, "replace": replaceFunc, "split": stdlib.SplitFunc,
		"substr": stdlib.SubstrFunc, "title": stdlib.TitleFunc, "trim": stdlib.TrimFunc,
		"trimprefix": stdlib.TrimPrefixFunc, "trimsuffix": stdlib.TrimSuffixFunc,
		"trimspace": stdlib.TrimSpaceFunc, "regex": stdlib.RegexFunc, "regexall": stdlib.RegexAllFunc,
		"chunklist": stdlib.ChunklistFunc, "coalesce": coalesceFunc,
		"coalescelist": stdlib.CoalesceListFunc, "compact": stdlib.CompactFunc,
		"concat": stdlib.ConcatFunc, "contains": stdlib.ContainsFunc, "distinct": stdlib.DistinctFunc,
		"element": stdlib.ElementFunc, "flatten": stdlib.FlattenFunc, "index": stdlib.IndexFunc,
		"keys": stdlib.KeysFunc, "length": lengthFunc, "lookup": stdlib.LookupFunc,
		"merge": stdlib.MergeFunc, "range": stdlib.RangeFunc, "reverse": stdlib.ReverseListFunc,
		"setintersection": stdlib.SetIntersectionFunc, "setproduct": stdlib.SetProductFunc,
		"setsubtract": stdlib.SetSubtractFunc, "setunion": stdlib.SetUnionFunc,
		"slice": stdlib.SliceFunc, "sort": stdlib.SortFunc, "values": stdlib.ValuesFunc,
		"zipmap": stdlib.ZipmapFunc, "csvdecode": stdlib.CSVDecodeFunc,
		"jsonencode": jsonEncodeFunc, "jsondecode": stdlib.JSONDecodeFunc,
		"timeadd": stdlib.TimeAddFunc, "formatdate": stdlib.FormatDateFunc,
		"tostring": stdlib.MakeToFunc(cty.String), "tonumber": stdlib.MakeToFunc(cty.Number),
		"tobool": stdlib.MakeToFunc(cty.Bool), "tolist": stdlib.MakeToFunc(cty.List(cty.DynamicPseudoType)),
		"toset": stdlib.MakeToFunc(cty.Set(cty.DynamicPseudoType)), "tomap": stdlib.MakeToFunc(cty.Map(cty.DynamicPseudoType)),
		"try": tryfunc.TryFunc, "can": tryfunc.CanFunc,
		"yamlencode": yamlEncodeFunc,
		"yamldecode": yamlDecodeFunc,
		"startswith": stringPredicateFunc(strings.HasPrefix),
		"endswith":   stringPredicateFunc(strings.HasSuffix),
		"alltrue":    boolListFunc(true),
		"anytrue":    boolListFunc(false),
		"base64encode": stringFunc(func(s string) (string, error) {
			return base64.StdEncoding.EncodeToString([]byte(s)), nil
		}),
		"base64decode": stringFunc(func(s string) (string, error) {
			b, err := base64.StdEncoding.DecodeString(s)
			return string(b), err
		}),
		"sha256": stringFunc(func(s string) (string, error) {
			return fmt.Sprintf("%x", sha256.Sum256([]byte(s))), nil
		}),
		"basename": stringFunc(func(s string) (string, error) { return filepath.Base(s), nil }),
		"dirname":  stringFunc(func(s string) (string, error) { return filepath.Dir(s), nil }),
	}
}

// yamlEncodeFunc encodes a value as YAML. Each unknown part becomes the deferred marker, so the
// known parts of a values document stay, and an unknown part that lands in an image is an error.
var yamlEncodeFunc = encodeFunc(sigsyaml.Marshal)

// jsonEncodeFunc encodes a value as JSON. Each unknown part becomes the deferred marker.
var jsonEncodeFunc = encodeFunc(json.Marshal)

// encodeFunc returns a function that encodes its argument with marshal.
func encodeFunc(marshal func(any) ([]byte, error)) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "value", Type: cty.DynamicPseudoType, AllowUnknown: true, AllowDynamicType: true, AllowNull: true}},
		Type:   function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			out, err := marshal(ctyToGo(args[0], deferredMarker))
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(strings.TrimSuffix(string(out), "\n")), nil
		},
	})
}

// yamlDecodeFunc decodes a YAML string into a value.
var yamlDecodeFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "src", Type: cty.String}},
	Type:   function.StaticReturnType(cty.DynamicPseudoType),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		var v any
		if err := sigsyaml.Unmarshal([]byte(args[0].AsString()), &v); err != nil {
			return cty.NilVal, err
		}
		return goToCty(v), nil
	},
})

// coalesceFunc returns its first argument that is not null and not an empty string, as Terraform does.
var coalesceFunc = function.New(&function.Spec{
	VarParam: &function.Parameter{Name: "vals", Type: cty.DynamicPseudoType, AllowNull: true, AllowUnknown: true, AllowDynamicType: true},
	Type:     function.StaticReturnType(cty.DynamicPseudoType),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		for _, v := range args {
			if !v.IsKnown() {
				return cty.DynamicVal, nil
			}
			if v.IsNull() || (v.Type() == cty.String && v.AsString() == "") {
				continue
			}
			return v, nil
		}
		return cty.NilVal, fmt.Errorf("no non-null, non-empty-string arguments")
	},
})

// replaceFunc replaces substr in str, or each match of a regular expression when substr is /re/,
// as Terraform does.
var replaceFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "str", Type: cty.String}, {Name: "substr", Type: cty.String}, {Name: "replace", Type: cty.String}},
	Type:   function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		substr := args[1].AsString()
		if len(substr) > 1 && strings.HasPrefix(substr, "/") && strings.HasSuffix(substr, "/") {
			return stdlib.RegexReplace(args[0], cty.StringVal(substr[1:len(substr)-1]), args[2])
		}
		return stdlib.Replace(args[0], args[1], args[2])
	},
})

// lengthFunc returns the length of a string, as Terraform does, or of a collection.
var lengthFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "value", Type: cty.DynamicPseudoType, AllowDynamicType: true, AllowUnknown: true}},
	Type:   function.StaticReturnType(cty.Number),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		if args[0].Type() == cty.String {
			return stdlib.Strlen(args[0])
		}
		return stdlib.Length(args[0])
	},
})

// stringFunc returns a function of one string argument.
func stringFunc(fn func(string) (string, error)) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "str", Type: cty.String}},
		Type:   function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			out, err := fn(args[0].AsString())
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(out), nil
		},
	})
}

// stringPredicateFunc returns a function that tests a string against a second string.
func stringPredicateFunc(fn func(s, part string) bool) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "str", Type: cty.String}, {Name: "part", Type: cty.String}},
		Type:   function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			return cty.BoolVal(fn(args[0].AsString(), args[1].AsString())), nil
		},
	})
}

// boolListFunc returns alltrue when all is true, else anytrue.
func boolListFunc(all bool) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "list", Type: cty.List(cty.Bool)}},
		Type:   function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			for it := args[0].ElementIterator(); it.Next(); {
				_, v := it.Element()
				if v.IsNull() {
					continue
				}
				if v.True() != all {
					return cty.BoolVal(!all), nil
				}
			}
			return cty.BoolVal(all), nil
		},
	})
}
