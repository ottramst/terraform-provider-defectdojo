package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
var _ resource.Resource = &JiraProjectResource{}
var _ resource.ResourceWithConfigure = &JiraProjectResource{}
var _ resource.ResourceWithImportState = &JiraProjectResource{}
var _ resource.ResourceWithIdentity = &JiraProjectResource{}
var _ resource.ResourceWithConfigValidators = &JiraProjectResource{}
var _ resource.ResourceWithModifyPlan = &JiraProjectResource{}
var _ resource.ResourceWithMoveState = &JiraProjectResource{}

// suppressSLANotificationCommentsSince is the first DefectDojo version with
// suppress_sla_notification_comments; older versions drop it.
const suppressSLANotificationCommentsSince = "3.5.103"

func NewJiraProjectResource() resource.Resource {
	return &JiraProjectResource{}
}

// JiraProjectResource defines the resource implementation.
type JiraProjectResource struct {
	client *defectdojo.Client
}

// JiraProjectResourceModel describes the resource data model.
type JiraProjectResourceModel struct {
	ID                                   types.Int64          `tfsdk:"id"`
	JiraInstanceID                       types.Int64          `tfsdk:"jira_instance_id"`
	ProductID                            types.Int64          `tfsdk:"product_id"`
	EngagementID                         types.Int64          `tfsdk:"engagement_id"`
	ProjectKey                           types.String         `tfsdk:"project_key"`
	Component                            types.String         `tfsdk:"component"`
	IssueTemplateDir                     types.String         `tfsdk:"issue_template_dir"`
	DefaultAssignee                      types.String         `tfsdk:"default_assignee"`
	JiraLabels                           types.String         `tfsdk:"jira_labels"`
	CustomFields                         jsontypes.Normalized `tfsdk:"custom_fields"`
	CloseTransitionFields                jsontypes.Normalized `tfsdk:"close_transition_fields"`
	ReopenTransitionFields               jsontypes.Normalized `tfsdk:"reopen_transition_fields"`
	EpicIssueTypeName                    types.String         `tfsdk:"epic_issue_type_name"`
	Enabled                              types.Bool           `tfsdk:"enabled"`
	PushAllIssues                        types.Bool           `tfsdk:"push_all_issues"`
	EnableEngagementEpicMapping          types.Bool           `tfsdk:"enable_engagement_epic_mapping"`
	PushNotes                            types.Bool           `tfsdk:"push_notes"`
	AddVulnerabilityIDToJiraLabel        types.Bool           `tfsdk:"add_vulnerability_id_to_jira_label"`
	ProductJiraSLANotification           types.Bool           `tfsdk:"product_jira_sla_notification"`
	SuppressSLANotificationComments      types.Bool           `tfsdk:"suppress_sla_notification_comments"`
	RiskAcceptanceExpirationNotification types.Bool           `tfsdk:"risk_acceptance_expiration_notification"`
}

func (r *JiraProjectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project"
}

// jsonObjectAttribute returns an attribute of a JSON object.
func jsonObjectAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description + " A JSON object, for example from `jsonencode()`; formatting and key order don't matter, " +
			"although changing only the formatting plans an update that changes nothing, as does the first plan after an import.",
		CustomType: jsontypes.NormalizedType{},
		Optional:   true,
		Validators: []validator.String{jsonObjectValidator{}},
	}
}

func optionalBool(description string, def bool) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: fmt.Sprintf("%s Defaults to `%t`.", description, def),
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(def),
	}
}

func (r *JiraProjectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jira project of a DefectDojo product or engagement: the project of a Jira instance " +
			"(`defectdojo_jira_instance`) that its findings are pushed to. An engagement's Jira project overrides its product's. " +
			"It needs edit permission on the product or engagement, not a superuser. " +
			"DefectDojo pushes findings only while the system setting `enable_jira` is on (`defectdojo_system_settings`), " +
			"but the Jira project can be managed without it.\n\n" +
			"DefectDojo lets a product have several Jira projects, but uses only one: the provider refuses to create one " +
			"for a product or engagement that already has one it can see (import that one instead). The check isn't atomic. " +
			"Destroying the resource also deletes the links between findings and Jira issues made through it; " +
			"the issues in Jira are kept. A replacement is destroyed before it is created again: with `create_before_destroy`, " +
			"the new Jira project would be refused while the old one exists.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the Jira project configuration.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"jira_instance_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the Jira instance.",
				Required:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"product_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the product. Exactly one of `product_id` and `engagement_id` must be set; changing either forces a new resource.",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"engagement_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the engagement. Exactly one of `product_id` and `engagement_id` must be set; changing either forces a new resource.",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"project_key": optionalText("The key of the Jira project, such as `SEC`. Without one, an engagement's findings go to its product's Jira project.", 200),
			"component":   optionalText("The Jira component of new issues, or several separated by commas.", 200),
			"issue_template_dir": optionalText("The folder of the Django templates that render issue descriptions, such as `issue-trackers/jira_full`; "+
				"by default the Jira instance's.", 255),
			"default_assignee": optionalText("The Jira user that new issues are assigned to; by default Jira's.", 200),
			"jira_labels":      optionalText("The labels of new issues, separated by spaces.", 200),
			"custom_fields": jsonObjectAttribute("The values of Jira fields of new issues, by field ID, such as " +
				"`{\"customfield_10122\" = [{ name = \"8.0.1\" }]}`."),
			"close_transition_fields": jsonObjectAttribute("The fields sent with the transition that closes an issue, such as " +
				"`{ resolution = { name = \"Won't Fix\" } }`."),
			"reopen_transition_fields": jsonObjectAttribute("The fields sent with the transition that reopens an issue."),
			"epic_issue_type_name": schema.StringAttribute{
				MarkdownDescription: "The name of Jira's epic issue type, for `enable_engagement_epic_mapping`. Defaults to `Epic`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("Epic"),
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 64),
					noSurroundingWhitespace(),
				},
			},
			"enabled": optionalBool("Whether findings are pushed to Jira; disabling stops it, also for findings pushed before.", true),
			"push_all_issues": optionalBool("Whether DefectDojo pushes every finding to Jira automatically "+
				"(only verified ones while the system setting `enforce_verified_status` is on), rather than when asked.", false),
			"enable_engagement_epic_mapping":     optionalBool("Whether DefectDojo creates a Jira epic for each engagement.", false),
			"push_notes":                         optionalBool("Whether DefectDojo pushes the notes of findings as comments.", false),
			"add_vulnerability_id_to_jira_label": optionalBool("Whether a finding's vulnerability IDs are added to the labels of its issue.", false),
			"product_jira_sla_notification": optionalBool("Whether DefectDojo posts SLA notifications as comments on issues, "+
				"even when the Jira instance's `global_jira_sla_notification` is off. Can't be set with `suppress_sla_notification_comments`.", false),
			"suppress_sla_notification_comments": optionalBool("Whether DefectDojo never posts SLA notifications as comments on issues, "+
				"even when the Jira instance's `global_jira_sla_notification` is on. Needs DefectDojo 3.5.103 or later: the provider refuses it on older versions, which would ignore it, "+
				"and reads the version from DefectDojo's OpenAPI schema, which needs no permission.", false),
			"risk_acceptance_expiration_notification": optionalBool("Whether DefectDojo posts notifications of expiring risk acceptances as comments on issues.", false),
		},
	}
}

func (r *JiraProjectResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(path.MatchRoot("product_id"), path.MatchRoot("engagement_id")),
		slaNotificationValidator{},
	}
}

// slaNotificationValidator rejects both SLA notification settings, which
// DefectDojo 3.4 doesn't check.
type slaNotificationValidator struct{}

func (v slaNotificationValidator) Description(_ context.Context) string {
	return "product_jira_sla_notification and suppress_sla_notification_comments can't both be true"
}

func (v slaNotificationValidator) MarkdownDescription(_ context.Context) string {
	return "`product_jira_sla_notification` and `suppress_sla_notification_comments` can't both be `true`"
}

func (v slaNotificationValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var send, suppress types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("product_jira_sla_notification"), &send)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("suppress_sla_notification_comments"), &suppress)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if send.ValueBool() && suppress.ValueBool() {
		resp.Diagnostics.AddAttributeError(path.Root("suppress_sla_notification_comments"), "Conflicting SLA notification settings",
			"product_jira_sla_notification sends SLA notifications as comments, and suppress_sla_notification_comments never does: set at most one.")
	}
}

// jsonObjectValidator checks that a value is a JSON object: DefectDojo would
// also store other JSON, or parse a JSON string.
type jsonObjectValidator struct{}

func (v jsonObjectValidator) Description(_ context.Context) string {
	return "value must be a JSON object"
}

func (v jsonObjectValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v jsonObjectValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if !known(req.ConfigValue) {
		return
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(req.ConfigValue.ValueString()), &object); err != nil || object == nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON object", "The value must be a JSON object, such as the result of jsonencode({ ... }).")
	}
}

func (r *JiraProjectResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("Jira project")
}

func (r *JiraProjectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

// ModifyPlan refuses suppress_sla_notification_comments on DefectDojo
// versions that would drop it. It reads the server's version only when the
// setting is planned to be true.
func (r *JiraProjectResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || r.client == nil {
		return
	}
	var suppress types.Bool
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("suppress_sla_notification_comments"), &suppress)...)
	if resp.Diagnostics.HasError() || !known(suppress) || !suppress.ValueBool() {
		return
	}
	version, err := r.client.ServerVersion(ctx)
	if err != nil {
		resp.Diagnostics.AddWarning("Could not read the DefectDojo version",
			"suppress_sla_notification_comments needs DefectDojo "+suppressSLANotificationCommentsSince+" or later, which couldn't be checked: "+err.Error())
		return
	}
	if compareVersions(version, suppressSLANotificationCommentsSince) < 0 {
		resp.Diagnostics.AddAttributeError(path.Root("suppress_sla_notification_comments"), "Unsupported DefectDojo version",
			fmt.Sprintf("suppress_sla_notification_comments needs DefectDojo %s or later; this server runs %s, which would ignore it.",
				suppressSLANotificationCommentsSince, version))
	}
}

func (r *JiraProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data JiraProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// DefectDojo lets a product have several Jira projects, of which it uses
	// one, so an existing one is imported rather than duplicated.
	opt := &defectdojo.ListJiraProjectsOptions{Product: int(data.ProductID.ValueInt64()), Engagement: int(data.EngagementID.ValueInt64())}
	what := fmt.Sprintf("product %d", opt.Product)
	if opt.Engagement != 0 {
		what = fmt.Sprintf("engagement %d", opt.Engagement)
	}
	existing, _, err := r.client.JiraProjects.List(ctx, opt)
	if err != nil {
		detail := fmt.Sprintf("Could not check the Jira projects of %s: %s", what, err)
		// DefectDojo's filter answers an ID that doesn't exist with a 400 on it.
		var errResp *defectdojo.ErrorResponse
		if errors.As(err, &errResp) && (len(errResp.Fields["product"]) > 0 || len(errResp.Fields["engagement"]) > 0) {
			detail = fmt.Sprintf("Could not create the Jira project: %s doesn't exist.", what)
		}
		resp.Diagnostics.AddError("Error creating DefectDojo Jira project", detail)
		return
	}
	if existing.Count > 0 {
		resp.Diagnostics.AddError("Jira project exists",
			fmt.Sprintf("%s already has a Jira project (ID %d); import it instead of creating another, which DefectDojo would ignore.",
				what, existing.JiraProjects[0].ID))
		return
	}

	created, _, err := r.client.JiraProjects.Create(ctx, jiraProjectFromModel(&data))
	if err != nil {
		resp.Diagnostics.AddError("Error creating DefectDojo Jira project",
			fmt.Sprintf("Could not create the Jira project of %s, unexpected error: %s", what, err))
		return
	}
	updateJiraProjectModel(&data, created)

	tflog.Trace(ctx, "created a Jira project", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *JiraProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data JiraProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, _, err := r.client.JiraProjects.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform, also with its product or engagement.
		if errors.Is(err, defectdojo.ErrNotFound) {
			removeGone(ctx, resp, data.ID)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo Jira project",
			fmt.Sprintf("Could not read Jira project %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateJiraProjectModel(&data, p)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *JiraProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data JiraProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	patch := &defectdojo.JiraProjectPatch{
		JiraInstance:                         new(int(data.JiraInstanceID.ValueInt64())),
		ProjectKey:                           new(data.ProjectKey.ValueString()),
		Component:                            new(data.Component.ValueString()),
		IssueTemplateDir:                     nullableString(data.IssueTemplateDir),
		DefaultAssignee:                      nullableString(data.DefaultAssignee),
		JiraLabels:                           nullableString(data.JiraLabels),
		CustomFields:                         nullableJSON(data.CustomFields),
		CloseTransitionFields:                nullableJSON(data.CloseTransitionFields),
		ReopenTransitionFields:               nullableJSON(data.ReopenTransitionFields),
		EpicIssueTypeName:                    new(data.EpicIssueTypeName.ValueString()),
		Enabled:                              new(data.Enabled.ValueBool()),
		PushAllIssues:                        new(data.PushAllIssues.ValueBool()),
		EnableEngagementEpicMapping:          new(data.EnableEngagementEpicMapping.ValueBool()),
		PushNotes:                            new(data.PushNotes.ValueBool()),
		AddVulnerabilityIDToJiraLabel:        new(data.AddVulnerabilityIDToJiraLabel.ValueBool()),
		ProductJiraSLANotification:           new(data.ProductJiraSLANotification.ValueBool()),
		SuppressSLANotificationComments:      new(data.SuppressSLANotificationComments.ValueBool()),
		RiskAcceptanceExpirationNotification: new(data.RiskAcceptanceExpirationNotification.ValueBool()),
	}
	updated, _, err := r.client.JiraProjects.Patch(ctx, int(data.ID.ValueInt64()), patch)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo Jira project",
			fmt.Sprintf("Could not update Jira project %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateJiraProjectModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *JiraProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data JiraProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())
	if _, err := r.client.JiraProjects.Delete(ctx, id); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo Jira project",
			fmt.Sprintf("Could not delete Jira project %d, unexpected error: %s", id, err),
		)
	}
}

// MoveState moves a defectdojo_jira_product_configuration of the
// doximity/defectdojo provider here, with a moved block.
func (r *JiraProjectResource) MoveState(_ context.Context) []resource.StateMover {
	return []resource.StateMover{moveFromDoximity("defectdojo_jira_product_configuration")}
}

func (r *JiraProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "Jira project")
}

// nullableJSON returns a JSON value, or null when it isn't set.
func nullableJSON(v jsontypes.Normalized) defectdojo.Nullable[json.RawMessage] {
	if !known(v) {
		return defectdojo.Null[json.RawMessage]()
	}
	return defectdojo.Value(json.RawMessage(v.ValueString()))
}

// jsonOrNull returns a JSON value read from the server, or null when it is
// unset.
func jsonOrNull(v json.RawMessage) jsontypes.Normalized {
	if v == nil {
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(string(v))
}

// jiraProjectFromModel returns the Jira project the model describes.
func jiraProjectFromModel(data *JiraProjectResourceModel) *defectdojo.JiraProject {
	p := &defectdojo.JiraProject{
		JiraInstance:                         int(data.JiraInstanceID.ValueInt64()),
		Product:                              int(data.ProductID.ValueInt64()),
		Engagement:                           int(data.EngagementID.ValueInt64()),
		ProjectKey:                           data.ProjectKey.ValueString(),
		Component:                            data.Component.ValueString(),
		IssueTemplateDir:                     data.IssueTemplateDir.ValueString(),
		DefaultAssignee:                      data.DefaultAssignee.ValueString(),
		JiraLabels:                           data.JiraLabels.ValueString(),
		EpicIssueTypeName:                    data.EpicIssueTypeName.ValueString(),
		Enabled:                              data.Enabled.ValueBool(),
		PushAllIssues:                        data.PushAllIssues.ValueBool(),
		EnableEngagementEpicMapping:          data.EnableEngagementEpicMapping.ValueBool(),
		PushNotes:                            data.PushNotes.ValueBool(),
		AddVulnerabilityIDToJiraLabel:        data.AddVulnerabilityIDToJiraLabel.ValueBool(),
		ProductJiraSLANotification:           data.ProductJiraSLANotification.ValueBool(),
		SuppressSLANotificationComments:      new(data.SuppressSLANotificationComments.ValueBool()),
		RiskAcceptanceExpirationNotification: data.RiskAcceptanceExpirationNotification.ValueBool(),
	}
	if known(data.CustomFields) {
		p.CustomFields = json.RawMessage(data.CustomFields.ValueString())
	}
	if known(data.CloseTransitionFields) {
		p.CloseTransitionFields = json.RawMessage(data.CloseTransitionFields.ValueString())
	}
	if known(data.ReopenTransitionFields) {
		p.ReopenTransitionFields = json.RawMessage(data.ReopenTransitionFields.ValueString())
	}
	return p
}

// updateJiraProjectModel copies the Jira project read from the server into
// data.
func updateJiraProjectModel(data *JiraProjectResourceModel, p *defectdojo.JiraProject) {
	data.ID = types.Int64Value(int64(p.ID))
	data.JiraInstanceID = int64OrNull(p.JiraInstance)
	data.ProductID = int64OrNull(p.Product)
	data.EngagementID = int64OrNull(p.Engagement)
	data.ProjectKey = stringOrNull(p.ProjectKey)
	data.Component = stringOrNull(p.Component)
	data.IssueTemplateDir = stringOrNull(p.IssueTemplateDir)
	data.DefaultAssignee = stringOrNull(p.DefaultAssignee)
	data.JiraLabels = stringOrNull(p.JiraLabels)
	data.CustomFields = jsonOrNull(p.CustomFields)
	data.CloseTransitionFields = jsonOrNull(p.CloseTransitionFields)
	data.ReopenTransitionFields = jsonOrNull(p.ReopenTransitionFields)
	data.EpicIssueTypeName = types.StringValue(p.EpicIssueTypeName)
	data.Enabled = types.BoolValue(p.Enabled)
	data.PushAllIssues = types.BoolValue(p.PushAllIssues)
	data.EnableEngagementEpicMapping = types.BoolValue(p.EnableEngagementEpicMapping)
	data.PushNotes = types.BoolValue(p.PushNotes)
	data.AddVulnerabilityIDToJiraLabel = types.BoolValue(p.AddVulnerabilityIDToJiraLabel)
	data.ProductJiraSLANotification = types.BoolValue(p.ProductJiraSLANotification)
	// DefectDojo before 3.5.103 doesn't have it, which means false.
	data.SuppressSLANotificationComments = types.BoolValue(p.SuppressSLANotificationComments != nil && *p.SuppressSLANotificationComments)
	data.RiskAcceptanceExpirationNotification = types.BoolValue(p.RiskAcceptanceExpirationNotification)
}
