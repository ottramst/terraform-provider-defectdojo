package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &SystemSettingsDataSource{}
var _ datasource.DataSourceWithConfigure = &SystemSettingsDataSource{}

func NewSystemSettingsDataSource() datasource.DataSource {
	return &SystemSettingsDataSource{}
}

// SystemSettingsDataSource defines the data source implementation. Like the
// resource, it sets its state attribute by attribute from
// systemSettingsFields.
type SystemSettingsDataSource struct {
	client *defectdojo.Client
}

func (d *SystemSettingsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_settings"
}

func (d *SystemSettingsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			MarkdownDescription: "The ID of the system settings.",
			Computed:            true,
		},
		"install_id": schema.StringAttribute{
			MarkdownDescription: "The UUID that identifies the DefectDojo installation.",
			Computed:            true,
		},
		"read_only_mode": schema.BoolAttribute{
			MarkdownDescription: "Whether DefectDojo is in read-only mode, in which it rejects every change but those of the system settings. " +
				"Null before DefectDojo 3.5.100, which doesn't have it.",
			Computed: true,
		},
	}
	for _, s := range systemSettingsFields {
		attributes[s.name] = s.dataSourceAttribute()
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads DefectDojo's system settings, the configuration of the whole instance. It needs a superuser. " +
			"The Slack token, the Microsoft Teams webhook URL and the Jira webhook secret are left out.",
		Attributes: attributes,
	}
}

func (d *SystemSettingsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *SystemSettingsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	settings, _, err := d.client.SystemSettings.Get(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error reading DefectDojo system settings", err.Error())
		return
	}
	values, err := systemSettingsValues(settings)
	if err != nil {
		resp.Diagnostics.AddError("Error reading DefectDojo system settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(int64(settings.ID)))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("install_id"), types.StringValue(settings.InstallID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("read_only_mode"), types.BoolPointerValue(settings.ReadOnlyMode))...)
	for _, s := range systemSettingsFields {
		v, err := s.value(values)
		if err != nil {
			resp.Diagnostics.AddError("Error reading DefectDojo system settings", err.Error())
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(s.name), v)...)
	}
}
