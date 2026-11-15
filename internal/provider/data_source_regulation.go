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
var _ datasource.DataSource = &RegulationDataSource{}
var _ datasource.DataSourceWithConfigure = &RegulationDataSource{}
var _ datasource.DataSourceWithConfigValidators = &RegulationDataSource{}

func NewRegulationDataSource() datasource.DataSource {
	return &RegulationDataSource{}
}

// RegulationDataSource looks up a regulation.
type RegulationDataSource struct {
	client *defectdojo.Client
}

func (d *RegulationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_regulation"
}

func (d *RegulationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	lookup := func(what string) string {
		return what + " Exactly one of `id`, `name` and `acronym` must be set."
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a DefectDojo regulation by ID, name or acronym, " +
			"for example one of the regulations DefectDojo comes with, for a product's `regulation_ids`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: lookup("The ID of the regulation to look up."),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: lookup("The name of the regulation to look up, compared exactly."),
				Optional:            true,
				Computed:            true,
			},
			"acronym": schema.StringAttribute{
				MarkdownDescription: lookup("The acronym of the regulation to look up, such as `PCI DSS`, compared exactly."),
				Optional:            true,
				Computed:            true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: "The subject of the regulation, such as `privacy`.",
				Computed:            true,
			},
			"jurisdiction": schema.StringAttribute{
				MarkdownDescription: "The territory the regulation applies to.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The purpose of the regulation.",
				Computed:            true,
			},
			"reference": schema.StringAttribute{
				MarkdownDescription: "A URL with more information.",
				Computed:            true,
			},
		},
	}
}

func (d *RegulationDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"), path.MatchRoot("acronym")),
	}
}

func (d *RegulationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *RegulationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RegulationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		reg  *defectdojo.Regulation
		err  error
		what string
	)
	switch {
	case !data.ID.IsNull():
		what = fmt.Sprintf("ID %d", data.ID.ValueInt64())
		reg, _, err = d.client.Regulations.Get(ctx, int(data.ID.ValueInt64()))
	case !data.Name.IsNull():
		what = fmt.Sprintf("name %q", data.Name.ValueString())
		reg, _, err = d.client.Regulations.GetByName(ctx, data.Name.ValueString())
	default:
		what = fmt.Sprintf("acronym %q", data.Acronym.ValueString())
		reg, _, err = d.client.Regulations.GetByAcronym(ctx, data.Acronym.ValueString())
	}
	if err != nil {
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.Diagnostics.AddError("DefectDojo regulation not found", "There is no regulation with "+what+".")
			return
		}
		resp.Diagnostics.AddError("Error reading DefectDojo regulation", "Could not read the regulation with "+what+": "+err.Error())
		return
	}

	updateRegulationModel(&data, reg)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
