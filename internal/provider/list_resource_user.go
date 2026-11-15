package provider

import (
	"context"
	"errors"
	"iter"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// UserListModel describes the configuration of the defectdojo_user list
// resource.
type UserListModel struct {
	UsernameContains types.String `tfsdk:"username_contains"`
}

func NewUserListResource() list.ListResource {
	return &listResource[UserListModel, defectdojo.User]{
		typeName: "_user",
		schema: schema.Schema{
			MarkdownDescription: "Lists the DefectDojo users, ordered by ID. Requires a superuser. " +
				"The generated configuration has no `password_wo`: DefectDojo's API can't change a password, so it only matters when a user is created." + listNotes,
			Attributes: map[string]schema.Attribute{
				"username_contains": listContainsAttribute("users", "username"),
			},
		},
		objects: "users",
		// DefectDojo's username filter matches a part of the username,
		// ignoring case.
		all: func(ctx context.Context, client *defectdojo.Client, config UserListModel) iter.Seq2[*defectdojo.User, error] {
			return client.Users.ListAll(ctx, &defectdojo.ListUsersOptions{Username: config.UsernameContains.ValueString()})
		},
		id:      func(u *defectdojo.User) int { return u.ID },
		results: userResults,
	}
}

// userResults builds the results of users as Read does after an import. The
// codenames of the configuration permissions are read once, when a user with
// permissions needs them; without a superuser they can't be read, and
// configuration_permissions is null.
func userResults(ctx context.Context, client *defectdojo.Client, req list.ListRequest, _ UserListModel) func(*defectdojo.User) list.ListResult {
	var codenames map[int]string
	var codenamesErr error
	loaded := false
	permissions := func(u *defectdojo.User, diags *diag.Diagnostics) types.Set {
		if len(u.ConfigurationPermissions) == 0 {
			return types.SetValueMust(types.StringType, nil)
		}
		first := !loaded
		if !loaded {
			codenames, codenamesErr = configurationPermissionCodenames(ctx, client)
			loaded = true
		}
		switch {
		case errors.Is(codenamesErr, defectdojo.ErrForbidden):
			if first {
				diags.AddWarning("Can't read DefectDojo configuration permissions",
					"Reading the codenames of configuration permissions needs a superuser: "+codenamesErr.Error()+". "+
						"configuration_permissions is null for the users that have some.")
			}
			return types.SetNull(types.StringType)
		case codenamesErr != nil:
			diags.AddError("Error reading DefectDojo configuration permissions", "Could not list the configuration permissions: "+codenamesErr.Error())
			return types.SetNull(types.StringType)
		}
		return codenameSet(u, codenames, diags)
	}
	return func(u *defectdojo.User) list.ListResult {
		var state UserResourceModel
		var diags diag.Diagnostics
		if req.IncludeResource {
			updateUserModel(&state, u)
			state.ConfigurationPermissions = permissions(u, &diags)
			if diags.HasError() {
				return errorResult(diags)
			}
		}
		result := newListResult(ctx, req, u.Username, u.ID, state)
		result.Diagnostics.Append(diags...)
		return result
	}
}
