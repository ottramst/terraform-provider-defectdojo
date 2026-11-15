package provider

import (
	"cmp"
	"context"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &TestTypesDataSource{}
var _ datasource.DataSourceWithConfigure = &TestTypesDataSource{}

func NewTestTypesDataSource() datasource.DataSource {
	return &TestTypesDataSource{}
}

// TestTypesDataSource lists the test types.
type TestTypesDataSource struct {
	client *defectdojo.Client
}

// TestTypesDataSourceModel describes the defectdojo_test_types data source.
type TestTypesDataSourceModel struct {
	Name      types.String `tfsdk:"name"`
	TestTypes types.List   `tfsdk:"test_types"`
	Names     types.List   `tfsdk:"names"`
}

// testTypeAttrTypes are the attributes of an element of test_types.
var testTypeAttrTypes = map[string]attr.Type{
	"id":           types.Int64Type,
	"name":         types.StringType,
	"static_tool":  types.BoolType,
	"dynamic_tool": types.BoolType,
	"active":       types.BoolType,
}

func (d *TestTypesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_types"
}

func (d *TestTypesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to list DefectDojo's test types: the kinds of tests, among them one per scanner DefectDojo can import, " +
			"such as `Trivy Scan`. For example, check that a scan type exists before a pipeline imports it:\n\n" +
			"```terraform\n" +
			"data \"defectdojo_test_types\" \"all\" {}\n\n" +
			"check \"trivy\" {\n" +
			"  assert {\n" +
			"    condition     = contains(data.defectdojo_test_types.all.names, \"Trivy Scan\")\n" +
			"    error_message = \"DefectDojo has no Trivy Scan test type.\"\n" +
			"  }\n" +
			"}\n" +
			"```",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Lists only the test type with exactly this name, compared case-sensitively. Without it, all test types are listed. " +
					"An unknown name gives empty lists, not an error.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"test_types": schema.ListNestedAttribute{
				MarkdownDescription: "The test types, ordered by name, case-sensitively, as Terraform's `sort()` does: names starting with a lowercase letter come last.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							MarkdownDescription: "The ID of the test type.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the test type, such as `Trivy Scan`.",
							Computed:            true,
						},
						"static_tool": schema.BoolAttribute{
							MarkdownDescription: "Whether the test type is a static analysis tool.",
							Computed:            true,
						},
						"dynamic_tool": schema.BoolAttribute{
							MarkdownDescription: "Whether the test type is a dynamic analysis tool.",
							Computed:            true,
						},
						"active": schema.BoolAttribute{
							MarkdownDescription: "Whether the test type is offered for new tests.",
							Computed:            true,
						},
					},
				},
			},
			"names": schema.ListAttribute{
				MarkdownDescription: "The names of the test types, in the order of `test_types`.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *TestTypesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *TestTypesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data TestTypesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var testTypes []*defectdojo.TestType
	for tt, err := range d.client.TestTypes.ListAll(ctx, &defectdojo.ListTestTypesOptions{Name: data.Name.ValueString()}) {
		if err != nil {
			resp.Diagnostics.AddError("Error reading DefectDojo test types", "Could not list the test types: "+err.Error())
			return
		}
		// DefectDojo strips surrounding whitespace from the filter, so check
		// the name again.
		if data.Name.IsNull() || tt.Name == data.Name.ValueString() {
			testTypes = append(testTypes, tt)
		}
	}
	slices.SortFunc(testTypes, func(a, b *defectdojo.TestType) int { return cmp.Compare(a.Name, b.Name) })

	objects := make([]attr.Value, 0, len(testTypes))
	names := make([]attr.Value, 0, len(testTypes))
	for _, tt := range testTypes {
		object, diags := types.ObjectValue(testTypeAttrTypes, map[string]attr.Value{
			"id":           types.Int64Value(int64(tt.ID)),
			"name":         types.StringValue(tt.Name),
			"static_tool":  types.BoolValue(tt.StaticTool),
			"dynamic_tool": types.BoolValue(tt.DynamicTool),
			"active":       types.BoolValue(tt.Active),
		})
		resp.Diagnostics.Append(diags...)
		objects = append(objects, object)
		names = append(names, types.StringValue(tt.Name))
	}
	testTypeList, diags := types.ListValue(types.ObjectType{AttrTypes: testTypeAttrTypes}, objects)
	resp.Diagnostics.Append(diags...)
	nameList, diags := types.ListValue(types.StringType, names)
	resp.Diagnostics.Append(diags...)
	data.TestTypes, data.Names = testTypeList, nameList
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
