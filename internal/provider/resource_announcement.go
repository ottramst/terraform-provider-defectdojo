package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &AnnouncementResource{}
var _ resource.ResourceWithConfigure = &AnnouncementResource{}
var _ resource.ResourceWithImportState = &AnnouncementResource{}
var _ resource.ResourceWithIdentity = &AnnouncementResource{}

func NewAnnouncementResource() resource.Resource {
	return &AnnouncementResource{}
}

// AnnouncementResource defines the resource implementation.
type AnnouncementResource struct {
	client *defectdojo.Client
}

// AnnouncementResourceModel describes the resource data model.
type AnnouncementResourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Message     types.String `tfsdk:"message"`
	Style       types.String `tfsdk:"style"`
	Dismissable types.Bool   `tfsdk:"dismissable"`
}

func (r *AnnouncementResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_announcement"
}

func (r *AnnouncementResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages DefectDojo's announcement: a banner on every page for every user. " +
			"DefectDojo keeps at most one announcement, and it always has the ID 1. " +
			"Managing it needs a superuser: `change_announcement` is the only configuration permission for it, and reading, creating and deleting it need more.\n\n" +
			"A change of `message` or `style` replaces the announcement, as DefectDojo's UI does, so that every user sees the new one, " +
			"including users who dismissed the old one. `create_before_destroy` can't work, since DefectDojo keeps only one announcement. " +
			"Creating the resource fails when an announcement exists; import it with the ID 1 instead.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the announcement, always 1.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"message": schema.StringAttribute{
				MarkdownDescription: "The text of the banner, at most 500 characters, without leading or trailing whitespace, which DefectDojo removes. " +
					"It may contain basic HTML, such as links. Changing it replaces the announcement, so every user sees it again.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 500),
					noSurroundingWhitespace(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"style": schema.StringAttribute{
				MarkdownDescription: "The style of the banner: one of `info`, `success`, `warning` and `danger`. Defaults to `info`. " +
					"Changing it replaces the announcement, so every user sees it again.",
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString(defectdojo.AnnouncementStyleInfo),
				Validators: []validator.String{stringvalidator.OneOf(defectdojo.AnnouncementStyleInfo, defectdojo.AnnouncementStyleSuccess, defectdojo.AnnouncementStyleWarning, defectdojo.AnnouncementStyleDanger)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"dismissable": schema.BoolAttribute{
				MarkdownDescription: "Whether users can dismiss the banner. Defaults to `false`. Changing it doesn't replace the announcement, " +
					"so turning it off doesn't show the banner again to users who dismissed it; change the message to replace it. " +
					"A dismissal doesn't last either: DefectDojo shows the banner again to a user whenever their user record is saved.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
		},
	}
}

func (r *AnnouncementResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("announcement")
}

func (r *AnnouncementResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *AnnouncementResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AnnouncementResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// DefectDojo's own error says only "No more than one Announcement is
	// allowed".
	current, _, err := r.client.Announcements.Current(ctx)
	switch {
	case err == nil:
		resp.Diagnostics.AddError(
			"DefectDojo announcement exists",
			fmt.Sprintf("DefectDojo already has an announcement (ID %d), and keeps only one. "+
				"Import it with an import block or terraform import, with the ID %d, or delete it first.", current.ID, current.ID),
		)
		return
	case !errors.Is(err, defectdojo.ErrNotFound):
		resp.Diagnostics.AddError("Error creating DefectDojo announcement", "Could not read the current announcement: "+err.Error())
		return
	}

	created, _, err := r.client.Announcements.Create(ctx, &defectdojo.Announcement{
		Message:     data.Message.ValueString(),
		Style:       data.Style.ValueString(),
		Dismissable: data.Dismissable.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating DefectDojo announcement", "Could not create the announcement, unexpected error: "+err.Error())
		return
	}
	updateAnnouncementModel(&data, created)

	tflog.Trace(ctx, "created the announcement", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *AnnouncementResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AnnouncementResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, _, err := r.client.Announcements.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo announcement",
			fmt.Sprintf("Could not read announcement %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateAnnouncementModel(&data, a)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *AnnouncementResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AnnouncementResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A change of the message or style replaces the announcement, so only
	// dismissable changes in place.
	updated, _, err := r.client.Announcements.Patch(ctx, int(data.ID.ValueInt64()), &defectdojo.AnnouncementPatch{
		Dismissable: new(data.Dismissable.ValueBool()),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo announcement",
			fmt.Sprintf("Could not update announcement %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateAnnouncementModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *AnnouncementResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AnnouncementResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.Announcements.Delete(ctx, int(data.ID.ValueInt64())); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo announcement",
			fmt.Sprintf("Could not delete announcement %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
	}
}

func (r *AnnouncementResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "announcement")
}

// updateAnnouncementModel copies the announcement read from the server into
// data.
func updateAnnouncementModel(data *AnnouncementResourceModel, a *defectdojo.Announcement) {
	data.ID = types.Int64Value(int64(a.ID))
	data.Message = types.StringValue(a.Message)
	data.Style = types.StringValue(a.Style)
	data.Dismissable = types.BoolValue(a.Dismissable)
}
