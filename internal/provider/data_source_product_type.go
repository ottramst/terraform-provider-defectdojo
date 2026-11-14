package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ProductTypeDataSource{}
var _ datasource.DataSourceWithConfigure = &ProductTypeDataSource{}
var _ datasource.DataSourceWithConfigValidators = &ProductTypeDataSource{}

func NewProductTypeDataSource() datasource.DataSource {
	return &ProductTypeDataSource{}
}

// ProductTypeDataSource looks up a product type.
type ProductTypeDataSource struct {
	client *defectdojo.Client
}

// ProductTypeDataSourceModel describes the defectdojo_product_type data source.
type ProductTypeDataSourceModel struct {
	ID              types.Int64  `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	CriticalProduct types.Bool   `tfsdk:"critical_product"`
	KeyProduct      types.Bool   `tfsdk:"key_product"`
	AuthorizedUsers types.Set    `tfsdk:"authorized_users"`
}

func (d *ProductTypeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product_type"
}

func (d *ProductTypeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a DefectDojo product type by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the product type to look up. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the product type to look up, compared exactly. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the product type.",
				Computed:            true,
			},
			"critical_product": schema.BoolAttribute{
				MarkdownDescription: "Whether the products of this type are critical.",
				Computed:            true,
			},
			"key_product": schema.BoolAttribute{
				MarkdownDescription: "Whether the products of this type are key products.",
				Computed:            true,
			},
			"authorized_users": schema.SetAttribute{
				MarkdownDescription: "The IDs of the users who may access the product type and its products, in addition to superusers and staff.",
				ElementType:         types.Int64Type,
				Computed:            true,
			},
		},
	}
}

func (d *ProductTypeDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *ProductTypeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *ProductTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ProductTypeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		pt   *defectdojo.ProductType
		err  error
		what string
	)
	if !data.ID.IsNull() {
		what = fmt.Sprintf("ID %d", data.ID.ValueInt64())
		pt, _, err = d.client.ProductTypes.Get(ctx, int(data.ID.ValueInt64()))
	} else {
		what = fmt.Sprintf("name %q", data.Name.ValueString())
		pt, _, err = d.client.ProductTypes.GetByName(ctx, data.Name.ValueString())
	}
	if err != nil {
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.Diagnostics.AddError(
				"DefectDojo product type not found",
				"There is no product type with "+what+", or the provider user may not see it.",
			)
			return
		}
		resp.Diagnostics.AddError("Error reading DefectDojo product type", "Could not read the product type with "+what+": "+err.Error())
		return
	}

	data.ID = types.Int64Value(int64(pt.ID))
	data.Name = types.StringValue(pt.Name)
	data.Description = stringOrNull(pt.Description)
	data.CriticalProduct = types.BoolValue(pt.CriticalProduct)
	data.KeyProduct = types.BoolValue(pt.KeyProduct)
	data.AuthorizedUsers = int64Set(pt.AuthorizedUsers)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
