package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &NotificationsResource{}
var _ resource.ResourceWithConfigure = &NotificationsResource{}
var _ resource.ResourceWithImportState = &NotificationsResource{}
var _ resource.ResourceWithIdentity = &NotificationsResource{}
var _ resource.ResourceWithConfigValidators = &NotificationsResource{}

// notificationChannels are the channels DefectDojo can notify on.
var notificationChannels = []string{
	string(defectdojo.NotificationChannelSlack),
	string(defectdojo.NotificationChannelMSTeams),
	string(defectdojo.NotificationChannelMail),
	string(defectdojo.NotificationChannelWebhooks),
	string(defectdojo.NotificationChannelAlert),
}

func NewNotificationsResource() resource.Resource {
	return &NotificationsResource{}
}

// NotificationsResource defines the resource implementation.
type NotificationsResource struct {
	client *defectdojo.Client
}

// NotificationsResourceModel describes the resource data model.
type NotificationsResourceModel struct {
	ID        types.Int64 `tfsdk:"id"`
	UserID    types.Int64 `tfsdk:"user_id"`
	ProductID types.Int64 `tfsdk:"product_id"`
	Template  types.Bool  `tfsdk:"template"`

	ProductTypeAdded         types.Set `tfsdk:"product_type_added"`
	ProductAdded             types.Set `tfsdk:"product_added"`
	EngagementAdded          types.Set `tfsdk:"engagement_added"`
	TestAdded                types.Set `tfsdk:"test_added"`
	ScanAdded                types.Set `tfsdk:"scan_added"`
	JiraUpdate               types.Set `tfsdk:"jira_update"`
	UpcomingEngagement       types.Set `tfsdk:"upcoming_engagement"`
	StaleEngagement          types.Set `tfsdk:"stale_engagement"`
	AutoCloseEngagement      types.Set `tfsdk:"auto_close_engagement"`
	CloseEngagement          types.Set `tfsdk:"close_engagement"`
	UserMentioned            types.Set `tfsdk:"user_mentioned"`
	CodeReview               types.Set `tfsdk:"code_review"`
	ReviewRequested          types.Set `tfsdk:"review_requested"`
	Other                    types.Set `tfsdk:"other"`
	SLABreach                types.Set `tfsdk:"sla_breach"`
	SLABreachCombined        types.Set `tfsdk:"sla_breach_combined"`
	RiskAcceptanceExpiration types.Set `tfsdk:"risk_acceptance_expiration"`
	ScanAddedEmpty           types.Set `tfsdk:"scan_added_empty"`
}

func (r *NotificationsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notifications"
}

// eventAttribute returns the attribute of an event: the channels DefectDojo
// notifies on, by default alert.
func eventAttribute(when string) schema.SetAttribute {
	return schema.SetAttribute{
		MarkdownDescription: "The channels on which DefectDojo notifies when " + when + ". Defaults to `[\"alert\"]`.",
		ElementType:         types.StringType,
		Optional:            true,
		Computed:            true,
		Default:             setdefault.StaticValue(channelSet([]defectdojo.NotificationChannel{defectdojo.NotificationChannelAlert})),
		Validators: []validator.Set{
			setvalidator.ValueStringsAre(stringvalidator.OneOf(notificationChannels...)),
		},
	}
}

func (r *NotificationsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			MarkdownDescription: "The ID of the notification settings.",
			Computed:            true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"user_id": schema.Int64Attribute{
			MarkdownDescription: "The ID of the user whose notification settings these are. " +
				"Without `user_id` and `template`, these are the system-wide settings. Changing it forces a new resource.",
			Optional: true,
			Validators: []validator.Int64{
				int64validator.AtLeast(1),
			},
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.RequiresReplace(),
			},
		},
		"product_id": schema.Int64Attribute{
			MarkdownDescription: "The ID of the product the user's settings apply to; they override the user's personal settings for it. " +
				"Requires `user_id`. Changing it forces a new resource.",
			Optional: true,
			Validators: []validator.Int64{
				int64validator.AtLeast(1),
				int64validator.AlsoRequires(path.MatchRoot("user_id")),
			},
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.RequiresReplace(),
			},
		},
		"template": schema.BoolAttribute{
			MarkdownDescription: "Whether these are the template that DefectDojo copies into the personal settings of each new user. " +
				"Conflicts with `user_id` and `product_id`. Defaults to `false`. Changing it forces a new resource.",
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(false),
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.RequiresReplace(),
			},
		},
		"scan_added_empty": schema.SetAttribute{
			MarkdownDescription: "The channel on which DefectDojo notifies when an import or reimport changes no findings. " +
				"DefectDojo's API can set at most one channel for this event (its UI can set several). Defaults to none.",
			ElementType: types.StringType,
			Optional:    true,
			Computed:    true,
			Default:     setdefault.StaticValue(channelSet([]defectdojo.NotificationChannel{})),
			Validators: []validator.Set{
				setvalidator.SizeAtMost(1),
				setvalidator.ValueStringsAre(stringvalidator.OneOf(notificationChannels...)),
			},
		},
	}
	attrs["product_type_added"] = eventAttribute("a product type is added")
	attrs["product_added"] = eventAttribute("a product is added")
	attrs["engagement_added"] = eventAttribute("an engagement is added")
	attrs["test_added"] = eventAttribute("a test is added")
	attrs["scan_added"] = eventAttribute("an import or reimport creates, updates or closes findings")
	attrs["jira_update"] = eventAttribute("the background Jira synchronization fails")
	attrs["upcoming_engagement"] = eventAttribute("an engagement is about to start")
	attrs["stale_engagement"] = eventAttribute("an engagement is past its end date")
	attrs["auto_close_engagement"] = eventAttribute("DefectDojo closes an engagement automatically")
	attrs["close_engagement"] = eventAttribute("an engagement is closed")
	attrs["user_mentioned"] = eventAttribute("the user is mentioned in a note")
	attrs["code_review"] = eventAttribute("a code review is requested")
	attrs["review_requested"] = eventAttribute("a review of findings is requested")
	attrs["other"] = eventAttribute("other events, such as a revoked API token")
	attrs["sla_breach"] = eventAttribute("a finding breaches its SLA")
	attrs["sla_breach_combined"] = eventAttribute("findings breach their SLA, in one combined notification")
	attrs["risk_acceptance_expiration"] = eventAttribute("a risk acceptance expires")
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages DefectDojo notification settings: on which channels (`slack`, `msteams`, `mail`, `webhooks`, `alert`) " +
			"DefectDojo notifies of each event. The settings are system-wide, the template for new users, a user's personal settings, " +
			"or a user's settings for one product. Managing them needs a superuser. " +
			"Slack, Microsoft Teams, mail and webhooks also need to be turned on in DefectDojo's system settings (`defectdojo_system_settings`), " +
			"and webhooks need a `defectdojo_notification_webhook`.\n\n" +
			"DefectDojo creates the system-wide settings and each user's personal settings itself, so creating this resource adopts " +
			"existing settings of its scope. The resource manages every event: an event that isn't configured gets DefectDojo's default " +
			"(`alert`, and none for `scan_added_empty`), so don't manage one scope with two resources.\n\n" +
			"Destroying the system-wide or a user's personal settings resets them to those defaults, without the template; " +
			"deleting a user's personal settings would stop the user's notifications. " +
			"Destroying a user's settings for a product, or the template, deletes them.",
		Attributes: attrs,
	}
}

func (r *NotificationsResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{templateScopeValidator{}}
}

// templateScopeValidator rejects a user or product for the template.
// ConflictsWith would also reject an explicit template = false.
type templateScopeValidator struct{}

func (v templateScopeValidator) Description(_ context.Context) string {
	return "template = true conflicts with user_id and product_id"
}

func (v templateScopeValidator) MarkdownDescription(_ context.Context) string {
	return "`template = true` conflicts with `user_id` and `product_id`"
}

func (v templateScopeValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var template types.Bool
	var user, product types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("template"), &template)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("user_id"), &user)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("product_id"), &product)...)
	if resp.Diagnostics.HasError() || !template.ValueBool() {
		return
	}
	if !user.IsNull() || !product.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("template"), "Invalid notification settings scope",
			"The template has no user or product: remove user_id and product_id, or set template = false.")
	}
}

func (r *NotificationsResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("notification settings")
}

func (r *NotificationsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *NotificationsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NotificationsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	user, product, template := int(data.UserID.ValueInt64()), int(data.ProductID.ValueInt64()), data.Template.ValueBool()
	scope := notificationsScope(user, product, template)

	// DefectDojo creates the system-wide and personal settings itself: adopt
	// existing settings of the scope.
	existing, _, err := r.client.Notifications.GetByScope(ctx, defectdojo.NotificationScope{User: user, Product: product, Template: template}, nil)
	var result *defectdojo.NotificationSettings
	switch {
	case err == nil:
		patch := notificationsPatch(ctx, &data, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		result, _, err = r.client.Notifications.Patch(ctx, existing.ID, patch, nil)
		tflog.Debug(ctx, "adopted existing notification settings", map[string]any{"id": existing.ID})
	case errors.Is(err, defectdojo.ErrNotFound):
		settings := notificationSettings(ctx, &data, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		settings.User, settings.Product, settings.Template = user, product, template
		result, _, err = r.client.Notifications.Create(ctx, settings, nil)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo notification settings",
			fmt.Sprintf("Could not create or adopt the notification settings of %s, unexpected error: %s", scope, err),
		)
		return
	}
	updateNotificationsModel(&data, result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *NotificationsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NotificationsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	n, _, err := r.client.Notifications.Get(ctx, int(data.ID.ValueInt64()), nil)
	if err != nil {
		// Deleted outside Terraform, or with its user or product
		if errors.Is(err, defectdojo.ErrNotFound) {
			removeGone(ctx, resp, data.ID)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo notification settings",
			fmt.Sprintf("Could not read notification settings %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateNotificationsModel(&data, n)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *NotificationsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NotificationsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := notificationsPatch(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// PATCH: a PUT would reset the scope, and can't update the template.
	updated, _, err := r.client.Notifications.Patch(ctx, int(data.ID.ValueInt64()), patch, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo notification settings",
			fmt.Sprintf("Could not update notification settings %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateNotificationsModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *NotificationsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NotificationsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())

	var err error
	if data.ProductID.IsNull() && !data.Template.ValueBool() {
		// The system-wide or a user's personal settings, which DefectDojo
		// needs: reset them to its defaults.
		_, _, err = r.client.Notifications.Patch(ctx, id, defaultNotificationsPatch(), nil)
	} else {
		_, err = r.client.Notifications.Delete(ctx, id, nil)
	}
	if err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo notification settings",
			fmt.Sprintf("Could not delete or reset notification settings %d, unexpected error: %s", id, err),
		)
	}
}

func (r *NotificationsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "notification settings")
}

// notificationsScope describes the scope of notification settings in
// errors.
func notificationsScope(user, product int, template bool) string {
	switch {
	case template:
		return "the template"
	case user == 0:
		return "the system"
	case product == 0:
		return fmt.Sprintf("user %d", user)
	}
	return fmt.Sprintf("user %d and product %d", user, product)
}

// channels returns the channels of an event's set.
func channels(ctx context.Context, set types.Set, diags *diag.Diagnostics) []defectdojo.NotificationChannel {
	values := []defectdojo.NotificationChannel{}
	for _, v := range stringsFromSet(ctx, set, diags) {
		values = append(values, defectdojo.NotificationChannel(v))
	}
	return values
}

// notificationSettings returns the events of data.
func notificationSettings(ctx context.Context, data *NotificationsResourceModel, diags *diag.Diagnostics) *defectdojo.NotificationSettings {
	return &defectdojo.NotificationSettings{
		ProductTypeAdded:         channels(ctx, data.ProductTypeAdded, diags),
		ProductAdded:             channels(ctx, data.ProductAdded, diags),
		EngagementAdded:          channels(ctx, data.EngagementAdded, diags),
		TestAdded:                channels(ctx, data.TestAdded, diags),
		ScanAdded:                channels(ctx, data.ScanAdded, diags),
		JiraUpdate:               channels(ctx, data.JiraUpdate, diags),
		UpcomingEngagement:       channels(ctx, data.UpcomingEngagement, diags),
		StaleEngagement:          channels(ctx, data.StaleEngagement, diags),
		AutoCloseEngagement:      channels(ctx, data.AutoCloseEngagement, diags),
		CloseEngagement:          channels(ctx, data.CloseEngagement, diags),
		UserMentioned:            channels(ctx, data.UserMentioned, diags),
		CodeReview:               channels(ctx, data.CodeReview, diags),
		ReviewRequested:          channels(ctx, data.ReviewRequested, diags),
		Other:                    channels(ctx, data.Other, diags),
		SLABreach:                channels(ctx, data.SLABreach, diags),
		SLABreachCombined:        channels(ctx, data.SLABreachCombined, diags),
		RiskAcceptanceExpiration: channels(ctx, data.RiskAcceptanceExpiration, diags),
		ScanAddedEmpty:           channels(ctx, data.ScanAddedEmpty, diags),
	}
}

// notificationsPatch returns a patch of every event of data.
func notificationsPatch(ctx context.Context, data *NotificationsResourceModel, diags *diag.Diagnostics) *defectdojo.NotificationSettingsPatch {
	var scanAddedEmpty defectdojo.NotificationChannel
	if c := channels(ctx, data.ScanAddedEmpty, diags); len(c) > 0 {
		scanAddedEmpty = c[0]
	}
	return &defectdojo.NotificationSettingsPatch{
		ProductTypeAdded:         new(channels(ctx, data.ProductTypeAdded, diags)),
		ProductAdded:             new(channels(ctx, data.ProductAdded, diags)),
		EngagementAdded:          new(channels(ctx, data.EngagementAdded, diags)),
		TestAdded:                new(channels(ctx, data.TestAdded, diags)),
		ScanAdded:                new(channels(ctx, data.ScanAdded, diags)),
		JiraUpdate:               new(channels(ctx, data.JiraUpdate, diags)),
		UpcomingEngagement:       new(channels(ctx, data.UpcomingEngagement, diags)),
		StaleEngagement:          new(channels(ctx, data.StaleEngagement, diags)),
		AutoCloseEngagement:      new(channels(ctx, data.AutoCloseEngagement, diags)),
		CloseEngagement:          new(channels(ctx, data.CloseEngagement, diags)),
		UserMentioned:            new(channels(ctx, data.UserMentioned, diags)),
		CodeReview:               new(channels(ctx, data.CodeReview, diags)),
		ReviewRequested:          new(channels(ctx, data.ReviewRequested, diags)),
		Other:                    new(channels(ctx, data.Other, diags)),
		SLABreach:                new(channels(ctx, data.SLABreach, diags)),
		SLABreachCombined:        new(channels(ctx, data.SLABreachCombined, diags)),
		RiskAcceptanceExpiration: new(channels(ctx, data.RiskAcceptanceExpiration, diags)),
		ScanAddedEmpty:           &scanAddedEmpty,
	}
}

// defaultNotificationsPatch returns a patch that sets every event to
// DefectDojo's model defaults: alert, and no channel for scan_added_empty.
func defaultNotificationsPatch() *defectdojo.NotificationSettingsPatch {
	alert := []defectdojo.NotificationChannel{defectdojo.NotificationChannelAlert}
	return &defectdojo.NotificationSettingsPatch{
		ProductTypeAdded:         &alert,
		ProductAdded:             &alert,
		EngagementAdded:          &alert,
		TestAdded:                &alert,
		ScanAdded:                &alert,
		JiraUpdate:               &alert,
		UpcomingEngagement:       &alert,
		StaleEngagement:          &alert,
		AutoCloseEngagement:      &alert,
		CloseEngagement:          &alert,
		UserMentioned:            &alert,
		CodeReview:               &alert,
		ReviewRequested:          &alert,
		Other:                    &alert,
		SLABreach:                &alert,
		SLABreachCombined:        &alert,
		RiskAcceptanceExpiration: &alert,
		ScanAddedEmpty:           new(defectdojo.NotificationChannel("")),
	}
}

// updateNotificationsModel copies the notification settings read from the
// server into data.
func updateNotificationsModel(data *NotificationsResourceModel, n *defectdojo.NotificationSettings) {
	data.ID = types.Int64Value(int64(n.ID))
	data.UserID = int64OrNull(n.User)
	data.ProductID = int64OrNull(n.Product)
	data.Template = types.BoolValue(n.Template)
	data.ProductTypeAdded = channelSet(n.ProductTypeAdded)
	data.ProductAdded = channelSet(n.ProductAdded)
	data.EngagementAdded = channelSet(n.EngagementAdded)
	data.TestAdded = channelSet(n.TestAdded)
	data.ScanAdded = channelSet(n.ScanAdded)
	data.JiraUpdate = channelSet(n.JiraUpdate)
	data.UpcomingEngagement = channelSet(n.UpcomingEngagement)
	data.StaleEngagement = channelSet(n.StaleEngagement)
	data.AutoCloseEngagement = channelSet(n.AutoCloseEngagement)
	data.CloseEngagement = channelSet(n.CloseEngagement)
	data.UserMentioned = channelSet(n.UserMentioned)
	data.CodeReview = channelSet(n.CodeReview)
	data.ReviewRequested = channelSet(n.ReviewRequested)
	data.Other = channelSet(n.Other)
	data.SLABreach = channelSet(n.SLABreach)
	data.SLABreachCombined = channelSet(n.SLABreachCombined)
	data.RiskAcceptanceExpiration = channelSet(n.RiskAcceptanceExpiration)
	data.ScanAddedEmpty = channelSet(n.ScanAddedEmpty)
}

// channelSet returns a set of the channels.
func channelSet(channels []defectdojo.NotificationChannel) types.Set {
	elems := make([]attr.Value, len(channels))
	for i, c := range channels {
		elems[i] = types.StringValue(string(c))
	}
	return types.SetValueMust(types.StringType, elems)
}
