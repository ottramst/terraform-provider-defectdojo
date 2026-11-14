package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &NotificationWebhookResource{}
var _ resource.ResourceWithConfigure = &NotificationWebhookResource{}
var _ resource.ResourceWithImportState = &NotificationWebhookResource{}
var _ resource.ResourceWithIdentity = &NotificationWebhookResource{}

var webhookURLPattern = regexp.MustCompile(`^https?://\S+$`)

func NewNotificationWebhookResource() resource.Resource {
	return &NotificationWebhookResource{}
}

// NotificationWebhookResource defines the resource implementation.
type NotificationWebhookResource struct {
	client *defectdojo.Client
}

// NotificationWebhookResourceModel describes the resource data model.
type NotificationWebhookResourceModel struct {
	ID                   types.Int64  `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	URL                  types.String `tfsdk:"url"`
	HeaderName           types.String `tfsdk:"header_name"`
	HeaderValueWO        types.String `tfsdk:"header_value_wo"`
	HeaderValueWOVersion types.Int64  `tfsdk:"header_value_wo_version"`
	Status               types.String `tfsdk:"status"`
	Note                 types.String `tfsdk:"note"`
	FirstError           types.String `tfsdk:"first_error"`
	LastError            types.String `tfsdk:"last_error"`
}

func (r *NotificationWebhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_webhook"
}

func (r *NotificationWebhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DefectDojo system notification webhook, to which DefectDojo posts the notifications " +
			"whose settings (`defectdojo_notifications`) choose the `webhooks` channel. " +
			"Managing webhooks needs a superuser, and DefectDojo sends to them only when the system setting `enable_webhooks_notifications` is on.\n\n" +
			"DefectDojo deactivates a webhook whose deliveries fail (see `status`). " +
			"A webhook it deactivates permanently can't be reactivated through the API: edit it in DefectDojo's UI, " +
			"or create it again with `terraform apply -replace`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the webhook.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the webhook, at most 100 characters, without leading or trailing whitespace, which DefectDojo removes. " +
					"Names are unique, compared case-sensitively.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 100),
					noSurroundingWhitespace(),
				},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The HTTP or HTTPS URL to which DefectDojo posts notifications, at most 200 characters. DefectDojo doesn't test it.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 200),
					stringvalidator.RegexMatches(webhookURLPattern, "must be an http:// or https:// URL"),
				},
			},
			"header_name": schema.StringAttribute{
				MarkdownDescription: "The name of a header that DefectDojo sends with each notification, such as `Authorization`, " +
					"at most 100 characters. Its value is `header_value_wo`.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 100),
					noSurroundingWhitespace(),
				},
			},
			"header_value_wo": schema.StringAttribute{
				MarkdownDescription: "The value of the header named by `header_name`, such as a token, at most 100 characters, " +
					"which is never stored in the plan or state. Requires Terraform 1.11 or later, and `header_value_wo_version`. " +
					"Terraform can't detect a change of a write-only value: change `header_value_wo_version` to send it. " +
					"DefectDojo returns the value in plain text to every superuser.",
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 100),
					noSurroundingWhitespace(),
					stringvalidator.AlsoRequires(path.MatchRoot("header_value_wo_version")),
				},
			},
			"header_value_wo_version": schema.Int64Attribute{
				MarkdownDescription: "The version of `header_value_wo`. The provider sends the header value when the webhook is created, " +
					"and when the version changes, including when it is set for the first time, for example after an import. " +
					"Removing it, with `header_value_wo`, clears the header value.",
				Optional: true,
				Validators: []validator.Int64{
					int64validator.AlsoRequires(path.MatchRoot("header_value_wo")),
				},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "The status of the webhook, which DefectDojo sets when deliveries fail: " +
					"`active`; `inactive_tmp`, paused for about a minute after a 5xx or 429 response or a timeout; " +
					"`active_tmp`, active again after a temporary failure; or `inactive_permanent`, " +
					"after a 4xx response, a connection error, a day of errors, or a deactivation in the UI.",
				Computed: true,
			},
			"note": schema.StringAttribute{
				MarkdownDescription: "DefectDojo's description of the last error or status change, if any.",
				Computed:            true,
			},
			"first_error": schema.StringAttribute{
				MarkdownDescription: "When deliveries started failing, in RFC 3339 format, if they did.",
				Computed:            true,
			},
			"last_error": schema.StringAttribute{
				MarkdownDescription: "When a delivery last failed, in RFC 3339 format, if one did.",
				Computed:            true,
			},
		},
	}
}

func (r *NotificationWebhookResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("notification webhook")
}

func (r *NotificationWebhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *NotificationWebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NotificationWebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Write-only values are only in the configuration.
	var headerValue types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("header_value_wo"), &headerValue)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, _, err := r.client.NotificationWebhooks.Create(ctx, &defectdojo.NotificationWebhook{
		Name:        data.Name.ValueString(),
		URL:         data.URL.ValueString(),
		HeaderName:  data.HeaderName.ValueString(),
		HeaderValue: headerValue.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo notification webhook",
			"Could not create notification webhook "+strconv.Quote(data.Name.ValueString())+", unexpected error: "+err.Error(),
		)
		return
	}
	updateNotificationWebhookModel(&data, created)

	tflog.Trace(ctx, "created a notification webhook", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *NotificationWebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NotificationWebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	wh, _, err := r.client.NotificationWebhooks.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo notification webhook",
			fmt.Sprintf("Could not read notification webhook %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateNotificationWebhookModel(&data, wh)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *NotificationWebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state NotificationWebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	var headerValue types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("header_value_wo"), &headerValue)...)
	if resp.Diagnostics.HasError() {
		return
	}

	patch := &defectdojo.NotificationWebhookPatch{
		Name:       new(data.Name.ValueString()),
		URL:        new(data.URL.ValueString()),
		HeaderName: nullableString(data.HeaderName),
	}
	// The header value is sent only when its version changes: Terraform
	// can't compare write-only values.
	if !data.HeaderValueWOVersion.Equal(state.HeaderValueWOVersion) {
		patch.HeaderValue = nullableString(headerValue)
	}

	updated, _, err := r.client.NotificationWebhooks.Patch(ctx, int(data.ID.ValueInt64()), patch)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo notification webhook",
			fmt.Sprintf("Could not update notification webhook %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateNotificationWebhookModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *NotificationWebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NotificationWebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())
	if _, err := r.client.NotificationWebhooks.Delete(ctx, id); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo notification webhook",
			fmt.Sprintf("Could not delete notification webhook %d, unexpected error: %s", id, err),
		)
	}
}

func (r *NotificationWebhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "notification webhook")
}

// updateNotificationWebhookModel copies the webhook read from the server into
// data. The header value is never stored: DefectDojo returns it in plain text.
func updateNotificationWebhookModel(data *NotificationWebhookResourceModel, wh *defectdojo.NotificationWebhook) {
	data.ID = types.Int64Value(int64(wh.ID))
	data.Name = types.StringValue(wh.Name)
	data.URL = types.StringValue(wh.URL)
	data.HeaderName = stringOrNull(wh.HeaderName)
	data.HeaderValueWO = types.StringNull()
	data.Status = types.StringValue(wh.Status)
	data.Note = stringOrNull(wh.Note)
	data.FirstError = timeOrNull(wh.FirstError)
	data.LastError = timeOrNull(wh.LastError)
}
