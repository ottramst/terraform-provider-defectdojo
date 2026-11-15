package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &NoteTypeResource{}
var _ resource.ResourceWithConfigure = &NoteTypeResource{}
var _ resource.ResourceWithImportState = &NoteTypeResource{}
var _ resource.ResourceWithIdentity = &NoteTypeResource{}

func NewNoteTypeResource() resource.Resource {
	return &NoteTypeResource{}
}

// NoteTypeResource defines the resource implementation.
type NoteTypeResource struct {
	client *defectdojo.Client
}

// NoteTypeResourceModel describes the resource data model.
type NoteTypeResourceModel struct {
	ID           types.Int64  `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	IsSingle     types.Bool   `tfsdk:"is_single"`
	IsActive     types.Bool   `tfsdk:"is_active"`
	IsMandatory  types.Bool   `tfsdk:"is_mandatory"`
	ForceDestroy types.Bool   `tfsdk:"force_destroy"`
}

func (r *NoteTypeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_note_type"
}

func (r *NoteTypeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	flag := func(description string, value bool) schema.BoolAttribute {
		return schema.BoolAttribute{
			MarkdownDescription: description + " Defaults to `" + strconv.FormatBool(value) + "`, as in DefectDojo.",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(value),
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DefectDojo note type, which classifies the notes on findings, tests and engagements. " +
			"Managing note types needs a superuser or the note type configuration permissions.\n\n" +
			"~> **Deleting a note type deletes every note of the type.** " +
			"The provider refuses to destroy a note type that has notes unless `force_destroy` is set.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the note type.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name":        requiredText("The name of the note type, unique across all note types, compared case-sensitively.", 100),
			"description": requiredText("The description of the note type.", 200),
			"is_single":   flag("Whether an object can have only one note of the type.", false),
			"is_active":   flag("Whether the type is offered for new notes.", true),
			"is_mandatory": flag("Whether, while the type is active, the UI keeps users from closing a finding, or setting it inactive or false positive, "+
				"until the finding has a note of the type. The API doesn't check it.", true),
			"force_destroy": schema.BoolAttribute{
				MarkdownDescription: "Whether to destroy the note type even when it has notes, which DefectDojo then deletes too. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *NoteTypeResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("note type")
}

func (r *NoteTypeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *NoteTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NoteTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, _, err := r.client.NoteTypes.Create(ctx, &defectdojo.NoteType{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		IsSingle:    data.IsSingle.ValueBool(),
		IsActive:    data.IsActive.ValueBool(),
		IsMandatory: data.IsMandatory.ValueBool(),
	}, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo note type",
			"Could not create note type "+strconv.Quote(data.Name.ValueString())+", unexpected error: "+err.Error(),
		)
		return
	}
	updateNoteTypeModel(&data, created)

	tflog.Trace(ctx, "created a note type", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *NoteTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NoteTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nt, _, err := r.client.NoteTypes.Get(ctx, int(data.ID.ValueInt64()), nil)
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			removeGone(ctx, resp, data.ID)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo note type",
			fmt.Sprintf("Could not read note type %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateNoteTypeModel(&data, nt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *NoteTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NoteTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, _, err := r.client.NoteTypes.Patch(ctx, int(data.ID.ValueInt64()), &defectdojo.NoteTypePatch{
		Name:        new(data.Name.ValueString()),
		Description: new(data.Description.ValueString()),
		IsSingle:    new(data.IsSingle.ValueBool()),
		IsActive:    new(data.IsActive.ValueBool()),
		IsMandatory: new(data.IsMandatory.ValueBool()),
	}, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo note type",
			fmt.Sprintf("Could not update note type %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateNoteTypeModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *NoteTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NoteTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())

	if !data.ForceDestroy.ValueBool() {
		n, _, err := r.client.NoteTypes.CountNotes(ctx, id, nil)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return
		}
		if err != nil {
			resp.Diagnostics.AddError(
				"Error deleting DefectDojo note type",
				fmt.Sprintf("Could not count the notes of note type %d, unexpected error: %s", id, err),
			)
			return
		}
		if n > 0 {
			resp.Diagnostics.AddError(
				"DefectDojo note type has notes",
				fmt.Sprintf("Note type %d (%q) has %d notes. Deleting it would delete them too. "+
					"Set force_destroy = true and apply before destroying, or deactivate the note type with is_active = false instead.",
					id, data.Name.ValueString(), n),
			)
			return
		}
	}

	if _, err := r.client.NoteTypes.Delete(ctx, id, nil); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo note type",
			fmt.Sprintf("Could not delete note type %d, unexpected error: %s", id, err),
		)
	}
}

func (r *NoteTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "note type")
}

// updateNoteTypeModel copies the note type read from the server into data.
func updateNoteTypeModel(data *NoteTypeResourceModel, nt *defectdojo.NoteType) {
	data.ID = types.Int64Value(int64(nt.ID))
	data.Name = types.StringValue(nt.Name)
	data.Description = types.StringValue(nt.Description)
	data.IsSingle = types.BoolValue(nt.IsSingle)
	data.IsActive = types.BoolValue(nt.IsActive)
	data.IsMandatory = types.BoolValue(nt.IsMandatory)
	// Imported resources get the default.
	if data.ForceDestroy.IsNull() || data.ForceDestroy.IsUnknown() {
		data.ForceDestroy = types.BoolValue(false)
	}
}
