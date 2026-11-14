package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

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
