package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// schemaBaseline is the schema of the latest release. Only the release pull
// request regenerates it, with
//
//	go test ./internal/provider -run TestSchemaCompatibility -update
var schemaBaseline = "testdata/schema-baseline.json"

var updateBaseline = flag.Bool("update", false, "regenerate "+schemaBaseline+" (release pull requests only)")

// schemaDump is the part of the provider's schemas that its compatibility
// depends on. Descriptions are left out: changing them is compatible.
type schemaDump struct {
	Provider      map[string]schemaAttribute `json:"provider"`
	Resources     map[string]schemaDumpType  `json:"resources"`
	DataSources   map[string]schemaDumpType  `json:"data_sources"`
	ListResources map[string]schemaDumpType  `json:"list_resources"`
}

type schemaDumpType struct {
	Version    int64                      `json:"version,omitempty"`
	Attributes map[string]schemaAttribute `json:"attributes"`
	Identity   *identityDump              `json:"identity,omitempty"`
}

type identityDump struct {
	Version    int64                      `json:"version,omitempty"`
	Attributes map[string]schemaAttribute `json:"attributes"`
}

// schemaAttribute is an attribute, or the attribute of a nested attribute or
// block, named by its path ("a.b").
type schemaAttribute struct {
	Type              json.RawMessage `json:"type,omitempty"`
	Nesting           string          `json:"nesting,omitempty"`
	Required          bool            `json:"required,omitempty"`
	Optional          bool            `json:"optional,omitempty"`
	Computed          bool            `json:"computed,omitempty"`
	Sensitive         bool            `json:"sensitive,omitempty"`
	WriteOnly         bool            `json:"write_only,omitempty"`
	Deprecated        bool            `json:"deprecated,omitempty"`
	RequiredForImport bool            `json:"required_for_import,omitempty"`
	OptionalForImport bool            `json:"optional_for_import,omitempty"`

	// Default is the description of the default (such as "value defaults
	// to false"), which the protocol schema doesn't have.
	Default string `json:"default,omitempty"`

	// RequiresReplace is true when a change of the attribute can replace the
	// resource: it has one of the framework's RequiresReplace,
	// RequiresReplaceIf or RequiresReplaceIfConfigured plan modifiers.
	// Replacement must only be forced by them, never by resp.RequiresReplace
	// in ModifyPlan or by a modifier of the provider's own, which this can't
	// see.
	RequiresReplace bool `json:"requires_replace,omitempty"`
}

// dumpSchemas returns the schemas of the provider, as Terraform sees them
// (GetProviderSchema and GetResourceIdentitySchemas), with the defaults and
// requires-replace plan modifiers of its resources' attributes.
func dumpSchemas(t *testing.T) schemaDump {
	t.Helper()
	ctx := context.Background()
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	identities, err := server.GetResourceIdentitySchemas(ctx, &tfprotov6.GetResourceIdentitySchemasRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range append(schemas.Diagnostics, identities.Diagnostics...) {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("%s: %s", d.Summary, d.Detail)
		}
	}

	dump := schemaDump{
		Provider:      blockAttributes(t, schemas.Provider.Block),
		Resources:     map[string]schemaDumpType{},
		DataSources:   map[string]schemaDumpType{},
		ListResources: map[string]schemaDumpType{},
	}
	for name, s := range schemas.ResourceSchemas {
		dump.Resources[name] = schemaDumpType{Version: s.Version, Attributes: blockAttributes(t, s.Block)}
	}
	for name, s := range schemas.DataSourceSchemas {
		dump.DataSources[name] = schemaDumpType{Version: s.Version, Attributes: blockAttributes(t, s.Block)}
	}
	for name, s := range schemas.ListResourceSchemas {
		dump.ListResources[name] = schemaDumpType{Version: s.Version, Attributes: blockAttributes(t, s.Block)}
	}
	for name, s := range identities.IdentitySchemas {
		r, ok := dump.Resources[name]
		if !ok {
			t.Fatalf("identity schema of the unknown resource %s", name)
		}
		identity := &identityDump{Version: s.Version, Attributes: map[string]schemaAttribute{}}
		for _, a := range s.IdentityAttributes {
			identity.Attributes[a.Name] = schemaAttribute{
				Type:              typeJSON(t, a.Type),
				RequiredForImport: a.RequiredForImport,
				OptionalForImport: a.OptionalForImport,
			}
		}
		r.Identity = identity
		dump.Resources[name] = r
	}

	// Defaults and plan modifiers are not part of the protocol schema: they
	// are read from the framework schemas.
	p := New("test")()
	var meta fwprovider.MetadataResponse
	p.Metadata(ctx, fwprovider.MetadataRequest{}, &meta)
	for _, newResource := range p.Resources(ctx) {
		r := newResource()
		var rmeta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: meta.TypeName}, &rmeta)
		var resp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &resp)
		d, ok := dump.Resources[rmeta.TypeName]
		if !ok {
			t.Fatalf("resource %s is missing from GetProviderSchema", rmeta.TypeName)
		}
		if len(resp.Schema.Blocks) > 0 {
			t.Fatalf("resource %s has blocks, whose defaults and plan modifiers dumpSchemas doesn't read", rmeta.TypeName)
		}
		for name, attr := range resp.Schema.Attributes {
			a := d.Attributes[name]
			if a.Nesting != "" {
				t.Fatalf("resource %s has the nested attribute %s, whose defaults and plan modifiers dumpSchemas doesn't read", rmeta.TypeName, name)
			}
			a.Default, a.RequiresReplace = frameworkModifiers(ctx, t, rmeta.TypeName+"."+name, attr)
			d.Attributes[name] = a
		}
	}
	return dump
}

// blockAttributes returns the attributes of a block, with those of nested
// attributes and blocks under their paths.
func blockAttributes(t *testing.T, b *tfprotov6.SchemaBlock) map[string]schemaAttribute {
	t.Helper()
	attributes := map[string]schemaAttribute{}
	var walk func(prefix string, attrs []*tfprotov6.SchemaAttribute, blocks []*tfprotov6.SchemaNestedBlock)
	walk = func(prefix string, attrs []*tfprotov6.SchemaAttribute, blocks []*tfprotov6.SchemaNestedBlock) {
		for _, a := range attrs {
			attr := schemaAttribute{
				Required:   a.Required,
				Optional:   a.Optional,
				Computed:   a.Computed,
				Sensitive:  a.Sensitive,
				WriteOnly:  a.WriteOnly,
				Deprecated: a.Deprecated,
			}
			if a.NestedType != nil {
				attr.Nesting = a.NestedType.Nesting.String()
				walk(prefix+a.Name+".", a.NestedType.Attributes, nil)
			} else {
				attr.Type = typeJSON(t, a.Type)
			}
			attributes[prefix+a.Name] = attr
		}
		for _, nb := range blocks {
			attributes[prefix+nb.TypeName] = schemaAttribute{Nesting: nb.Nesting.String(), Deprecated: nb.Block.Deprecated}
			walk(prefix+nb.TypeName+".", nb.Block.Attributes, nb.Block.BlockTypes)
		}
	}
	if b != nil {
		walk("", b.Attributes, b.BlockTypes)
	}
	return attributes
}

// typeJSON returns the JSON type constraint of a type, as Terraform writes it.
func typeJSON(t *testing.T, typ interface{ MarshalJSON() ([]byte, error) }) json.RawMessage {
	t.Helper()
	b, err := typ.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// frameworkModifiers returns the description of the default of a framework
// resource attribute, and whether a plan modifier can force replacement. The
// framework's attribute types have typed methods (StringDefaultValue,
// StringPlanModifiers, ...), so they are found by name. The framework's
// RequiresReplace modifiers of every type are one unexported type per
// package, requiresReplaceIfModifier, which recognizes them whatever their
// description.
func frameworkModifiers(ctx context.Context, t *testing.T, path string, attr any) (string, bool) {
	t.Helper()
	type described interface {
		Description(context.Context) string
	}
	v := reflect.ValueOf(attr)
	var def string
	var requiresReplace bool
	for i := range v.NumMethod() {
		name := v.Type().Method(i).Name
		switch {
		case strings.HasSuffix(name, "DefaultValue"):
			out := v.Method(i).Call(nil)[0]
			if out.IsNil() {
				continue
			}
			d, ok := out.Interface().(described)
			if !ok {
				t.Fatalf("%s: the default %T has no description", path, out.Interface())
			}
			def = d.Description(ctx)
		case strings.HasSuffix(name, "PlanModifiers"):
			mods := v.Method(i).Call(nil)[0]
			for j := range mods.Len() {
				if reflect.TypeOf(mods.Index(j).Interface()).Name() == "requiresReplaceIfModifier" {
					requiresReplace = true
				}
			}
		}
	}
	return def, requiresReplace
}

// TestSchemaCompatibility fails when the provider's schemas changed in a way
// that can break configurations or states of the latest release (the
// baseline): from v1.0.0, such a change needs a new major version. Additions
// are compatible. Run it with -update only in a release pull request.
func TestSchemaCompatibility(t *testing.T) {
	current := dumpSchemas(t)
	if *updateBaseline {
		b, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(schemaBaseline, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(schemaBaseline)
	if err != nil {
		t.Fatal(err)
	}
	var baseline schemaDump
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&baseline); err != nil {
		t.Fatal(err)
	}
	// The indented file reformats the type constraints.
	baseline.compactTypes(t)
	if problems := schemaIncompatibilities(baseline, current); len(problems) > 0 {
		t.Errorf("the schema is incompatible with %s, the latest release:\n%s", schemaBaseline, strings.Join(problems, "\n"))
	}
}

// compactTypes removes the whitespace from the type constraints.
func (d *schemaDump) compactTypes(t *testing.T) {
	t.Helper()
	compact := func(attrs map[string]schemaAttribute) {
		for name, a := range attrs {
			if a.Type == nil {
				continue
			}
			var buf bytes.Buffer
			if err := json.Compact(&buf, a.Type); err != nil {
				t.Fatal(err)
			}
			a.Type = buf.Bytes()
			attrs[name] = a
		}
	}
	compact(d.Provider)
	for _, types := range []map[string]schemaDumpType{d.Resources, d.DataSources, d.ListResources} {
		for _, typ := range types {
			compact(typ.Attributes)
			if typ.Identity != nil {
				compact(typ.Identity.Attributes)
			}
		}
	}
}

// schemaIncompatibilities lists the changes from base to cur that can break
// configurations or states written for base.
func schemaIncompatibilities(base, cur schemaDump) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	compareAttributes(add, "provider", base.Provider, cur.Provider)
	for _, kind := range []struct {
		name      string
		base, cur map[string]schemaDumpType
	}{
		{"resource", base.Resources, cur.Resources},
		{"data source", base.DataSources, cur.DataSources},
		{"list resource", base.ListResources, cur.ListResources},
	} {
		for _, name := range sortedKeys(kind.base) {
			o := kind.base[name]
			n, ok := kind.cur[name]
			if !ok {
				add("%s %s: removed", kind.name, name)
				continue
			}
			if n.Version < o.Version {
				add("%s %s: schema version lowered from %d to %d", kind.name, name, o.Version, n.Version)
			}
			compareAttributes(add, kind.name+" "+name, o.Attributes, n.Attributes)
			if o.Identity != nil && !reflect.DeepEqual(o.Identity, n.Identity) {
				add("%s %s: identity schema changed", kind.name, name)
			}
		}
	}
	return problems
}

func compareAttributes(add func(string, ...any), where string, base, cur map[string]schemaAttribute) {
	for _, name := range sortedKeys(base) {
		o := base[name]
		n, ok := cur[name]
		if !ok {
			add("%s: attribute %s removed", where, name)
			continue
		}
		at := where + ": attribute " + name
		switch {
		case !bytes.Equal(o.Type, n.Type) || o.Nesting != n.Nesting:
			add("%s: type changed from %s%s to %s%s", at, o.Type, o.Nesting, n.Type, n.Nesting)
		case n.Required && !o.Required:
			add("%s: became required", at)
		case (o.Optional || o.Required) && !n.Optional && !n.Required:
			add("%s: can no longer be set", at)
		case o.Computed && !n.Computed:
			add("%s: is no longer computed", at)
		case o.Optional && !o.Computed && n.Computed:
			// Null cleared the value; with Computed, it keeps it.
			add("%s: became computed, which changes what null means", at)
		}
		if n.Sensitive && !o.Sensitive {
			add("%s: became sensitive", at)
		}
		if n.WriteOnly && !o.WriteOnly {
			add("%s: became write-only", at)
		}
		// Only write-only attributes take ephemeral values.
		if o.WriteOnly && !n.WriteOnly {
			add("%s: is no longer write-only", at)
		}
		if o.Default != n.Default {
			add("%s: default changed from %q to %q", at, o.Default, n.Default)
		}
		if n.RequiresReplace && !o.RequiresReplace {
			add("%s: now forces replacement", at)
		}
	}
	for _, name := range sortedKeys(cur) {
		if _, ok := base[name]; !ok && cur[name].Required {
			add("%s: new attribute %s is required", where, name)
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestSchemaIncompatibilities(t *testing.T) {
	str := json.RawMessage(`"string"`)
	num := json.RawMessage(`"number"`)
	base := func() schemaDump {
		return schemaDump{
			Provider: map[string]schemaAttribute{"url": {Type: str, Optional: true}},
			Resources: map[string]schemaDumpType{
				"defectdojo_thing": {
					Attributes: map[string]schemaAttribute{
						"id":            {Type: num, Computed: true},
						"name":          {Type: str, Required: true},
						"description":   {Type: str, Optional: true},
						"enabled":       {Type: str, Optional: true, Computed: true, Default: "value defaults to true"},
						"kind":          {Type: str, Optional: true, Computed: true, RequiresReplace: true},
						"secret_wo":     {Type: str, Optional: true, Sensitive: true, WriteOnly: true},
						"deprecated":    {Type: str, Optional: true},
						"force_destroy": {Type: str, Optional: true, Computed: true},
					},
					Identity: &identityDump{Attributes: map[string]schemaAttribute{"id": {Type: num, RequiredForImport: true}}},
				},
			},
			DataSources:   map[string]schemaDumpType{"defectdojo_thing": {Attributes: map[string]schemaAttribute{"id": {Type: num, Optional: true}}}},
			ListResources: map[string]schemaDumpType{"defectdojo_thing": {Attributes: map[string]schemaAttribute{"name_contains": {Type: str, Optional: true}}}},
		}
	}
	attr := func(d schemaDump, name string, change func(*schemaAttribute)) {
		a := d.Resources["defectdojo_thing"].Attributes[name]
		change(&a)
		d.Resources["defectdojo_thing"].Attributes[name] = a
	}
	tests := []struct {
		name   string
		change func(d *schemaDump)
		want   string // "" for a compatible change
	}{
		{"unchanged", func(*schemaDump) {}, ""},
		{"new optional attribute", func(d *schemaDump) {
			d.Resources["defectdojo_thing"].Attributes["new"] = schemaAttribute{Type: str, Optional: true}
		}, ""},
		{"new resource", func(d *schemaDump) {
			d.Resources["defectdojo_other"] = schemaDumpType{Attributes: map[string]schemaAttribute{"id": {Type: num, Computed: true}}}
		}, ""},
		{"deprecated", func(d *schemaDump) { attr(*d, "deprecated", func(a *schemaAttribute) { a.Deprecated = true }) }, ""},
		{"required to optional", func(d *schemaDump) {
			attr(*d, "name", func(a *schemaAttribute) { a.Required, a.Optional = false, true })
		}, ""},
		{"replacement removed", func(d *schemaDump) { attr(*d, "kind", func(a *schemaAttribute) { a.RequiresReplace = false }) }, ""},
		{"schema version raised", func(d *schemaDump) {
			r := d.Resources["defectdojo_thing"]
			r.Version = 1
			d.Resources["defectdojo_thing"] = r
		}, ""},
		{"resource removed", func(d *schemaDump) { delete(d.Resources, "defectdojo_thing") }, "resource defectdojo_thing: removed"},
		{"data source removed", func(d *schemaDump) { delete(d.DataSources, "defectdojo_thing") }, "data source defectdojo_thing: removed"},
		{"list resource attribute removed", func(d *schemaDump) {
			delete(d.ListResources["defectdojo_thing"].Attributes, "name_contains")
		}, "list resource defectdojo_thing: attribute name_contains removed"},
		{"provider attribute required", func(d *schemaDump) {
			d.Provider["url"] = schemaAttribute{Type: str, Required: true}
		}, "provider: attribute url: became required"},
		{"attribute removed", func(d *schemaDump) {
			delete(d.Resources["defectdojo_thing"].Attributes, "description")
		}, "attribute description removed"},
		{"type changed", func(d *schemaDump) { attr(*d, "description", func(a *schemaAttribute) { a.Type = num }) }, "type changed"},
		{"optional to required", func(d *schemaDump) {
			attr(*d, "description", func(a *schemaAttribute) { a.Optional, a.Required = false, true })
		}, "became required"},
		{"required to computed", func(d *schemaDump) {
			attr(*d, "name", func(a *schemaAttribute) { a.Required, a.Computed = false, true })
		}, "can no longer be set"},
		{"optional to computed", func(d *schemaDump) {
			attr(*d, "description", func(a *schemaAttribute) { a.Optional, a.Computed = false, true })
		}, "can no longer be set"},
		{"write-only removed", func(d *schemaDump) { attr(*d, "secret_wo", func(a *schemaAttribute) { a.WriteOnly = false }) }, "no longer write-only"},
		{"no longer computed", func(d *schemaDump) {
			attr(*d, "force_destroy", func(a *schemaAttribute) { a.Computed = false })
		}, "is no longer computed"},
		{"computed added", func(d *schemaDump) { attr(*d, "description", func(a *schemaAttribute) { a.Computed = true }) }, "became computed"},
		{"sensitive added", func(d *schemaDump) { attr(*d, "description", func(a *schemaAttribute) { a.Sensitive = true }) }, "became sensitive"},
		{"write-only added", func(d *schemaDump) { attr(*d, "description", func(a *schemaAttribute) { a.WriteOnly = true }) }, "became write-only"},
		{"default changed", func(d *schemaDump) {
			attr(*d, "enabled", func(a *schemaAttribute) { a.Default = "value defaults to false" })
		}, "default changed"},
		{"default added", func(d *schemaDump) {
			attr(*d, "force_destroy", func(a *schemaAttribute) { a.Default = "value defaults to false" })
		}, "default changed"},
		{"replacement added", func(d *schemaDump) {
			attr(*d, "description", func(a *schemaAttribute) { a.RequiresReplace = true })
		}, "now forces replacement"},
		{"new required attribute", func(d *schemaDump) {
			d.Resources["defectdojo_thing"].Attributes["new"] = schemaAttribute{Type: str, Required: true}
		}, "new attribute new is required"},
		{"identity changed", func(d *schemaDump) {
			d.Resources["defectdojo_thing"].Identity.Attributes["id"] = schemaAttribute{Type: str, RequiredForImport: true}
		}, "identity schema changed"},
		{"schema version lowered", func(d *schemaDump) {
			old := d.Resources["defectdojo_thing"]
			old.Version = -1
			d.Resources["defectdojo_thing"] = old
		}, "schema version lowered"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := base()
			tt.change(&changed)
			problems := schemaIncompatibilities(base(), changed)
			switch {
			case tt.want == "" && len(problems) > 0:
				t.Errorf("want compatible, got %q", problems)
			case tt.want != "" && (len(problems) != 1 || !strings.Contains(problems[0], tt.want)):
				t.Errorf("want one problem containing %q, got %q", tt.want, problems)
			}
		})
	}
}
