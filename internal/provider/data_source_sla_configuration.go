package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &SLAConfigurationDataSource{}
var _ datasource.DataSourceWithConfigure = &SLAConfigurationDataSource{}
var _ datasource.DataSourceWithConfigValidators = &SLAConfigurationDataSource{}

func NewSLAConfigurationDataSource() datasource.DataSource {
	return &SLAConfigurationDataSource{}
}

// SLAConfigurationDataSource looks up an SLA configuration.
type SLAConfigurationDataSource struct {
	client *defectdojo.Client
}

func (d *SLAConfigurationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sla_configuration"
}

func (d *SLAConfigurationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	days := func(severity string) schema.Int64Attribute {
		return schema.Int64Attribute{
			MarkdownDescription: "The number of days in which " + severity + " findings must be remediated.",
			Computed:            true,
		}
	}
	enforce := func(severity string) schema.BoolAttribute {
		return schema.BoolAttribute{
			MarkdownDescription: "Whether " + severity + " findings get an SLA expiration date.",
			Computed:            true,
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a DefectDojo SLA configuration by ID or name, " +
			"such as the `Default` one that DefectDojo creates.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the SLA configuration to look up. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the SLA configuration to look up, compared exactly. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the SLA configuration.",
				Computed:            true,
			},
			"critical":         days("critical"),
			"enforce_critical": enforce("critical"),
			"high":             days("high"),
			"enforce_high":     enforce("high"),
			"medium":           days("medium"),
			"enforce_medium":   enforce("medium"),
			"low":              days("low"),
			"enforce_low":      enforce("low"),
			"restart_sla_on_reactivation": schema.BoolAttribute{
				MarkdownDescription: "Whether findings that a reimport reactivates get a new SLA period.",
				Computed:            true,
			},
		},
	}
}

func (d *SLAConfigurationDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *SLAConfigurationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *SLAConfigurationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SLAConfigurationResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		sla  *defectdojo.SLAConfiguration
		err  error
		what string
	)
	if !data.ID.IsNull() {
		what = fmt.Sprintf("ID %d", data.ID.ValueInt64())
		sla, _, err = d.client.SLAConfigurations.Get(ctx, int(data.ID.ValueInt64()), nil)
	} else {
		what = fmt.Sprintf("name %q", data.Name.ValueString())
		sla, _, err = d.client.SLAConfigurations.GetByName(ctx, data.Name.ValueString(), nil)
	}
	if err != nil {
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.Diagnostics.AddError(
				"DefectDojo SLA configuration not found",
				"There is no SLA configuration with "+what+".",
			)
			return
		}
		resp.Diagnostics.AddError("Error reading DefectDojo SLA configuration", "Could not read the SLA configuration with "+what+": "+err.Error())
		return
	}

	updateSLAConfigurationModel(&data, sla)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
