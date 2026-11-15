package provider

import (
	"context"
	"iter"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// listNotes ends the description of every list resource.
const listNotes = "\n\nRequires Terraform 1.14 or later. Terraform returns at most 100 results unless the `list` block sets `limit`. " +
	"`terraform query -generate-config-out` writes every attribute that has a value, including those the provider otherwise leaves to others " +
	"when they are not configured (such as `authorized_users` and `tags`): remove the ones another system manages before applying the generated configuration."

var _ list.ListResourceWithConfigure = &listResource[struct{}, struct{}]{}

// listResource is a list resource of the objects of type T, configured by
// the model C. The list resources differ only in these fields, which each
// New…ListResource fills in.
type listResource[C, T any] struct {
	client *defectdojo.Client

	// typeName follows the provider's type name, such as "_product_type":
	// the type name of the managed resource.
	typeName string
	schema   schema.Schema

	// objects names the objects in error messages, such as "product types".
	objects string

	// all returns the objects that DefectDojo's filters select, as the
	// client's lazy ListAll does.
	all func(ctx context.Context, client *defectdojo.Client, config C) iter.Seq2[*T, error]

	id func(*T) int

	// keep reports whether an object matches the filters DefectDojo can't
	// apply; nil keeps every object.
	keep func(object *T, config C) bool

	// results returns the function that builds the result of each object,
	// for one listing: it can keep what several objects need, such as a
	// product they belong to.
	results func(ctx context.Context, client *defectdojo.Client, req list.ListRequest, config C) func(*T) list.ListResult

	// failure returns the diagnostics of an error of all; nil uses
	// listError.
	failure func(err error, config C) diag.Diagnostics
}

func (r *listResource[C, T]) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + r.typeName
}

func (r *listResource[C, T]) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = r.schema
}

func (r *listResource[C, T]) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := clientFromProviderData(req.ProviderData, "List Resource", &resp.Diagnostics); client != nil {
		r.client = client
	}
}

func (r *listResource[C, T]) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config C
	if diags := req.Config.Get(ctx, &config); diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}
	var keep func(*T) bool
	if r.keep != nil {
		keep = func(object *T) bool { return r.keep(object, config) }
	}
	failure := func(err error) diag.Diagnostics { return listError(r.objects)(err) }
	if r.failure != nil {
		failure = func(err error) diag.Diagnostics { return r.failure(err, config) }
	}
	stream.Results = listResults(r.all(ctx, r.client, config), r.id, keep, r.results(ctx, r.client, req, config), failure)
}

// modelResults returns the results function of a list resource whose state
// is the model M filled by update, the resource's own model function, as
// Read does after an import.
func modelResults[C, T, M any](id func(*T) int, displayName func(*T) string, update func(*M, *T)) func(context.Context, *defectdojo.Client, list.ListRequest, C) func(*T) list.ListResult {
	return func(ctx context.Context, _ *defectdojo.Client, req list.ListRequest, _ C) func(*T) list.ListResult {
		return func(object *T) list.ListResult {
			var state M
			update(&state, object)
			return newListResult(ctx, req, displayName(object), id(object), state)
		}
	}
}

// listContainsAttribute is an optional filter that matches a part of a value,
// ignoring case.
func listContainsAttribute(objects, value string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "Only list the " + objects + " whose " + value + " contains this text, ignoring case.",
		Optional:            true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
			noSurroundingWhitespace(),
		},
	}
}

// listIDAttribute is an optional filter by the ID of another object.
func listIDAttribute(description string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: description,
		Optional:            true,
		Validators:          []validator.Int64{int64validator.AtLeast(1)},
	}
}

// NameListModel describes the configuration of a list resource whose only
// filter is name_contains.
type NameListModel struct {
	NameContains types.String `tfsdk:"name_contains"`
}

// nameListResource returns a list resource of objects with a name, filtered
// by name_contains on the client side (DefectDojo's name filters are exact, if
// it has any), whose state is the model M filled by update and whose display
// name is the name.
func nameListResource[T, M any](typeName, objects, description string,
	all func(ctx context.Context, client *defectdojo.Client) iter.Seq2[*T, error],
	id func(*T) int, name func(*T) string, update func(*M, *T),
) *listResource[NameListModel, T] {
	return &listResource[NameListModel, T]{
		typeName: typeName,
		schema: schema.Schema{
			MarkdownDescription: description + listNotes,
			Attributes: map[string]schema.Attribute{
				"name_contains": listContainsAttribute(objects, "name"),
			},
		},
		objects: objects,
		all: func(ctx context.Context, client *defectdojo.Client, _ NameListModel) iter.Seq2[*T, error] {
			return all(ctx, client)
		},
		id:      id,
		keep:    func(object *T, config NameListModel) bool { return containsFold(name(object), config.NameContains) },
		results: modelResults[NameListModel](id, name, update),
	}
}

// sortedByID returns all objects of all ordered by ID. It reads every object
// before returning the first: for collections whose order isn't total, where
// paging by offset could skip objects.
func sortedByID[T any](all iter.Seq2[*T, error], id func(*T) int) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		var objects []*T
		for object, err := range all {
			if err != nil {
				yield(nil, err)
				return
			}
			objects = append(objects, object)
		}
		slices.SortFunc(objects, func(a, b *T) int { return id(a) - id(b) })
		for _, object := range objects {
			if !yield(object, nil) {
				return
			}
		}
	}
}

// containsFold reports whether value contains filter, ignoring case. A null
// filter matches every value.
func containsFold(value string, filter types.String) bool {
	return filter.IsNull() || strings.Contains(strings.ToLower(value), strings.ToLower(filter.ValueString()))
}

// newListResult returns a result with the identity and display name of the
// object with the ID id, and its state when Terraform requested it. The state
// must be what an import of the object followed by Read stores, so that
// generated configuration plans no changes.
func newListResult(ctx context.Context, req list.ListRequest, displayName string, id int, state any) list.ListResult {
	result := req.NewListResult(ctx)
	result.DisplayName = displayName
	result.Diagnostics.Append(result.Identity.Set(ctx, IDIdentityModel{ID: types.Int64Value(int64(id))})...)
	if req.IncludeResource {
		result.Diagnostics.Append(result.Resource.Set(ctx, state)...)
	}
	return result
}

// errorResult returns a result that only holds diagnostics.
func errorResult(diags diag.Diagnostics) list.ListResult {
	return list.ListResult{Diagnostics: diags}
}

// listResults streams the result of each object of all, the client's lazy
// ListAll, that keep accepts (nil accepts all). Paging by offset can return
// an object twice when objects are added or deleted meanwhile, so objects
// already streamed are skipped: two results for one object would generate two
// import blocks. An error of all ends the stream with a result that holds
// the diagnostics of failure, and so does a result with an error. Terraform
// stops the stream once it has the number of results it requested (the limit
// of the list block).
func listResults[T any](all iter.Seq2[*T, error], id func(*T) int, keep func(*T) bool, result func(*T) list.ListResult, failure func(error) diag.Diagnostics) iter.Seq[list.ListResult] {
	return func(push func(list.ListResult) bool) {
		seen := map[int]bool{}
		for object, err := range all {
			if err != nil {
				push(errorResult(failure(err)))
				return
			}
			if seen[id(object)] || keep != nil && !keep(object) {
				continue
			}
			seen[id(object)] = true
			r := result(object)
			// An error, such as a product that can't be read, would
			// repeat for the objects that follow: it ends the stream.
			if !push(r) || r.Diagnostics.HasError() {
				return
			}
		}
	}
}

// listError returns the diagnostics of an error that ended a list.
func listError(objects string) func(error) diag.Diagnostics {
	return func(err error) diag.Diagnostics {
		var diags diag.Diagnostics
		diags.AddError("Error listing DefectDojo "+objects, "Could not list the "+objects+": "+err.Error())
		return diags
	}
}
