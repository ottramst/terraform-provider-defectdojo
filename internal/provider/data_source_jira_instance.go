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
var _ datasource.DataSource = &JiraInstanceDataSource{}
var _ datasource.DataSourceWithConfigure = &JiraInstanceDataSource{}
var _ datasource.DataSourceWithConfigValidators = &JiraInstanceDataSource{}

func NewJiraInstanceDataSource() datasource.DataSource {
	return &JiraInstanceDataSource{}
}

// JiraInstanceDataSource looks up a Jira instance.
type JiraInstanceDataSource struct {
	client *defectdojo.Client
}

// JiraInstanceDataSourceModel is the resource's model without the password.
type JiraInstanceDataSourceModel struct {
	ID                             types.Int64  `tfsdk:"id"`
	ConfigurationName              types.String `tfsdk:"configuration_name"`
	URL                            types.String `tfsdk:"url"`
	Username                       types.String `tfsdk:"username"`
	DefaultIssueType               types.String `tfsdk:"default_issue_type"`
	IssueTemplateDir               types.String `tfsdk:"issue_template_dir"`
	EpicNameID                     types.Int64  `tfsdk:"epic_name_id"`
	OpenStatusKey                  types.Int64  `tfsdk:"open_status_key"`
	CloseStatusKey                 types.Int64  `tfsdk:"close_status_key"`
	InfoMappingSeverity            types.String `tfsdk:"info_mapping_severity"`
	LowMappingSeverity             types.String `tfsdk:"low_mapping_severity"`
	MediumMappingSeverity          types.String `tfsdk:"medium_mapping_severity"`
	HighMappingSeverity            types.String `tfsdk:"high_mapping_severity"`
	CriticalMappingSeverity        types.String `tfsdk:"critical_mapping_severity"`
	FindingText                    types.String `tfsdk:"finding_text"`
	AcceptedMappingResolution      types.String `tfsdk:"accepted_mapping_resolution"`
	FalsePositiveMappingResolution types.String `tfsdk:"false_positive_mapping_resolution"`
	GlobalJiraSLANotification      types.Bool   `tfsdk:"global_jira_sla_notification"`
	FindingJiraSync                types.Bool   `tfsdk:"finding_jira_sync"`
}

func (d *JiraInstanceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_instance"
}

func (d *JiraInstanceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{MarkdownDescription: description, Computed: true}
	}
	computedInt := func(description string) schema.Int64Attribute {
		return schema.Int64Attribute{MarkdownDescription: description, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a DefectDojo Jira instance by ID or name, " +
			"for example to connect a `defectdojo_jira_project` to an instance managed elsewhere. It needs a superuser, " +
			"or the configuration permissions for Jira instances.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the Jira instance to look up. Exactly one of `id` and `configuration_name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"configuration_name": schema.StringAttribute{
				MarkdownDescription: "The name of the Jira instance to look up, compared exactly. Exactly one of `id` and `configuration_name` must be set. " +
					"DefectDojo doesn't require unique names: the lookup fails when several instances have the name.",
				Optional: true,
				Computed: true,
			},
			"url":                               computed("The URL of the Jira server."),
			"username":                          computed("The user name or email address of the Jira account."),
			"default_issue_type":                computed("The type of new Jira issues."),
			"issue_template_dir":                computed("The folder of the templates that render issue descriptions, if set."),
			"epic_name_id":                      computedInt("The number of Jira's Epic Name custom field."),
			"open_status_key":                   computedInt("The ID of the Jira transition that reopens issues."),
			"close_status_key":                  computedInt("The ID of the Jira transition that closes issues."),
			"info_mapping_severity":             computed("The Jira priority of info findings' issues."),
			"low_mapping_severity":              computed("The Jira priority of low findings' issues."),
			"medium_mapping_severity":           computed("The Jira priority of medium findings' issues."),
			"high_mapping_severity":             computed("The Jira priority of high findings' issues."),
			"critical_mapping_severity":         computed("The Jira priority of critical findings' issues."),
			"finding_text":                      computed("Text added to the issues of findings, if set."),
			"accepted_mapping_resolution":       computed("The Jira resolutions that make a finding risk accepted, if set."),
			"false_positive_mapping_resolution": computed("The Jira resolutions that make a finding a false positive, if set."),
			"global_jira_sla_notification": schema.BoolAttribute{
				MarkdownDescription: "Whether DefectDojo posts SLA notifications as comments on issues.",
				Computed:            true,
			},
			"finding_jira_sync": schema.BoolAttribute{
				MarkdownDescription: "Whether DefectDojo pushes changes of findings to their issues.",
				Computed:            true,
			},
		},
	}
}

func (d *JiraInstanceDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("configuration_name")),
	}
}

func (d *JiraInstanceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *JiraInstanceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data JiraInstanceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		ji   *defectdojo.JiraInstance
		err  error
		what string
	)
	if !data.ID.IsNull() {
		what = fmt.Sprintf("ID %d", data.ID.ValueInt64())
		ji, _, err = d.client.JiraInstances.Get(ctx, int(data.ID.ValueInt64()), nil)
	} else {
		what = fmt.Sprintf("name %q", data.ConfigurationName.ValueString())
		ji, _, err = d.client.JiraInstances.GetByName(ctx, data.ConfigurationName.ValueString(), nil)
	}
	if err != nil {
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.Diagnostics.AddError("DefectDojo Jira instance not found", "There is no Jira instance with "+what+".")
			return
		}
		resp.Diagnostics.AddError("Error reading DefectDojo Jira instance", "Could not read the Jira instance with "+what+": "+err.Error())
		return
	}

	var m JiraInstanceResourceModel
	updateJiraInstanceModel(&m, ji)
	data = JiraInstanceDataSourceModel{
		ID: m.ID, ConfigurationName: m.ConfigurationName, URL: m.URL, Username: m.Username,
		DefaultIssueType: m.DefaultIssueType, IssueTemplateDir: m.IssueTemplateDir,
		EpicNameID: m.EpicNameID, OpenStatusKey: m.OpenStatusKey, CloseStatusKey: m.CloseStatusKey,
		InfoMappingSeverity: m.InfoMappingSeverity, LowMappingSeverity: m.LowMappingSeverity,
		MediumMappingSeverity: m.MediumMappingSeverity, HighMappingSeverity: m.HighMappingSeverity,
		CriticalMappingSeverity: m.CriticalMappingSeverity, FindingText: m.FindingText,
		AcceptedMappingResolution: m.AcceptedMappingResolution, FalsePositiveMappingResolution: m.FalsePositiveMappingResolution,
		GlobalJiraSLANotification: m.GlobalJiraSLANotification, FindingJiraSync: m.FindingJiraSync,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
