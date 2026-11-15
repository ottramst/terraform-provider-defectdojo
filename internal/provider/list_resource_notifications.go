package provider

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// NotificationsListModel describes the configuration of the
// defectdojo_notifications list resource.
type NotificationsListModel struct {
	UserID    types.Int64 `tfsdk:"user_id"`
	ProductID types.Int64 `tfsdk:"product_id"`
	Template  types.Bool  `tfsdk:"template"`
}

func NewNotificationsListResource() list.ListResource {
	id := func(n *defectdojo.NotificationSettings) int { return n.ID }
	return &listResource[NotificationsListModel, defectdojo.NotificationSettings]{
		typeName: "_notifications",
		schema: schema.Schema{
			MarkdownDescription: "Lists DefectDojo's notification settings, ordered by ID, with the display name `system-wide`, `template`, `user <ID>`, `product <ID>` or `user <ID>, product <ID>`. " +
				"DefectDojo creates personal settings for users, so the list is long unless it is filtered." + listNotes,
			Attributes: map[string]schema.Attribute{
				"user_id":    listIDAttribute("Only list the settings of the user with this ID."),
				"product_id": listIDAttribute("Only list the settings of the product with this ID."),
				"template": schema.BoolAttribute{
					MarkdownDescription: "Only list the template for new users (`true`), or only the other settings (`false`).",
					Optional:            true,
				},
			},
		},
		objects: "notification settings",
		all: func(ctx context.Context, client *defectdojo.Client, config NotificationsListModel) iter.Seq2[*defectdojo.NotificationSettings, error] {
			opts := &defectdojo.ListNotificationsOptions{
				User:    int(config.UserID.ValueInt64()),
				Product: int(config.ProductID.ValueInt64()),
			}
			if known(config.Template) {
				opts.Template = new(config.Template.ValueBool())
			}
			return client.Notifications.ListAll(ctx, opts)
		},
		id: id,
		results: modelResults[NotificationsListModel](id, notificationsDisplayName,
			updateNotificationsModel),
		failure: func(err error, config NotificationsListModel) diag.Diagnostics {
			// The user and product filters reject objects that don't exist
			// with a 400.
			var errResp *defectdojo.ErrorResponse
			if errors.As(err, &errResp) && errResp.StatusCode == http.StatusBadRequest {
				var diags diag.Diagnostics
				switch {
				case len(errResp.Fields["user"]) > 0:
					diags.AddError("DefectDojo user not found",
						fmt.Sprintf("There is no user with the ID %d, or the provider user may not see it.", config.UserID.ValueInt64()))
					return diags
				case len(errResp.Fields["product"]) > 0:
					diags.AddError("DefectDojo product not found",
						fmt.Sprintf("There is no product with the ID %d, or the provider user may not see it.", config.ProductID.ValueInt64()))
					return diags
				}
			}
			return listError("notification settings")(err)
		},
	}
}

// notificationsDisplayName names the scope of notification settings.
func notificationsDisplayName(n *defectdojo.NotificationSettings) string {
	switch {
	case n.Template:
		return "template"
	case n.User != 0 && n.Product != 0:
		return fmt.Sprintf("user %d, product %d", n.User, n.Product)
	case n.User != 0:
		return fmt.Sprintf("user %d", n.User)
	case n.Product != 0:
		return fmt.Sprintf("product %d", n.Product)
	}
	return "system-wide"
}
