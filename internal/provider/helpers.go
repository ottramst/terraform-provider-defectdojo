package provider

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// clientFromProviderData returns the client the provider passes to resources
// and data sources, or nil when the provider is not configured yet.
func clientFromProviderData(providerData any, kind string, diags *diag.Diagnostics) *defectdojo.Client {
	if providerData == nil {
		return nil
	}
	client, ok := providerData.(*defectdojo.Client)
	if !ok {
		diags.AddError(
			"Unexpected "+kind+" Configure Type",
			fmt.Sprintf("Expected *defectdojo.Client, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil
	}
	return client
}

// configureResource sets the client of a resource.
func configureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *defectdojo.Client {
	return clientFromProviderData(req.ProviderData, "Resource", &resp.Diagnostics)
}

// configureDataSource sets the client of a data source.
func configureDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *defectdojo.Client {
	return clientFromProviderData(req.ProviderData, "Data Source", &resp.Diagnostics)
}

// stringOrNull returns a null string for an empty value: DefectDojo stores an
// unset text field as null or "".
func stringOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

// int64Set returns the IDs as a set of numbers, empty (not null) for none.
func int64Set(ids []int) types.Set {
	elems := make([]attr.Value, len(ids))
	for i, id := range ids {
		elems[i] = types.Int64Value(int64(id))
	}
	return types.SetValueMust(types.Int64Type, elems)
}

// intsFromSet returns the numbers of a known set.
func intsFromSet(ctx context.Context, set types.Set, diags *diag.Diagnostics) []int {
	var ids []int64
	diags.Append(set.ElementsAs(ctx, &ids, false)...)
	out := make([]int, len(ids))
	for i, id := range ids {
		out[i] = int(id)
	}
	return out
}

// parseImportID parses a numeric import ID.
func parseImportID(id, object string, diags *diag.Diagnostics) (int64, bool) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		diags.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected the numeric ID of the %s, got: %q", object, id),
		)
		return 0, false
	}
	return n, true
}

// noSurroundingWhitespace rejects strings with leading or trailing
// whitespace: DefectDojo trims text fields with Python's str.strip, so such a
// value would not match the planned value after apply. Whitespace is ASCII
// and Unicode whitespace, the vertical tab \x0b (which RE2's \s leaves out),
// and the separators \x1c-\x1f and \x85.
func noSurroundingWhitespace() validator.String {
	return stringvalidator.RegexMatches(
		regexp.MustCompile(`(?s)^$|^[^\s\p{Z}\x{0b}\x{1c}-\x{1f}\x{85}](.*[^\s\p{Z}\x{0b}\x{1c}-\x{1f}\x{85}])?$`),
		"must not have leading or trailing whitespace, which DefectDojo removes",
	)
}

// deletePollInterval is how often waitForDeletion checks whether an object is
// gone.
var deletePollInterval = time.Second

// waitForDeletion polls get until it returns defectdojo.ErrNotFound. DefectDojo
// answers a delete before the object is gone when the server deletes in the
// background (DD_ASYNC_OBJECT_DELETE). It gives up when ctx is done.
func waitForDeletion(ctx context.Context, get func(ctx context.Context) error) error {
	for {
		err := get(ctx)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the object still exists: %w", ctx.Err())
		case <-time.After(deletePollInterval):
		}
	}
}

// int64OrNull returns a null number for 0: DefectDojo returns unset
// references as null, which the client decodes as 0.
func int64OrNull(value int) types.Int64 {
	if value == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(int64(value))
}

// nullableString returns the value of an optional text attribute for a
// partial update: null clears it.
func nullableString(value types.String) defectdojo.Nullable[string] {
	if value.IsNull() {
		return defectdojo.Null[string]()
	}
	return defectdojo.Value(value.ValueString())
}

// nullableInt returns the value of an optional reference for a partial
// update: null clears it.
func nullableInt(value types.Int64) defectdojo.Nullable[int] {
	if value.IsNull() {
		return defectdojo.Null[int]()
	}
	return defectdojo.Value(int(value.ValueInt64()))
}

// known reports whether a configured or planned value is set: neither null
// nor unknown.
func known(value attr.Value) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// stringsFromSet returns the strings of a known set.
func stringsFromSet(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	out := []string{}
	diags.Append(set.ElementsAs(ctx, &out, false)...)
	return out
}

// tagSet returns DefectDojo's tags as a set. DefectDojo gives a tag the
// spelling of an existing tag that matches it ignoring case, and write
// responses can list a tag twice; so tags are deduplicated ignoring case, and
// a tag that matches one of reference (the plan, or the prior state) ignoring
// case takes its spelling.
func tagSet(tags []string, reference types.Set) types.Set {
	spelling := map[string]string{}
	if known(reference) {
		for _, e := range reference.Elements() {
			if s, ok := e.(types.String); ok && known(s) {
				spelling[strings.ToLower(s.ValueString())] = s.ValueString()
			}
		}
	}
	seen := map[string]bool{}
	elems := []attr.Value{}
	for _, tag := range tags {
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		if s, ok := spelling[key]; ok {
			tag = s
		}
		elems = append(elems, types.StringValue(tag))
	}
	return types.SetValueMust(types.StringType, elems)
}

// tagValidators returns the validators of a set of tags: DefectDojo splits
// tags on commas, rejects spaces and quotes, drops empty tags, fails on tags
// longer than 255 characters, and merges tags that differ only in case.
func tagValidators() []validator.Set {
	return []validator.Set{
		setvalidator.ValueStringsAre(
			stringvalidator.UTF8LengthBetween(1, 255),
			stringvalidator.RegexMatches(regexp.MustCompile(`^[^ ,'"]*$`), "must not contain spaces, commas or quotes"),
			noSurroundingWhitespace(),
		),
		uniqueIgnoringCase{},
	}
}

// uniqueIgnoringCase rejects sets of strings with two elements that differ
// only in case.
type uniqueIgnoringCase struct{}

func (v uniqueIgnoringCase) Description(_ context.Context) string {
	return "elements must be unique, ignoring case"
}

func (v uniqueIgnoringCase) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v uniqueIgnoringCase) ValidateSet(_ context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if !known(req.ConfigValue) {
		return
	}
	seen := map[string]string{}
	for _, e := range req.ConfigValue.Elements() {
		s, ok := e.(types.String)
		if !ok || !known(s) {
			continue
		}
		key := strings.ToLower(s.ValueString())
		if other, dup := seen[key]; dup {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid Attribute Value",
				fmt.Sprintf("%q and %q differ only in case; DefectDojo stores them as one tag.", other, s.ValueString()))
			return
		}
		seen[key] = s.ValueString()
	}
}

// compareVersions compares two dotted versions such as "3.5.103", as
// cmp.Compare does. Missing parts count as 0, and a suffix such as "-dev" is
// ignored, so that DefectDojo's "3.6.1-dev" compares as "3.6.1".
func compareVersions(a, b string) int {
	a, _, _ = strings.Cut(a, "-")
	b, _, _ = strings.Cut(b, "-")
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if c := cmp.Compare(x, y); c != 0 {
			return c
		}
	}
	return 0
}
