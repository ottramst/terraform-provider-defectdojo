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
var _ datasource.DataSource = &ProductDataSource{}
var _ datasource.DataSourceWithConfigure = &ProductDataSource{}
var _ datasource.DataSourceWithConfigValidators = &ProductDataSource{}

func NewProductDataSource() datasource.DataSource {
	return &ProductDataSource{}
}

// ProductDataSource looks up a product.
type ProductDataSource struct {
	client *defectdojo.Client
}

// ProductDataSourceModel describes the defectdojo_product data source. It has
// the attributes of the resource, without force_destroy.
type ProductDataSourceModel struct {
	ID                            types.Int64  `tfsdk:"id"`
	Name                          types.String `tfsdk:"name"`
	Description                   types.String `tfsdk:"description"`
	ProductTypeID                 types.Int64  `tfsdk:"product_type_id"`
	BusinessCriticality           types.String `tfsdk:"business_criticality"`
	Platform                      types.String `tfsdk:"platform"`
	Lifecycle                     types.String `tfsdk:"life_cycle"`
	Origin                        types.String `tfsdk:"origin"`
	ExternalAudience              types.Bool   `tfsdk:"external_audience"`
	InternetAccessible            types.Bool   `tfsdk:"internet_accessible"`
	EnableProductTagInheritance   types.Bool   `tfsdk:"enable_product_tag_inheritance"`
	EnableSimpleRiskAcceptance    types.Bool   `tfsdk:"enable_simple_risk_acceptance"`
	EnableFullRiskAcceptance      types.Bool   `tfsdk:"enable_full_risk_acceptance"`
	DisableSLABreachNotifications types.Bool   `tfsdk:"disable_sla_breach_notifications"`
	SLAConfigurationID            types.Int64  `tfsdk:"sla_configuration_id"`
	ProductManagerID              types.Int64  `tfsdk:"product_manager_id"`
	TechnicalContactID            types.Int64  `tfsdk:"technical_contact_id"`
	TeamManagerID                 types.Int64  `tfsdk:"team_manager_id"`
	AuthorizedUsers               types.Set    `tfsdk:"authorized_users"`
	RegulationIDs                 types.Set    `tfsdk:"regulation_ids"`
	Tags                          types.Set    `tfsdk:"tags"`
}

func (d *ProductDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

func (d *ProductDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computed := func(description string) schema.Attribute {
		return schema.StringAttribute{MarkdownDescription: description, Computed: true}
	}
	flag := func(description string) schema.Attribute {
		return schema.BoolAttribute{MarkdownDescription: description, Computed: true}
	}
	id := func(description string) schema.Attribute {
		return schema.Int64Attribute{MarkdownDescription: description, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a DefectDojo product by ID or name. DefectDojo 3.x shows products as *assets*.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the product to look up. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the product to look up, compared exactly and case-sensitively. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"description":                      computed("The description of the product."),
			"product_type_id":                  id("The ID of the product type of the product."),
			"business_criticality":             computed("The business criticality of the product, or null."),
			"platform":                         computed("The platform of the product, or null."),
			"life_cycle":                       computed("The lifecycle of the product, or null."),
			"origin":                           computed("The origin of the product, or null."),
			"external_audience":                flag("Whether the product is used by people outside the organization."),
			"internet_accessible":              flag("Whether the product is accessible from the internet."),
			"enable_product_tag_inheritance":   flag("Whether the product's engagements, tests and findings inherit its tags."),
			"enable_simple_risk_acceptance":    flag("Whether findings can be risk-accepted with a checkbox."),
			"enable_full_risk_acceptance":      flag("Whether findings can be risk-accepted with a risk acceptance form."),
			"disable_sla_breach_notifications": flag("Whether the notifications of SLA breaches of the product's findings are disabled."),
			"sla_configuration_id":             id("The ID of the SLA configuration of the product."),
			"product_manager_id":               id("The ID of the product manager, or null."),
			"technical_contact_id":             id("The ID of the technical contact, or null."),
			"team_manager_id":                  id("The ID of the team manager, or null."),
			"authorized_users": schema.SetAttribute{
				MarkdownDescription: "The IDs of the users who may access the product, in addition to superusers, staff and the authorized users of its product type.",
				ElementType:         types.Int64Type,
				Computed:            true,
			},
			"regulation_ids": schema.SetAttribute{
				MarkdownDescription: "The IDs of the regulations that apply to the product.",
				ElementType:         types.Int64Type,
				Computed:            true,
			},
			"tags": schema.SetAttribute{
				MarkdownDescription: "The tags of the product.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *ProductDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *ProductDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *ProductDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ProductDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		p    *defectdojo.Product
		err  error
		what string
	)
	if !data.ID.IsNull() {
		what = fmt.Sprintf("ID %d", data.ID.ValueInt64())
		p, _, err = d.client.Products.Get(ctx, int(data.ID.ValueInt64()))
	} else {
		what = fmt.Sprintf("name %q", data.Name.ValueString())
		p, _, err = d.client.Products.GetByName(ctx, data.Name.ValueString())
	}
	if err != nil {
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.Diagnostics.AddError(
				"DefectDojo product not found",
				"There is no product with "+what+", or the provider user may not see it.",
			)
			return
		}
		resp.Diagnostics.AddError("Error reading DefectDojo product", "Could not read the product with "+what+": "+err.Error())
		return
	}

	var m ProductResourceModel
	updateProductModel(&m, p, types.SetNull(types.StringType))
	data = ProductDataSourceModel{
		ID:                            m.ID,
		Name:                          m.Name,
		Description:                   m.Description,
		ProductTypeID:                 m.ProductTypeID,
		BusinessCriticality:           m.BusinessCriticality,
		Platform:                      m.Platform,
		Lifecycle:                     m.Lifecycle,
		Origin:                        m.Origin,
		ExternalAudience:              m.ExternalAudience,
		InternetAccessible:            m.InternetAccessible,
		EnableProductTagInheritance:   m.EnableProductTagInheritance,
		EnableSimpleRiskAcceptance:    m.EnableSimpleRiskAcceptance,
		EnableFullRiskAcceptance:      m.EnableFullRiskAcceptance,
		DisableSLABreachNotifications: m.DisableSLABreachNotifications,
		SLAConfigurationID:            m.SLAConfigurationID,
		ProductManagerID:              m.ProductManagerID,
		TechnicalContactID:            m.TechnicalContactID,
		TeamManagerID:                 m.TeamManagerID,
		AuthorizedUsers:               m.AuthorizedUsers,
		RegulationIDs:                 m.RegulationIDs,
		Tags:                          m.Tags,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
