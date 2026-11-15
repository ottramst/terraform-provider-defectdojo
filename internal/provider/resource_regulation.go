package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
var _ resource.Resource = &RegulationResource{}
var _ resource.ResourceWithConfigure = &RegulationResource{}
var _ resource.ResourceWithImportState = &RegulationResource{}
var _ resource.ResourceWithIdentity = &RegulationResource{}

// regulationCategories are the categories DefectDojo accepts.
var regulationCategories = []string{
	defectdojo.RegulationCategoryPrivacy,
	defectdojo.RegulationCategoryFinance,
	defectdojo.RegulationCategoryEducation,
	defectdojo.RegulationCategoryMedical,
	defectdojo.RegulationCategoryCorporate,
	defectdojo.RegulationCategorySecurity,
	defectdojo.RegulationCategoryGovernment,
	defectdojo.RegulationCategoryOther,
}

func NewRegulationResource() resource.Resource {
	return &RegulationResource{}
}

// RegulationResource defines the resource implementation.
type RegulationResource struct {
	client *defectdojo.Client
}

// RegulationModel describes the resource and data source data model.
type RegulationModel struct {
	ID           types.Int64  `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Acronym      types.String `tfsdk:"acronym"`
	Category     types.String `tfsdk:"category"`
	Jurisdiction types.String `tfsdk:"jurisdiction"`
	Description  types.String `tfsdk:"description"`
	Reference    types.String `tfsdk:"reference"`
}

func (r *RegulationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_regulation"
}

func (r *RegulationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DefectDojo regulation, such as PCI DSS or HIPAA, which products list in `regulation_ids`. " +
			"DefectDojo comes with 26 regulations; look them up with the `defectdojo_regulation` data source.\n\n" +
			"~> **Destroying a regulation removes it from every product** that lists it. " +
			"The `regulation_ids` of a `defectdojo_product` that listed it then show a change.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the regulation.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name":    requiredText("The name of the regulation, unique across all regulations, compared case-sensitively.", 128),
			"acronym": requiredText("The acronym of the regulation, such as `GDPR`, unique across all regulations, compared case-sensitively.", 20),
			"category": schema.StringAttribute{
				MarkdownDescription: "The subject of the regulation: one of `privacy`, `finance`, `education`, `medical`, `corporate`, `security`, `government` and `other`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(regulationCategories...)},
			},
			"jurisdiction": requiredText("The territory the regulation applies to, such as `European Union`.", 64),
			"description": schema.StringAttribute{
				MarkdownDescription: "The purpose of the regulation, without leading or trailing whitespace, which DefectDojo removes. Trim heredocs with `chomp()` or `trimspace()`.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					noSurroundingWhitespace(),
				},
			},
			"reference": schema.StringAttribute{
				MarkdownDescription: "An http, https, ftp or ftps URL with more information, at most 200 characters.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 200),
					stringvalidator.RegexMatches(urlPattern, "must be an http, https, ftp or ftps URL"),
				},
			},
		},
	}
}

func (r *RegulationResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("regulation")
}

func (r *RegulationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *RegulationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RegulationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, _, err := r.client.Regulations.Create(ctx, &defectdojo.Regulation{
		Name:         data.Name.ValueString(),
		Acronym:      data.Acronym.ValueString(),
		Category:     data.Category.ValueString(),
		Jurisdiction: data.Jurisdiction.ValueString(),
		Description:  data.Description.ValueString(),
		Reference:    data.Reference.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo regulation",
			"Could not create regulation "+strconv.Quote(data.Name.ValueString())+", unexpected error: "+err.Error(),
		)
		return
	}
	updateRegulationModel(&data, created)

	tflog.Trace(ctx, "created a regulation", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *RegulationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RegulationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	reg, _, err := r.client.Regulations.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo regulation",
			fmt.Sprintf("Could not read regulation %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateRegulationModel(&data, reg)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *RegulationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RegulationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Description and reference aren't nullable: "" clears them.
	updated, _, err := r.client.Regulations.Patch(ctx, int(data.ID.ValueInt64()), &defectdojo.RegulationPatch{
		Name:         new(data.Name.ValueString()),
		Acronym:      new(data.Acronym.ValueString()),
		Category:     new(data.Category.ValueString()),
		Jurisdiction: new(data.Jurisdiction.ValueString()),
		Description:  new(data.Description.ValueString()),
		Reference:    new(data.Reference.ValueString()),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo regulation",
			fmt.Sprintf("Could not update regulation %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateRegulationModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *RegulationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RegulationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.Regulations.Delete(ctx, int(data.ID.ValueInt64())); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo regulation",
			fmt.Sprintf("Could not delete regulation %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
	}
}

func (r *RegulationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "regulation")
}

// updateRegulationModel copies the regulation read from the server into data.
func updateRegulationModel(data *RegulationModel, reg *defectdojo.Regulation) {
	data.ID = types.Int64Value(int64(reg.ID))
	data.Name = types.StringValue(reg.Name)
	data.Acronym = types.StringValue(reg.Acronym)
	data.Category = types.StringValue(reg.Category)
	data.Jurisdiction = types.StringValue(reg.Jurisdiction)
	data.Description = stringOrNull(reg.Description)
	data.Reference = stringOrNull(reg.Reference)
}
