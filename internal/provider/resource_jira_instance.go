package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &JiraInstanceResource{}
var _ resource.ResourceWithConfigure = &JiraInstanceResource{}
var _ resource.ResourceWithImportState = &JiraInstanceResource{}
var _ resource.ResourceWithIdentity = &JiraInstanceResource{}
var _ resource.ResourceWithModifyPlan = &JiraInstanceResource{}

func NewJiraInstanceResource() resource.Resource {
	return &JiraInstanceResource{}
}

// JiraInstanceResource defines the resource implementation.
type JiraInstanceResource struct {
	client *defectdojo.Client
}

// JiraInstanceResourceModel describes the resource data model.
type JiraInstanceResourceModel struct {
	ID                             types.Int64  `tfsdk:"id"`
	ConfigurationName              types.String `tfsdk:"configuration_name"`
	URL                            types.String `tfsdk:"url"`
	Username                       types.String `tfsdk:"username"`
	PasswordWO                     types.String `tfsdk:"password_wo"`
	PasswordWOVersion              types.Int64  `tfsdk:"password_wo_version"`
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

func (r *JiraInstanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_instance"
}

// requiredText returns a required text attribute of at most maxLength
// characters.
func requiredText(description string, maxLength int) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: fmt.Sprintf("%s At most %d characters, without leading or trailing whitespace, which DefectDojo removes.", description, maxLength),
		Required:            true,
		Validators: []validator.String{
			stringvalidator.UTF8LengthBetween(1, maxLength),
			noSurroundingWhitespace(),
		},
	}
}

// optionalText returns an optional text attribute, which DefectDojo stores as
// null when it is unset; maxLength 0 means no limit.
func optionalText(description string, maxLength int) schema.StringAttribute {
	validators := []validator.String{stringvalidator.LengthAtLeast(1), noSurroundingWhitespace()}
	limit := ""
	if maxLength > 0 {
		validators[0] = stringvalidator.UTF8LengthBetween(1, maxLength)
		limit = fmt.Sprintf(" At most %d characters.", maxLength)
	}
	return schema.StringAttribute{
		MarkdownDescription: description + limit,
		Optional:            true,
		Validators:          validators,
	}
}

func (r *JiraInstanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	severity := func(s string) schema.StringAttribute {
		return requiredText(fmt.Sprintf("The Jira priority of %s findings' issues, such as `%s`.", strings.ToLower(s), s), 200)
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DefectDojo Jira instance: the connection to a Jira server that Jira projects " +
			"(`defectdojo_jira_project`) push findings to. DefectDojo doesn't contact Jira when the instance is created or changed, " +
			"and pushes findings only while the system setting `enable_jira` is on (`defectdojo_system_settings`). " +
			"Managing Jira instances needs a superuser, or the configuration permissions for Jira instances, " +
			"which DefectDojo lists only while `enable_jira` is on.\n\n" +
			"DefectDojo can't delete an instance that Jira projects use: the provider counts them first and refuses, " +
			"since DefectDojo would answer with a server error. The count includes only the Jira projects the provider's user may see.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the Jira instance.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"configuration_name": requiredText("The name of the Jira instance. Names don't have to be unique, but the data source looks instances up by name.", 2000),
			"url": schema.StringAttribute{
				MarkdownDescription: "The HTTP or HTTPS URL of the Jira server, at most 2000 characters.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 2000),
					stringvalidator.RegexMatches(webhookURLPattern, "must be an http:// or https:// URL"),
				},
			},
			"username": requiredText("The user name or email address of the Jira account.", 2000),
			"password_wo": schema.StringAttribute{
				MarkdownDescription: "The password or API token of the Jira account, at most 2000 characters, without leading or trailing whitespace, " +
					"which DefectDojo removes. It is write-only: never stored in the plan or state, and DefectDojo never returns it. " +
					"The provider sends it when the instance is created, when `password_wo_version` changes, and when `url` changes, " +
					"which DefectDojo allows only with the password (a URL that differs only in a trailing slash isn't a change): " +
					"it is required then, and otherwise optional, so that an imported instance can be managed without it. " +
					"Terraform can't detect a change of a write-only value: change `password_wo_version` to send a new one. " +
					"Removing it together with `password_wo_version` keeps the password DefectDojo has. " +
					"Requires Terraform 1.11 or later.",
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 2000),
					noSurroundingWhitespace(),
				},
			},
			"password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "The version of `password_wo`. Changing it, or removing it while `password_wo` is set, sends the password; " +
					"setting a new version needs `password_wo`.",
				Optional: true,
			},
			"default_issue_type": schema.StringAttribute{
				MarkdownDescription: "The type of new Jira issues. DefectDojo offers `Task`, `Story`, `Epic`, `Spike`, `Bug` and `Security`, " +
					"and `DD_JIRA_EXTRA_ISSUE_TYPES` adds more. Defaults to `Bug`.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("Bug"),
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 255),
					noSurroundingWhitespace(),
				},
			},
			"issue_template_dir": optionalText("The folder of the Django templates that render issue descriptions, such as `issue-trackers/jira_full`; "+
				"by default DefectDojo's.", 255),
			"epic_name_id": schema.Int64Attribute{
				MarkdownDescription: "The number of Jira's Epic Name custom field (`customfield_<number>`), from `https://<jira>/rest/api/2/field`.",
				Required:            true,
				Validators:          []validator.Int64{int32Range()},
			},
			"open_status_key": schema.Int64Attribute{
				MarkdownDescription: "The ID of the Jira transition that reopens issues, from `https://<jira>/rest/api/latest/issue/<key>/transitions`.",
				Required:            true,
				Validators:          []validator.Int64{int32Range()},
			},
			"close_status_key": schema.Int64Attribute{
				MarkdownDescription: "The ID of the Jira transition that closes issues.",
				Required:            true,
				Validators:          []validator.Int64{int32Range()},
			},
			"info_mapping_severity":     severity("Info"),
			"low_mapping_severity":      severity("Low"),
			"medium_mapping_severity":   severity("Medium"),
			"high_mapping_severity":     severity("High"),
			"critical_mapping_severity": severity("Critical"),
			"finding_text":              optionalText("Text added to the issues of findings, such as whom to contact.", 0),
			"accepted_mapping_resolution": optionalText("Jira resolutions, separated by commas, that make the finding of an issue closed in Jira "+
				"risk accepted.", 300),
			"false_positive_mapping_resolution": optionalText("Jira resolutions, separated by commas, that make the finding of an issue closed in Jira "+
				"a false positive.", 300),
			"global_jira_sla_notification": schema.BoolAttribute{
				MarkdownDescription: "Whether DefectDojo posts SLA notifications as comments on issues; each Jira project can override it. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"finding_jira_sync": schema.BoolAttribute{
				MarkdownDescription: "Whether DefectDojo pushes changes of findings to their issues. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *JiraInstanceResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("Jira instance")
}

func (r *JiraInstanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *JiraInstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data JiraInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	// Write-only values are only in the configuration.
	var password types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("password_wo"), &password)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if password.IsNull() {
		resp.Diagnostics.Append(missingJiraPassword("create a Jira instance"))
		return
	}

	created, _, err := r.client.JiraInstances.Create(ctx, &defectdojo.JiraInstanceWrite{
		JiraInstance: jiraInstanceFromModel(&data),
		Password:     password.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo Jira instance",
			"Could not create Jira instance "+strconv.Quote(data.ConfigurationName.ValueString())+", unexpected error: "+err.Error(),
		)
		return
	}
	updateJiraInstanceModel(&data, created)

	tflog.Trace(ctx, "created a Jira instance", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *JiraInstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data JiraInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ji, _, err := r.client.JiraInstances.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			removeGone(ctx, resp, data.ID)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo Jira instance",
			fmt.Sprintf("Could not read Jira instance %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateJiraInstanceModel(&data, ji)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *JiraInstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state JiraInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	var password types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("password_wo"), &password)...)
	if resp.Diagnostics.HasError() {
		return
	}

	patch := &defectdojo.JiraInstancePatch{
		ConfigurationName:              new(data.ConfigurationName.ValueString()),
		URL:                            new(data.URL.ValueString()),
		Username:                       new(data.Username.ValueString()),
		DefaultIssueType:               new(data.DefaultIssueType.ValueString()),
		IssueTemplateDir:               nullableString(data.IssueTemplateDir),
		EpicNameID:                     new(int(data.EpicNameID.ValueInt64())),
		OpenStatusKey:                  new(int(data.OpenStatusKey.ValueInt64())),
		CloseStatusKey:                 new(int(data.CloseStatusKey.ValueInt64())),
		InfoMappingSeverity:            new(data.InfoMappingSeverity.ValueString()),
		LowMappingSeverity:             new(data.LowMappingSeverity.ValueString()),
		MediumMappingSeverity:          new(data.MediumMappingSeverity.ValueString()),
		HighMappingSeverity:            new(data.HighMappingSeverity.ValueString()),
		CriticalMappingSeverity:        new(data.CriticalMappingSeverity.ValueString()),
		FindingText:                    nullableString(data.FindingText),
		AcceptedMappingResolution:      nullableString(data.AcceptedMappingResolution),
		FalsePositiveMappingResolution: nullableString(data.FalsePositiveMappingResolution),
		GlobalJiraSLANotification:      new(data.GlobalJiraSLANotification.ValueBool()),
		FindingJiraSync:                new(data.FindingJiraSync.ValueBool()),
	}
	// The password is sent when its version changes, since Terraform can't
	// compare write-only values, and when the URL changes, which DefectDojo
	// allows only with the password. Without one, nothing is sent: ModifyPlan
	// refused the cases that need it, except a new version that was unknown
	// at plan time, refused here.
	if password.IsNull() && !data.PasswordWOVersion.IsNull() && !data.PasswordWOVersion.Equal(state.PasswordWOVersion) {
		resp.Diagnostics.AddAttributeError(path.Root("password_wo"), "Missing Jira password",
			"A new password_wo_version sends password_wo to DefectDojo, but password_wo isn't set. "+
				"Set password_wo, or keep password_wo_version as it was.")
		return
	}
	if !password.IsNull() && password.ValueString() != "" &&
		(!data.PasswordWOVersion.Equal(state.PasswordWOVersion) || jiraURLChanged(state.URL.ValueString(), data.URL.ValueString())) {
		patch.Password = new(password.ValueString())
	}

	updated, _, err := r.client.JiraInstances.Patch(ctx, int(data.ID.ValueInt64()), patch)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo Jira instance",
			fmt.Sprintf("Could not update Jira instance %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateJiraInstanceModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

// jiraProjectsUsing describes how many Jira projects use an instance.
func jiraProjectsUsing(n int) string {
	if n == 1 {
		return "1 Jira project uses it"
	}
	return fmt.Sprintf("%d Jira projects use it", n)
}

// ModifyPlan requires password_wo when DefectDojo needs the password: to
// create the instance, to change its URL, and when a new password_wo_version
// asks to send it. A password unknown at plan time, such as an attribute of a
// resource created in the same apply, passes.
func (r *JiraInstanceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var password types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("password_wo"), &password)...)
	if resp.Diagnostics.HasError() || !password.IsNull() {
		return
	}
	if req.State.Raw.IsNull() {
		resp.Diagnostics.Append(missingJiraPassword("create a Jira instance"))
		return
	}
	var plan, state JiraInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	switch {
	case plan.URL.IsUnknown() || jiraURLChanged(state.URL.ValueString(), plan.URL.ValueString()):
		resp.Diagnostics.Append(missingJiraPassword("change the URL of a Jira instance"))
	case !plan.PasswordWOVersion.IsNull() && !plan.PasswordWOVersion.IsUnknown() && !plan.PasswordWOVersion.Equal(state.PasswordWOVersion):
		resp.Diagnostics.AddAttributeError(path.Root("password_wo"), "Missing Jira password",
			"A new password_wo_version sends password_wo to DefectDojo, but password_wo isn't set. "+
				"Set password_wo, or keep password_wo_version as it was.")
	}
}

// missingJiraPassword is the error when DefectDojo needs the Jira password to
// do something, such as "create a Jira instance", and password_wo isn't set.
func missingJiraPassword(action string) diag.Diagnostic {
	return diag.NewAttributeErrorDiagnostic(path.Root("password_wo"), "Missing Jira password",
		"DefectDojo needs the password or API token of the Jira account to "+action+": set password_wo.")
}

// jiraURLChanged reports whether DefectDojo considers the URL changed, which
// needs the password: a trailing slash doesn't count.
func jiraURLChanged(old, updated string) bool {
	return strings.TrimRight(old, "/") != strings.TrimRight(updated, "/")
}

func (r *JiraInstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data JiraInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())
	// DefectDojo answers a delete of an instance in use with a 500.
	n, _, err := r.client.JiraInstances.CountJiraProjects(ctx, id)
	if errors.Is(err, defectdojo.ErrNotFound) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo Jira instance",
			fmt.Sprintf("Could not count the Jira projects of Jira instance %d, unexpected error: %s", id, err),
		)
		return
	}
	if n > 0 {
		resp.Diagnostics.AddError(
			"Jira instance in use",
			fmt.Sprintf("DefectDojo can't delete Jira instance %d: %s. Delete them, or move them to another Jira instance, first.", id, jiraProjectsUsing(n)),
		)
		return
	}
	if _, err := r.client.JiraInstances.Delete(ctx, id); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo Jira instance",
			fmt.Sprintf("Could not delete Jira instance %d, unexpected error: %s", id, err),
		)
	}
}

func (r *JiraInstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "Jira instance")
}

// jiraInstanceFromModel returns the Jira instance the model describes.
func jiraInstanceFromModel(data *JiraInstanceResourceModel) defectdojo.JiraInstance {
	return defectdojo.JiraInstance{
		ConfigurationName:              data.ConfigurationName.ValueString(),
		URL:                            data.URL.ValueString(),
		Username:                       data.Username.ValueString(),
		DefaultIssueType:               data.DefaultIssueType.ValueString(),
		IssueTemplateDir:               data.IssueTemplateDir.ValueString(),
		EpicNameID:                     int(data.EpicNameID.ValueInt64()),
		OpenStatusKey:                  int(data.OpenStatusKey.ValueInt64()),
		CloseStatusKey:                 int(data.CloseStatusKey.ValueInt64()),
		InfoMappingSeverity:            data.InfoMappingSeverity.ValueString(),
		LowMappingSeverity:             data.LowMappingSeverity.ValueString(),
		MediumMappingSeverity:          data.MediumMappingSeverity.ValueString(),
		HighMappingSeverity:            data.HighMappingSeverity.ValueString(),
		CriticalMappingSeverity:        data.CriticalMappingSeverity.ValueString(),
		FindingText:                    data.FindingText.ValueString(),
		AcceptedMappingResolution:      data.AcceptedMappingResolution.ValueString(),
		FalsePositiveMappingResolution: data.FalsePositiveMappingResolution.ValueString(),
		GlobalJiraSLANotification:      data.GlobalJiraSLANotification.ValueBool(),
		FindingJiraSync:                data.FindingJiraSync.ValueBool(),
	}
}

// updateJiraInstanceModel copies the Jira instance read from the server into
// data. The password is never stored; DefectDojo doesn't return it.
func updateJiraInstanceModel(data *JiraInstanceResourceModel, ji *defectdojo.JiraInstance) {
	data.ID = types.Int64Value(int64(ji.ID))
	data.ConfigurationName = types.StringValue(ji.ConfigurationName)
	data.URL = types.StringValue(ji.URL)
	data.Username = types.StringValue(ji.Username)
	data.PasswordWO = types.StringNull()
	data.DefaultIssueType = types.StringValue(ji.DefaultIssueType)
	data.IssueTemplateDir = stringOrNull(ji.IssueTemplateDir)
	data.EpicNameID = types.Int64Value(int64(ji.EpicNameID))
	data.OpenStatusKey = types.Int64Value(int64(ji.OpenStatusKey))
	data.CloseStatusKey = types.Int64Value(int64(ji.CloseStatusKey))
	data.InfoMappingSeverity = types.StringValue(ji.InfoMappingSeverity)
	data.LowMappingSeverity = types.StringValue(ji.LowMappingSeverity)
	data.MediumMappingSeverity = types.StringValue(ji.MediumMappingSeverity)
	data.HighMappingSeverity = types.StringValue(ji.HighMappingSeverity)
	data.CriticalMappingSeverity = types.StringValue(ji.CriticalMappingSeverity)
	data.FindingText = stringOrNull(ji.FindingText)
	data.AcceptedMappingResolution = stringOrNull(ji.AcceptedMappingResolution)
	data.FalsePositiveMappingResolution = stringOrNull(ji.FalsePositiveMappingResolution)
	data.GlobalJiraSLANotification = types.BoolValue(ji.GlobalJiraSLANotification)
	data.FindingJiraSync = types.BoolValue(ji.FindingJiraSync)
}
