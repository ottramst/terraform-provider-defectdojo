package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &SystemSettingsResource{}
var _ resource.ResourceWithConfigure = &SystemSettingsResource{}
var _ resource.ResourceWithImportState = &SystemSettingsResource{}
var _ resource.ResourceWithIdentity = &SystemSettingsResource{}
var _ resource.ResourceWithModifyPlan = &SystemSettingsResource{}

func NewSystemSettingsResource() resource.Resource {
	return &SystemSettingsResource{}
}

// SystemSettingsResource defines the resource implementation. Its state is
// read and written attribute by attribute, from systemSettingsFields, rather
// than through a model struct.
type SystemSettingsResource struct {
	client *defectdojo.Client
}

func (r *SystemSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_settings"
}

func (r *SystemSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			MarkdownDescription: "The ID of the system settings, `1` on every DefectDojo installation.",
			Computed:            true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
	}
	for _, s := range systemSettingsFields {
		attributes[s.name] = s.resourceAttribute()
	}
	for _, s := range systemSettingsSecrets {
		lengths := fmt.Sprintf("at most %d characters", s.maxLength)
		validators := []validator.String{stringvalidator.UTF8LengthAtMost(s.maxLength), noSurroundingWhitespace()}
		if s.minLength > 0 {
			lengths = fmt.Sprintf("%d to %d characters", s.minLength, s.maxLength)
			validators[0] = stringvalidator.UTF8LengthBetween(s.minLength, s.maxLength)
		}
		attributes[s.name+"_wo"] = schema.StringAttribute{
			MarkdownDescription: s.description + " " + lengths + ", without leading or trailing whitespace, which DefectDojo removes. " +
				"It is write-only: never stored in the plan or state, and Terraform can't detect a change of it, or one made in DefectDojo; " +
				"it is sent when `" + s.name + "_wo_version` changes. Removing both leaves DefectDojo's value: it can't be cleared from Terraform. " +
				"Requires Terraform 1.11 or later.",
			Optional:   true,
			Sensitive:  true,
			WriteOnly:  true,
			Validators: append(validators, stringvalidator.AlsoRequires(path.MatchRoot(s.name+"_wo_version"))),
		}
		attributes[s.name+"_wo_version"] = schema.Int64Attribute{
			MarkdownDescription: "The version of `" + s.name + "_wo`. The provider sends the value when the version is set or changes.",
			Optional:            true,
			Validators: []validator.Int64{
				int64validator.AlsoRequires(path.MatchRoot(s.name + "_wo")),
			},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages DefectDojo's system settings: the configuration of the whole instance. It needs a superuser.\n\n" +
			"The provider changes only the settings in the configuration. The others keep DefectDojo's values, which the resource reads, " +
			"so importing the settings and adding the resource change nothing; removing a setting from the configuration leaves its value as it is. " +
			"A setting someone changes in DefectDojo is changed back by the next apply when it is in the configuration, and otherwise taken over. " +
			"Manage the settings with one resource only: two would undo each other's changes.\n\n" +
			"The system settings always exist: creating the resource changes the configured settings, and destroying it only removes it from the Terraform state, " +
			"leaving the settings as they are.\n\n" +
			"DefectDojo returns the Slack token, the Microsoft Teams webhook URL and the Jira webhook secret in plain text: " +
			"they are write-only attributes here, which are never stored, with a version that makes the provider send them. " +
			"DefectDojo's read-only mode is left out on purpose: while it is on, DefectDojo rejects every change but those of the system settings, " +
			"so turn it on and off in DefectDojo.",
		Attributes: attributes,
	}
}

func (r *SystemSettingsResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("system settings")
}

func (r *SystemSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

// ModifyPlan checks that the minimum password length doesn't exceed the
// maximum, which DefectDojo doesn't check. It checks the planned values, so
// that a setting the configuration leaves out is compared with DefectDojo's.
func (r *SystemSettingsResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var minimum, maximum types.Int64
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("minimum_password_length"), &minimum)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("maximum_password_length"), &maximum)...)
	if resp.Diagnostics.HasError() || !known(minimum) || !known(maximum) {
		return
	}
	if minimum.ValueInt64() > maximum.ValueInt64() {
		resp.Diagnostics.AddAttributeError(path.Root("minimum_password_length"), "Invalid password lengths",
			fmt.Sprintf("minimum_password_length (%d) exceeds maximum_password_length (%d), which DefectDojo would accept and no password could meet.",
				minimum.ValueInt64(), maximum.ValueInt64()))
	}
}

func (r *SystemSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	settings, _, err := r.client.SystemSettings.Get(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading DefectDojo system settings", err.Error())
		return
	}
	r.apply(ctx, settings.ID, req.Config, req.Plan, nil, &resp.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, types.Int64Value(int64(settings.ID)))...)
	tflog.Trace(ctx, "created DefectDojo system settings", map[string]any{"id": settings.ID})
}

func (r *SystemSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var id types.Int64
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, int(id.ValueInt64()), req.Config, req.Plan, &req.State, &resp.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, id)...)
}

// apply PATCHes every setting the configuration sets, and the secrets whose
// version changed (every secret with a version on create, when prior is
// nil), then sets the state from the plan and the response.
func (r *SystemSettingsResource) apply(ctx context.Context, id int, config tfsdk.Config, plan tfsdk.Plan, prior *tfsdk.State, state *tfsdk.State, diags *diag.Diagnostics) {
	values := make(map[string]any)
	for _, s := range systemSettingsFields {
		v, ok := configuredSetting(ctx, config, s, diags)
		if ok {
			values[s.jsonName()] = v
		}
	}
	for _, s := range systemSettingsSecrets {
		var planned, previous types.Int64
		diags.Append(plan.GetAttribute(ctx, path.Root(s.name+"_wo_version"), &planned)...)
		if prior != nil {
			diags.Append(prior.GetAttribute(ctx, path.Root(s.name+"_wo_version"), &previous)...)
		}
		if planned.IsNull() || planned.Equal(previous) {
			continue
		}
		var secret types.String
		diags.Append(config.GetAttribute(ctx, path.Root(s.name+"_wo"), &secret)...)
		if known(secret) {
			values[s.name] = secret.ValueString()
		}
	}
	if diags.HasError() {
		return
	}
	patch, err := systemSettingsPatch(values)
	if err != nil {
		diags.AddError("Error building the DefectDojo system settings", err.Error())
		return
	}
	settings, _, err := r.client.SystemSettings.Patch(ctx, id, patch)
	if err != nil {
		diags.AddError("Error updating DefectDojo system settings",
			fmt.Sprintf("Could not update the system settings (ID %d): %s", id, err))
		return
	}
	state.Raw = plan.Raw
	setSystemSettingsState(ctx, settings, state, diags)
}

// configuredSetting returns the configured value of the setting, as JSON
// takes it, and whether the configuration sets it.
func configuredSetting(ctx context.Context, config tfsdk.Config, s systemSetting, diags *diag.Diagnostics) (any, bool) {
	switch s.kind {
	case settingBool:
		var v types.Bool
		diags.Append(config.GetAttribute(ctx, path.Root(s.name), &v)...)
		return v.ValueBool(), known(v)
	case settingInt, settingNullableInt:
		var v types.Int64
		diags.Append(config.GetAttribute(ctx, path.Root(s.name), &v)...)
		return v.ValueInt64(), known(v)
	default:
		var v types.String
		diags.Append(config.GetAttribute(ctx, path.Root(s.name), &v)...)
		return v.ValueString(), known(v)
	}
}

// setSystemSettingsState sets the ID and every setting of the state from the
// system settings.
func setSystemSettingsState(ctx context.Context, settings *defectdojo.SystemSettings, state *tfsdk.State, diags *diag.Diagnostics) {
	values, err := systemSettingsValues(settings)
	if err != nil {
		diags.AddError("Error reading DefectDojo system settings", err.Error())
		return
	}
	diags.Append(state.SetAttribute(ctx, path.Root("id"), types.Int64Value(int64(settings.ID)))...)
	for _, s := range systemSettingsFields {
		v, err := s.value(values)
		if err != nil {
			diags.AddError("Error reading DefectDojo system settings", err.Error())
			return
		}
		diags.Append(state.SetAttribute(ctx, path.Root(s.name), v)...)
	}
}

func (r *SystemSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var id types.Int64
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	settings, _, err := r.client.SystemSettings.Get(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading DefectDojo system settings", err.Error())
		return
	}
	// The system settings have one ID: a state with another one, such as an
	// import of the wrong ID, doesn't describe them.
	if known(id) && id.ValueInt64() != int64(settings.ID) {
		resp.Diagnostics.AddWarning("DefectDojo system settings not found",
			fmt.Sprintf("DefectDojo's system settings have the ID %d, not %d; removing them from the state. Import them with the ID %d.", settings.ID, id.ValueInt64(), settings.ID))
		resp.State.RemoveResource(ctx)
		return
	}
	resp.State.Raw = req.State.Raw
	setSystemSettingsState(ctx, settings, &resp.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, types.Int64Value(int64(settings.ID)))...)
}

// Delete only removes the resource from the state: the system settings
// always exist, and are left as they are.
func (r *SystemSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removed DefectDojo system settings from the state; they are left unchanged")
}

func (r *SystemSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "system settings")
}
