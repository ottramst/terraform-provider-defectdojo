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
var _ datasource.DataSource = &UserDataSource{}
var _ datasource.DataSourceWithConfigure = &UserDataSource{}
var _ datasource.DataSourceWithConfigValidators = &UserDataSource{}

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

// UserDataSource looks up a user.
type UserDataSource struct {
	client *defectdojo.Client
}

// UserDataSourceModel describes the defectdojo_user data source.
type UserDataSourceModel struct {
	ID                       types.Int64  `tfsdk:"id"`
	Username                 types.String `tfsdk:"username"`
	Email                    types.String `tfsdk:"email"`
	FirstName                types.String `tfsdk:"first_name"`
	LastName                 types.String `tfsdk:"last_name"`
	IsActive                 types.Bool   `tfsdk:"is_active"`
	IsStaff                  types.Bool   `tfsdk:"is_staff"`
	IsSuperuser              types.Bool   `tfsdk:"is_superuser"`
	ConfigurationPermissions types.Set    `tfsdk:"configuration_permissions"`
	DateJoined               types.String `tfsdk:"date_joined"`
	LastLogin                types.String `tfsdk:"last_login"`
}

func (d *UserDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *UserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{MarkdownDescription: description, Computed: true}
	}
	flag := func(description string) schema.BoolAttribute {
		return schema.BoolAttribute{MarkdownDescription: description, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a DefectDojo user by ID or username, for example to give it access to a product " +
			"or name it as a product's contact. Reading users needs a superuser, or the configuration permissions for users; " +
			"reading `configuration_permissions` needs a superuser.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the user to look up. Exactly one of `id` and `username` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "The username of the user to look up, compared exactly, case included. Exactly one of `id` and `username` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"email":        computed("The email address of the user."),
			"first_name":   computed("The first name of the user."),
			"last_name":    computed("The last name of the user."),
			"is_active":    flag("Whether the user can sign in."),
			"is_staff":     flag("Whether the user is staff."),
			"is_superuser": flag("Whether the user is a superuser."),
			"configuration_permissions": schema.SetAttribute{
				MarkdownDescription: configurationPermissionsDescription,
				ElementType:         types.StringType,
				Computed:            true,
			},
			"date_joined": computed("When the user was created, in RFC 3339 format."),
			"last_login":  computed("When the user last signed in to the UI, in RFC 3339 format, or null if never."),
		},
	}
}

func (d *UserDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("username")),
	}
}

func (d *UserDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data UserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var (
		u    *defectdojo.User
		err  error
		what string
	)
	if !data.ID.IsNull() {
		what = fmt.Sprintf("ID %d", data.ID.ValueInt64())
		u, _, err = d.client.Users.Get(ctx, int(data.ID.ValueInt64()))
	} else {
		what = fmt.Sprintf("username %q", data.Username.ValueString())
		u, _, err = d.client.Users.GetByUsername(ctx, data.Username.ValueString())
	}
	if err != nil {
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.Diagnostics.AddError(
				"DefectDojo user not found",
				"There is no user with "+what+".",
			)
			return
		}
		resp.Diagnostics.AddError("Error reading DefectDojo user", "Could not read the user with "+what+": "+err.Error())
		return
	}

	var model UserResourceModel
	updateUserModel(&model, u)
	data.ID = model.ID
	data.Username = model.Username
	data.Email = model.Email
	data.FirstName = model.FirstName
	data.LastName = model.LastName
	data.IsActive = model.IsActive
	data.IsStaff = model.IsStaff
	data.IsSuperuser = model.IsSuperuser
	data.DateJoined = model.DateJoined
	data.LastLogin = timeOrNull(u.LastLogin)
	data.ConfigurationPermissions = permissionCodenames(ctx, d.client, u, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
