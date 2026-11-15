package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &CICDInfrastructureDataSource{}
var _ datasource.DataSourceWithConfigure = &CICDInfrastructureDataSource{}
var _ datasource.DataSourceWithConfigValidators = &CICDInfrastructureDataSource{}

func NewCICDInfrastructureDataSource() datasource.DataSource {
	return &CICDInfrastructureDataSource{}
}

// CICDInfrastructureDataSource looks up a CI/CD infrastructure object.
type CICDInfrastructureDataSource struct {
	client *defectdojo.Client
}

func (d *CICDInfrastructureDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cicd_infrastructure"
}

func (d *CICDInfrastructureDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a DefectDojo CI/CD infrastructure object by ID, or by name and type, " +
			"for example for an engagement's `cicd_build_server_id`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the object to look up. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the object to look up, compared exactly. Names are unique within a type: " +
					"set `infrastructure_type` too when objects of several types have the name. Exactly one of `id` and `name` must be set.",
				Optional: true,
				Computed: true,
			},
			"infrastructure_type": schema.StringAttribute{
				MarkdownDescription: "The type of the object: `scm_server`, `build_server` or `orchestration`. " +
					"It narrows a lookup by `name`, and can only be set with `name`.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.OneOf(cicdInfrastructureTypes...)},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the object; null for users without the view or change permission for CI/CD infrastructure.",
				Computed:            true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The URL of the tool; null for users without the view or change permission for CI/CD infrastructure.",
				Computed:            true,
			},
		},
	}
}

func (d *CICDInfrastructureDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		datasourcevalidator.Conflicting(path.MatchRoot("id"), path.MatchRoot("infrastructure_type")),
	}
}

func (d *CICDInfrastructureDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *CICDInfrastructureDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CICDInfrastructureModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		ci   *defectdojo.CICDInfrastructure
		err  error
		what string
	)
	if !data.ID.IsNull() {
		what = fmt.Sprintf("ID %d", data.ID.ValueInt64())
		ci, _, err = d.client.CICDInfrastructure.Get(ctx, int(data.ID.ValueInt64()), nil)
	} else {
		what = fmt.Sprintf("name %q", data.Name.ValueString())
		if !data.InfrastructureType.IsNull() {
			what += " and type " + data.InfrastructureType.ValueString()
		}
		ci, _, err = d.client.CICDInfrastructure.GetByName(ctx, data.Name.ValueString(), &defectdojo.GetCICDInfrastructureByNameOptions{
			InfrastructureType: defectdojo.CICDInfrastructureType(data.InfrastructureType.ValueString()),
		})
	}
	if err != nil {
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.Diagnostics.AddError("DefectDojo CI/CD infrastructure not found", "There is no CI/CD infrastructure object with "+what+".")
			return
		}
		resp.Diagnostics.AddError("Error reading DefectDojo CI/CD infrastructure", "Could not read the CI/CD infrastructure object with "+what+": "+err.Error())
		return
	}

	updateCICDInfrastructureModel(&data, ci)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
