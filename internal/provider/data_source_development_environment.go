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
var _ datasource.DataSource = &DevelopmentEnvironmentDataSource{}
var _ datasource.DataSourceWithConfigure = &DevelopmentEnvironmentDataSource{}
var _ datasource.DataSourceWithConfigValidators = &DevelopmentEnvironmentDataSource{}

func NewDevelopmentEnvironmentDataSource() datasource.DataSource {
	return &DevelopmentEnvironmentDataSource{}
}

// DevelopmentEnvironmentDataSource looks up a development environment.
type DevelopmentEnvironmentDataSource struct {
	client *defectdojo.Client
}

func (d *DevelopmentEnvironmentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_development_environment"
}

func (d *DevelopmentEnvironmentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a DefectDojo development environment by ID or name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the development environment to look up. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the development environment to look up, compared exactly. " +
					"Names aren't unique in DefectDojo: the lookup fails when several environments have the name. " +
					"Exactly one of `id` and `name` must be set.",
				Optional: true,
				Computed: true,
			},
		},
	}
}

func (d *DevelopmentEnvironmentDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *DevelopmentEnvironmentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *DevelopmentEnvironmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DevelopmentEnvironmentModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		env  *defectdojo.DevelopmentEnvironment
		err  error
		what string
	)
	if !data.ID.IsNull() {
		what = fmt.Sprintf("ID %d", data.ID.ValueInt64())
		env, _, err = d.client.DevelopmentEnvironments.Get(ctx, int(data.ID.ValueInt64()), nil)
	} else {
		what = fmt.Sprintf("name %q", data.Name.ValueString())
		env, _, err = d.client.DevelopmentEnvironments.GetByName(ctx, data.Name.ValueString(), nil)
	}
	if err != nil {
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.Diagnostics.AddError("DefectDojo development environment not found", "There is no development environment with "+what+".")
			return
		}
		resp.Diagnostics.AddError("Error reading DefectDojo development environment", "Could not read the development environment with "+what+": "+err.Error())
		return
	}

	updateDevelopmentEnvironmentModel(&data, env)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
