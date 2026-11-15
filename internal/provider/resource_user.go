package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithConfigure = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}
var _ resource.ResourceWithIdentity = &UserResource{}

var (
	// usernamePattern is Django's username rule: Unicode letters and digits,
	// and @ . + - _.
	usernamePattern = regexp.MustCompile(`^[\p{L}\p{N}_.@+-]+$`)
	emailPattern    = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)
	codenamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)
)

func NewUserResource() resource.Resource {
	return &UserResource{}
}

// UserResource defines the resource implementation.
type UserResource struct {
	client *defectdojo.Client
}

// UserResourceModel describes the resource data model.
type UserResourceModel struct {
	ID                       types.Int64  `tfsdk:"id"`
	Username                 types.String `tfsdk:"username"`
	Email                    types.String `tfsdk:"email"`
	FirstName                types.String `tfsdk:"first_name"`
	LastName                 types.String `tfsdk:"last_name"`
	IsActive                 types.Bool   `tfsdk:"is_active"`
	IsStaff                  types.Bool   `tfsdk:"is_staff"`
	IsSuperuser              types.Bool   `tfsdk:"is_superuser"`
	ConfigurationPermissions types.Set    `tfsdk:"configuration_permissions"`
	PasswordWO               types.String `tfsdk:"password_wo"`
	DateJoined               types.String `tfsdk:"date_joined"`
}

func (r *UserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

// configurationPermissionsDescription describes configuration_permissions in
// the resource and the data source.
const configurationPermissionsDescription = "The codenames of the user's configuration permissions, which grant global rights, " +
	"such as `add_product_type` or `change_sla_configuration`. " +
	"Which codenames exist depends on DefectDojo's version and system settings: " +
	"the `*_jira_instance` permissions exist only with Jira enabled, `*_github_conf` only with GitHub, " +
	"and `*_engagement_survey` and `*_question` only with questionnaires; `*_tool_type` and `*_tool_configuration` go away with DefectDojo 3.6."

func (r *UserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	nameAttribute := func(which string) schema.StringAttribute {
		return schema.StringAttribute{
			MarkdownDescription: "The " + which + " name of the user, at most 150 characters, without leading or trailing whitespace, which DefectDojo removes.",
			Optional:            true,
			Validators: []validator.String{
				stringvalidator.UTF8LengthBetween(1, 150),
				noSurroundingWhitespace(),
			},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DefectDojo user, such as a service account for a CI/CD pipeline or Dependency-Track. " +
			"Managing users needs a superuser, or the configuration permissions for users; " +
			"only superusers can manage staff users, superusers, configuration permissions, and the username or email of another user.\n\n" +
			"DefectDojo creates the user's notification settings, which `defectdojo_notifications` can manage. " +
			"Destroying the user deletes them, with the user's alerts and notification webhooks. " +
			"DefectDojo refuses to delete a user that other objects refer to, such as the findings it imported or reported, " +
			"the tests it leads, or the products and engagements that name it as a contact or lead, " +
			"and the provider's own user: deactivate such users with `is_active = false` instead, " +
			"and remove them from the state with a `removed` block.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the user.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "The username, at most 150 letters, digits and `@`, `.`, `+`, `-` and `_`. " +
					"Usernames are unique, compared case-sensitively.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 150),
					stringvalidator.RegexMatches(usernamePattern, "must contain only letters, digits and @ . + - _"),
				},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The email address of the user, without leading or trailing whitespace. DefectDojo doesn't require it to be unique.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(3, 254),
					stringvalidator.RegexMatches(emailPattern, "must be an email address"),
					noSurroundingWhitespace(),
				},
			},
			"first_name": nameAttribute("first"),
			"last_name":  nameAttribute("last"),
			"is_active": schema.BoolAttribute{
				MarkdownDescription: "Whether the user can sign in. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"is_staff": schema.BoolAttribute{
				MarkdownDescription: "Whether the user is staff, with access to all product types and products and some configuration. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"is_superuser": schema.BoolAttribute{
				MarkdownDescription: "Whether the user is a superuser, with every permission. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"configuration_permissions": schema.SetAttribute{
				MarkdownDescription: configurationPermissionsDescription + " Managing them needs a superuser; " +
					"without one, the provider can't read them and keeps the value in the state, with a warning. " +
					"When it is not set, the provider leaves the permissions as they are, so they can be managed elsewhere. " +
					"An empty set removes all the user's permissions, including Django permissions granted outside DefectDojo's configuration permissions. " +
					"If they change elsewhere between the plan and the apply of another change to the user, the apply fails with an inconsistent result; plan and apply again.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.RegexMatches(codenamePattern, "must be a permission codename, such as add_product_type")),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"password_wo": schema.StringAttribute{
				MarkdownDescription: "The initial password of the user, which is never stored in the plan or state. Requires Terraform 1.11 or later. " +
					"DefectDojo's API can't change a password, so it is only sent when the user is created: " +
					"later changes, and the password of an imported user, are ignored; change passwords in DefectDojo's UI or through single sign-on. " +
					"It must meet DefectDojo's password rules (by default at least 9 characters, with a digit, an uppercase letter and a special character). " +
					"DefectDojo requires one unless `DD_REQUIRE_PASSWORD_ON_USER` is off; without one, the user can only sign in through single sign-on.",
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					noSurroundingWhitespace(),
				},
			},
			"date_joined": schema.StringAttribute{
				MarkdownDescription: "When the user was created, in RFC 3339 format.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *UserResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("user")
}

func (r *UserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Write-only values are only in the configuration.
	var password types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("password_wo"), &password)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u := &defectdojo.UserCreate{
		User: defectdojo.User{
			Username:    data.Username.ValueString(),
			Email:       data.Email.ValueString(),
			FirstName:   data.FirstName.ValueString(),
			LastName:    data.LastName.ValueString(),
			IsActive:    data.IsActive.ValueBool(),
			IsStaff:     data.IsStaff.ValueBool(),
			IsSuperuser: data.IsSuperuser.ValueBool(),
		},
		Password: password.ValueString(),
	}
	if known(data.ConfigurationPermissions) {
		u.ConfigurationPermissions = r.permissionIDs(ctx, data.ConfigurationPermissions, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	created, _, err := r.client.Users.Create(ctx, u)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo user",
			"Could not create user "+strconv.Quote(u.Username)+", unexpected error: "+err.Error(),
		)
		return
	}
	r.updateModel(ctx, &data, created, &resp.Diagnostics)

	tflog.Trace(ctx, "created a user", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, _, err := r.client.Users.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			removeGone(ctx, resp, data.ID)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo user",
			fmt.Sprintf("Could not read user %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	r.updateModel(ctx, &data, u, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The plan holds the prior permissions when they are not configured
	// (UseStateForUnknown), so check the configuration.
	var configured types.Set
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("configuration_permissions"), &configured)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The names can't be null: "" clears them.
	patch := &defectdojo.UserPatch{
		Username:    new(data.Username.ValueString()),
		Email:       new(data.Email.ValueString()),
		FirstName:   new(data.FirstName.ValueString()),
		LastName:    new(data.LastName.ValueString()),
		IsActive:    new(data.IsActive.ValueBool()),
		IsStaff:     new(data.IsStaff.ValueBool()),
		IsSuperuser: new(data.IsSuperuser.ValueBool()),
	}
	if known(configured) {
		patch.ConfigurationPermissions = new(r.permissionIDs(ctx, configured, &resp.Diagnostics))
		if resp.Diagnostics.HasError() {
			return
		}
	}

	updated, _, err := r.client.Users.Patch(ctx, int(data.ID.ValueInt64()), patch)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo user",
			fmt.Sprintf("Could not update user %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	r.updateModel(ctx, &data, updated, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())

	_, err := r.client.Users.Delete(ctx, id)
	switch {
	case err == nil, errors.Is(err, defectdojo.ErrNotFound):
	case errors.Is(err, defectdojo.ErrConflict):
		resp.Diagnostics.AddError(
			"DefectDojo user is in use",
			fmt.Sprintf("User %d (%q) can't be deleted while other objects refer to it, such as the findings it imported or reported, "+
				"the tests it leads, or products and engagements that name it as a contact or lead: %s\n\n"+
				"Keep the user and deactivate it with is_active = false (and remove it from the state with a removed block), "+
				"or remove those references first.", id, data.Username.ValueString(), err),
		)
	case errors.Is(err, defectdojo.ErrBadRequest):
		resp.Diagnostics.AddError(
			"DefectDojo user can't be deleted",
			fmt.Sprintf("Could not delete user %d (%q): %s\n\nDefectDojo doesn't let a user delete itself, and the provider signs in as this user.",
				id, data.Username.ValueString(), err),
		)
	default:
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo user",
			fmt.Sprintf("Could not delete user %d, unexpected error: %s", id, err),
		)
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "user")
}

// permissionIDs returns the IDs of the configuration permissions with the
// codenames in set. The IDs are database IDs, which differ between DefectDojo
// installations.
func (r *UserResource) permissionIDs(ctx context.Context, set types.Set, diags *diag.Diagnostics) []int {
	ids := []int{}
	for _, codename := range stringsFromSet(ctx, set, diags) {
		p, _, err := r.client.ConfigurationPermissions.GetByCodename(ctx, codename)
		if errors.Is(err, defectdojo.ErrNotFound) {
			diags.AddAttributeError(path.Root("configuration_permissions"),
				"Unknown DefectDojo configuration permission",
				fmt.Sprintf("DefectDojo has no configuration permission %q. Which ones exist depends on its version and system settings "+
					"(Jira, GitHub and questionnaires); GET /api/v2/configuration_permissions/ lists them.", codename))
			continue
		}
		if err != nil {
			diags.AddError("Error reading DefectDojo configuration permissions",
				fmt.Sprintf("Could not look up configuration permission %q: %s", codename, err))
			return nil
		}
		ids = append(ids, p.ID)
	}
	return ids
}

// updateModel copies the user read from the server into data, mapping the
// configuration permission IDs to codenames. When the provider may not read
// the configuration permissions, the planned or prior ones are kept.
func (r *UserResource) updateModel(ctx context.Context, data *UserResourceModel, u *defectdojo.User, diags *diag.Diagnostics) {
	prior := data.ConfigurationPermissions
	updateUserModel(data, u)
	if !known(prior) {
		prior = types.SetNull(types.StringType)
	}
	data.ConfigurationPermissions = permissionCodenames(ctx, r.client, u, prior, diags)
}

// updateUserModel copies the fields of the user other than its configuration
// permissions into data.
func updateUserModel(data *UserResourceModel, u *defectdojo.User) {
	data.ID = types.Int64Value(int64(u.ID))
	data.Username = types.StringValue(u.Username)
	data.Email = types.StringValue(u.Email)
	data.FirstName = stringOrNull(u.FirstName)
	data.LastName = stringOrNull(u.LastName)
	data.IsActive = types.BoolValue(u.IsActive)
	data.IsStaff = types.BoolValue(u.IsStaff)
	data.IsSuperuser = types.BoolValue(u.IsSuperuser)
	data.DateJoined = timeOrNull(u.DateJoined)
}

// permissionCodenames returns the codenames of the user's configuration
// permissions. It reads the configuration permissions only when the user has
// some. Reading them needs a superuser: without one, it returns fallback with
// a warning. A permission that the list lacks is left out with a warning.
func permissionCodenames(ctx context.Context, client *defectdojo.Client, u *defectdojo.User, fallback types.Set, diags *diag.Diagnostics) types.Set {
	if len(u.ConfigurationPermissions) == 0 {
		return types.SetValueMust(types.StringType, nil)
	}
	codenames, err := configurationPermissionCodenames(ctx, client)
	if errors.Is(err, defectdojo.ErrForbidden) {
		diags.AddWarning("Can't read DefectDojo configuration permissions",
			fmt.Sprintf("User %d has configuration permissions, but reading their codenames needs a superuser: %s. "+
				"configuration_permissions keeps its previous value.", u.ID, err))
		return fallback
	}
	if err != nil {
		diags.AddError("Error reading DefectDojo configuration permissions",
			fmt.Sprintf("Could not list the configuration permissions of user %d: %s", u.ID, err))
		return types.SetNull(types.StringType)
	}
	return codenameSet(u, codenames, diags)
}

// configurationPermissionCodenames returns the codenames of DefectDojo's
// configuration permissions by ID. Reading them needs a superuser.
func configurationPermissionCodenames(ctx context.Context, client *defectdojo.Client) (map[int]string, error) {
	codenames := make(map[int]string)
	for p, err := range client.ConfigurationPermissions.ListAll(ctx, nil) {
		if err != nil {
			return nil, err
		}
		codenames[p.ID] = p.Codename
	}
	return codenames, nil
}

// codenameSet returns the codenames of the user's configuration permissions,
// from the codenames by ID. A permission that codenames lacks is left out
// with a warning.
func codenameSet(u *defectdojo.User, codenames map[int]string, diags *diag.Diagnostics) types.Set {
	names := []string{}
	var unknown []string
	for _, id := range u.ConfigurationPermissions {
		if name, ok := codenames[id]; ok {
			names = append(names, name)
		} else {
			unknown = append(unknown, strconv.Itoa(id))
		}
	}
	if len(unknown) > 0 {
		diags.AddWarning("Unknown DefectDojo configuration permissions",
			fmt.Sprintf("User %d has permissions with IDs %s, which DefectDojo doesn't list as configuration permissions, "+
				"so they are left out of configuration_permissions.",
				u.ID, strings.Join(unknown, ", ")))
	}
	slices.Sort(names)
	return stringSet(names)
}

// stringSet returns a set of the strings.
func stringSet(values []string) types.Set {
	elems := make([]attr.Value, len(values))
	for i, v := range values {
		elems[i] = types.StringValue(v)
	}
	return types.SetValueMust(types.StringType, elems)
}

// timeOrNull returns t in RFC 3339 format, or null for the zero time.
func timeOrNull(t time.Time) types.String {
	if t.IsZero() {
		return types.StringNull()
	}
	return types.StringValue(t.Format(time.RFC3339Nano))
}
