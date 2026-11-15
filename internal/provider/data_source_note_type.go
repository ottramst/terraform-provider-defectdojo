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
var _ datasource.DataSource = &NoteTypeDataSource{}
var _ datasource.DataSourceWithConfigure = &NoteTypeDataSource{}
var _ datasource.DataSourceWithConfigValidators = &NoteTypeDataSource{}

func NewNoteTypeDataSource() datasource.DataSource {
	return &NoteTypeDataSource{}
}

// NoteTypeDataSource looks up a note type.
type NoteTypeDataSource struct {
	client *defectdojo.Client
}

// NoteTypeDataSourceModel describes the defectdojo_note_type data source.
type NoteTypeDataSourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	IsSingle    types.Bool   `tfsdk:"is_single"`
	IsActive    types.Bool   `tfsdk:"is_active"`
	IsMandatory types.Bool   `tfsdk:"is_mandatory"`
}

func (d *NoteTypeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_note_type"
}

func (d *NoteTypeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a DefectDojo note type by ID or name. It needs a superuser or the note type configuration permissions.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the note type to look up. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the note type to look up, compared exactly. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the note type.",
				Computed:            true,
			},
			"is_single": schema.BoolAttribute{
				MarkdownDescription: "Whether an object can have only one note of the type.",
				Computed:            true,
			},
			"is_active": schema.BoolAttribute{
				MarkdownDescription: "Whether the type is offered for new notes.",
				Computed:            true,
			},
			"is_mandatory": schema.BoolAttribute{
				MarkdownDescription: "Whether, while the type is active, the UI requires a note of the type before a finding is closed, or set inactive or false positive.",
				Computed:            true,
			},
		},
	}
}

func (d *NoteTypeDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *NoteTypeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *NoteTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NoteTypeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		nt   *defectdojo.NoteType
		err  error
		what string
	)
	if !data.ID.IsNull() {
		what = fmt.Sprintf("ID %d", data.ID.ValueInt64())
		nt, _, err = d.client.NoteTypes.Get(ctx, int(data.ID.ValueInt64()), nil)
	} else {
		what = fmt.Sprintf("name %q", data.Name.ValueString())
		nt, _, err = d.client.NoteTypes.GetByName(ctx, data.Name.ValueString(), nil)
	}
	if err != nil {
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.Diagnostics.AddError("DefectDojo note type not found", "There is no note type with "+what+".")
			return
		}
		resp.Diagnostics.AddError("Error reading DefectDojo note type", "Could not read the note type with "+what+": "+err.Error())
		return
	}

	data.ID = types.Int64Value(int64(nt.ID))
	data.Name = types.StringValue(nt.Name)
	data.Description = types.StringValue(nt.Description)
	data.IsSingle = types.BoolValue(nt.IsSingle)
	data.IsActive = types.BoolValue(nt.IsActive)
	data.IsMandatory = types.BoolValue(nt.IsMandatory)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
