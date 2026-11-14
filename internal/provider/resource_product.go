package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ProductResource{}
var _ resource.ResourceWithConfigure = &ProductResource{}
var _ resource.ResourceWithImportState = &ProductResource{}
var _ resource.ResourceWithIdentity = &ProductResource{}

// productDeleteTimeout bounds how long Delete waits for an asynchronous delete
// to finish.
const productDeleteTimeout = 5 * time.Minute

// businessCriticalities are the values of a product's business_criticality.
var businessCriticalities = []string{
	defectdojo.BusinessCriticalityVeryHigh,
	defectdojo.BusinessCriticalityHigh,
	defectdojo.BusinessCriticalityMedium,
	defectdojo.BusinessCriticalityLow,
	defectdojo.BusinessCriticalityVeryLow,
	defectdojo.BusinessCriticalityNone,
}

func NewProductResource() resource.Resource {
	return &ProductResource{}
}

// ProductResource defines the resource implementation.
type ProductResource struct {
	client *defectdojo.Client
}

// ProductResourceModel describes the resource data model.
type ProductResourceModel struct {
	ID                            types.Int64  `tfsdk:"id"`
	Name                          types.String `tfsdk:"name"`
	Description                   types.String `tfsdk:"description"`
	ProductTypeID                 types.Int64  `tfsdk:"product_type_id"`
	BusinessCriticality           types.String `tfsdk:"business_criticality"`
	Platform                      types.String `tfsdk:"platform"`
	Lifecycle                     types.String `tfsdk:"life_cycle"`
	Origin                        types.String `tfsdk:"origin"`
	ExternalAudience              types.Bool   `tfsdk:"external_audience"`
	InternetAccessible            types.Bool   `tfsdk:"internet_accessible"`
	EnableProductTagInheritance   types.Bool   `tfsdk:"enable_product_tag_inheritance"`
	EnableSimpleRiskAcceptance    types.Bool   `tfsdk:"enable_simple_risk_acceptance"`
	EnableFullRiskAcceptance      types.Bool   `tfsdk:"enable_full_risk_acceptance"`
	DisableSLABreachNotifications types.Bool   `tfsdk:"disable_sla_breach_notifications"`
	SLAConfigurationID            types.Int64  `tfsdk:"sla_configuration_id"`
	ProductManagerID              types.Int64  `tfsdk:"product_manager_id"`
	TechnicalContactID            types.Int64  `tfsdk:"technical_contact_id"`
	TeamManagerID                 types.Int64  `tfsdk:"team_manager_id"`
	AuthorizedUsers               types.Set    `tfsdk:"authorized_users"`
	RegulationIDs                 types.Set    `tfsdk:"regulation_ids"`
	Tags                          types.Set    `tfsdk:"tags"`
	ForceDestroy                  types.Bool   `tfsdk:"force_destroy"`
}

func (r *ProductResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

// managedElsewhere is the part of the description of Optional+Computed
// attributes that are only sent when they are configured.
func managedElsewhere(what string) string {
	return "When it is not set, the provider leaves the " + what + " as they are, so they can be managed elsewhere. " + changedBetweenPlanAndApply
}

// changedBetweenPlanAndApply explains why an apply fails when an attribute
// that is only sent when configured changes elsewhere in the meantime.
const changedBetweenPlanAndApply = "If it changes elsewhere between the plan and the apply of another change to the product, " +
	"the apply fails with an inconsistent result; plan and apply again."

func (r *ProductResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	userID := func(role string) schema.Int64Attribute {
		return schema.Int64Attribute{
			MarkdownDescription: "The ID of the product's " + role + ", an active user.",
			Optional:            true,
			Validators:          []validator.Int64{int64validator.AtLeast(1)},
		}
	}
	lookup := func(what, values string) schema.StringAttribute {
		return schema.StringAttribute{
			MarkdownDescription: "The " + what + " of the product: the value of an entry of DefectDojo's product " + what + "s, such as " + values + ".",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
		}
	}
	flag := func(description string, value bool) schema.BoolAttribute {
		return schema.BoolAttribute{
			MarkdownDescription: description + " Defaults to `" + strconv.FormatBool(value) + "`.",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(value),
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DefectDojo product. DefectDojo 3.x shows products as *assets*.\n\n" +
			"~> **Deleting a product deletes its engagements**, with their tests and findings. " +
			"The provider refuses to destroy a product that has engagements unless `force_destroy` is set.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the product.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the product, at most 255 characters, without leading or trailing whitespace, which DefectDojo removes. " +
					"Names are unique across all products, compared case-sensitively.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 255),
					noSurroundingWhitespace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the product, at most 4000 characters, without leading or trailing whitespace, which DefectDojo removes. " +
					"Trim heredocs with `chomp()` or `trimspace()`.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 4000),
					noSurroundingWhitespace(),
				},
			},
			"product_type_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the product type of the product. Changing it moves the product to another product type.",
				Required:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"business_criticality": schema.StringAttribute{
				MarkdownDescription: "The business criticality of the product: one of `very high`, `high`, `medium`, `low`, `very low` and `none`.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.OneOf(businessCriticalities...)},
			},
			"platform":            lookup("platform", "`web`, `web service`, `desktop`, `mobile` or `iot`"),
			"life_cycle":          lookup("lifecycle", "`construction`, `production` or `retirement`"),
			"origin":              lookup("origin", "`internal`, `third party library`, `purchased`, `contractor`, `open source` or `outsourced`"),
			"external_audience":   flag("Whether the product is used by people outside the organization.", false),
			"internet_accessible": flag("Whether the product is accessible from the internet.", false),
			"enable_product_tag_inheritance": flag("Whether the product's engagements, tests and findings inherit its tags. "+
				"DefectDojo also has a system-wide setting for this.", false),
			"enable_simple_risk_acceptance":    flag("Whether findings can be risk-accepted with a checkbox.", false),
			"enable_full_risk_acceptance":      flag("Whether findings can be risk-accepted with a risk acceptance form.", true),
			"disable_sla_breach_notifications": flag("Whether to disable the notifications of SLA breaches of the product's findings.", false),
			"sla_configuration_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the SLA configuration of the product, such as a `defectdojo_sla_configuration`. DefectDojo uses the Default SLA configuration (1) when it is not set. " +
					"Changing it makes DefectDojo recalculate the SLA dates of the product's findings in the background; until that finishes, " +
					"neither it nor the days of the new SLA configuration can be changed again. " +
					"When it is not set, the provider leaves it as it is, so it can be managed elsewhere. " + changedBetweenPlanAndApply,
				Optional:   true,
				Computed:   true,
				Validators: []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"product_manager_id":   userID("product manager"),
			"technical_contact_id": userID("technical contact"),
			"team_manager_id":      userID("team manager"),
			"authorized_users": schema.SetAttribute{
				MarkdownDescription: "The IDs of the users who may access the product, in addition to superusers, staff and the authorized users of its product type. " +
					managedElsewhere("authorized users") + " An empty set removes them all.",
				ElementType: types.Int64Type,
				Optional:    true,
				Computed:    true,
				Validators:  []validator.Set{setvalidator.ValueInt64sAre(int64validator.AtLeast(1))},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"regulation_ids": schema.SetAttribute{
				MarkdownDescription: "The IDs of the regulations that apply to the product. " + managedElsewhere("regulations") + " An empty set removes them all.",
				ElementType:         types.Int64Type,
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Set{setvalidator.ValueInt64sAre(int64validator.AtLeast(1))},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"tags": schema.SetAttribute{
				MarkdownDescription: "The tags of the product, each at most 255 characters, without spaces, commas or quotes. " +
					"Tags are compared ignoring case: DefectDojo gives a tag the spelling of an existing product tag that matches it ignoring case, and the provider keeps the configured spelling " +
					"(after an import, the first apply sets the configured spelling once). " +
					managedElsewhere("tags") + " An empty set removes them all.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Validators:  tagValidators(),
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"force_destroy": schema.BoolAttribute{
				MarkdownDescription: "Whether to destroy the product even when it has engagements, which DefectDojo then deletes too, " +
					"with their tests and findings. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
		},
	}
}

func (r *ProductResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("product")
}

func (r *ProductResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *ProductResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ProductResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unknown values (Optional+Computed attributes that are not configured)
	// are zero: DefectDojo's defaults.
	p := &defectdojo.Product{
		Name:                          data.Name.ValueString(),
		Description:                   data.Description.ValueString(),
		ProdType:                      int(data.ProductTypeID.ValueInt64()),
		BusinessCriticality:           data.BusinessCriticality.ValueString(),
		Platform:                      data.Platform.ValueString(),
		Lifecycle:                     data.Lifecycle.ValueString(),
		Origin:                        data.Origin.ValueString(),
		ExternalAudience:              data.ExternalAudience.ValueBool(),
		InternetAccessible:            data.InternetAccessible.ValueBool(),
		EnableProductTagInheritance:   data.EnableProductTagInheritance.ValueBool(),
		EnableSimpleRiskAcceptance:    data.EnableSimpleRiskAcceptance.ValueBool(),
		EnableFullRiskAcceptance:      data.EnableFullRiskAcceptance.ValueBool(),
		DisableSLABreachNotifications: data.DisableSLABreachNotifications.ValueBool(),
		SLAConfiguration:              int(data.SLAConfigurationID.ValueInt64()),
		ProductManager:                int(data.ProductManagerID.ValueInt64()),
		TechnicalContact:              int(data.TechnicalContactID.ValueInt64()),
		TeamManager:                   int(data.TeamManagerID.ValueInt64()),
	}
	if known(data.AuthorizedUsers) {
		p.AuthorizedUsers = intsFromSet(ctx, data.AuthorizedUsers, &resp.Diagnostics)
	}
	if known(data.RegulationIDs) {
		p.Regulations = intsFromSet(ctx, data.RegulationIDs, &resp.Diagnostics)
	}
	if known(data.Tags) {
		p.Tags = stringsFromSet(ctx, data.Tags, &resp.Diagnostics)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	created, _, err := r.client.Products.Create(ctx, p)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo product",
			"Could not create product "+strconv.Quote(p.Name)+", unexpected error: "+err.Error(),
		)
		return
	}
	updateProductModel(&data, created, data.Tags)

	tflog.Trace(ctx, "created a product", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *ProductResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ProductResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, _, err := r.client.Products.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo product",
			fmt.Sprintf("Could not read product %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateProductModel(&data, p, data.Tags)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *ProductResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ProductResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The plan holds the prior values of these attributes when they are not
	// configured (UseStateForUnknown), so check the configuration.
	var sla types.Int64
	var users, regulations, tags types.Set
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("sla_configuration_id"), &sla)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("authorized_users"), &users)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("regulation_ids"), &regulations)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("tags"), &tags)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A partial update, so that unmanaged attributes are kept.
	patch := &defectdojo.ProductPatch{
		Name:                          new(data.Name.ValueString()),
		Description:                   new(data.Description.ValueString()),
		ProdType:                      new(int(data.ProductTypeID.ValueInt64())),
		BusinessCriticality:           nullableString(data.BusinessCriticality),
		Platform:                      nullableString(data.Platform),
		Lifecycle:                     nullableString(data.Lifecycle),
		Origin:                        nullableString(data.Origin),
		ExternalAudience:              new(data.ExternalAudience.ValueBool()),
		InternetAccessible:            new(data.InternetAccessible.ValueBool()),
		EnableProductTagInheritance:   new(data.EnableProductTagInheritance.ValueBool()),
		EnableSimpleRiskAcceptance:    new(data.EnableSimpleRiskAcceptance.ValueBool()),
		EnableFullRiskAcceptance:      new(data.EnableFullRiskAcceptance.ValueBool()),
		DisableSLABreachNotifications: new(data.DisableSLABreachNotifications.ValueBool()),
		ProductManager:                nullableInt(data.ProductManagerID),
		TechnicalContact:              nullableInt(data.TechnicalContactID),
		TeamManager:                   nullableInt(data.TeamManagerID),
	}
	if known(sla) {
		patch.SLAConfiguration = new(int(sla.ValueInt64()))
	}
	if known(users) {
		patch.AuthorizedUsers = new(intsFromSet(ctx, users, &resp.Diagnostics))
	}
	if known(regulations) {
		patch.Regulations = new(intsFromSet(ctx, regulations, &resp.Diagnostics))
	}
	if known(tags) {
		patch.Tags = new(stringsFromSet(ctx, tags, &resp.Diagnostics))
	}
	if resp.Diagnostics.HasError() {
		return
	}

	updated, _, err := r.client.Products.Patch(ctx, int(data.ID.ValueInt64()), patch)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo product",
			fmt.Sprintf("Could not update product %d, unexpected error: %s", data.ID.ValueInt64(), err)+slaRecalculationHint(err),
		)
		return
	}
	updateProductModel(&data, updated, data.Tags)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

// slaRecalculationHint explains the error DefectDojo returns for an SLA
// change made while it still recalculates the SLA dates of an earlier one:
// a change of a product's SLA configuration, or of an SLA configuration's
// days.
func slaRecalculationHint(err error) string {
	if !defectdojo.IsSLARecalculating(err) {
		return ""
	}
	return "\n\nDefectDojo is still recalculating the SLA expiration dates of findings after an earlier SLA change, " +
		"which its Celery workers do in the background. Apply again once they have finished. " +
		"If the system setting enable_finding_sla is off, DefectDojo never finishes."
}

func (r *ProductResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ProductResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())

	if !data.ForceDestroy.ValueBool() {
		n, _, err := r.client.Products.CountEngagements(ctx, id)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return // Deleted outside Terraform
		}
		if err != nil {
			resp.Diagnostics.AddError(
				"Error deleting DefectDojo product",
				fmt.Sprintf("Could not count the engagements of product %d, unexpected error: %s", id, err),
			)
			return
		}
		if n > 0 {
			resp.Diagnostics.AddError(
				"DefectDojo product has engagements",
				fmt.Sprintf("Product %d (%q) has %d engagements. Deleting it would delete them too, with their tests and findings. "+
					"Delete or move the engagements first, or set force_destroy = true and apply before destroying.",
					id, data.Name.ValueString(), n),
			)
			return
		}
	}

	if _, err := r.client.Products.Delete(ctx, id); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo product",
			fmt.Sprintf("Could not delete product %d, unexpected error: %s", id, err),
		)
		return
	}

	waitCtx, cancel := context.WithTimeout(ctx, productDeleteTimeout)
	defer cancel()
	err := waitForDeletion(waitCtx, func(ctx context.Context) error {
		_, _, err := r.client.Products.Get(ctx, id)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo product",
			fmt.Sprintf("Product %d was not deleted: %s", id, err),
		)
	}
}

func (r *ProductResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "product")
}

// updateProductModel copies the product read from the server into data.
// Tags that match one of tagReference ignoring case keep its spelling.
func updateProductModel(data *ProductResourceModel, p *defectdojo.Product, tagReference types.Set) {
	data.ID = types.Int64Value(int64(p.ID))
	data.Name = types.StringValue(p.Name)
	data.Description = types.StringValue(p.Description)
	data.ProductTypeID = types.Int64Value(int64(p.ProdType))
	data.BusinessCriticality = stringOrNull(p.BusinessCriticality)
	data.Platform = stringOrNull(p.Platform)
	data.Lifecycle = stringOrNull(p.Lifecycle)
	data.Origin = stringOrNull(p.Origin)
	data.ExternalAudience = types.BoolValue(p.ExternalAudience)
	data.InternetAccessible = types.BoolValue(p.InternetAccessible)
	data.EnableProductTagInheritance = types.BoolValue(p.EnableProductTagInheritance)
	data.EnableSimpleRiskAcceptance = types.BoolValue(p.EnableSimpleRiskAcceptance)
	data.EnableFullRiskAcceptance = types.BoolValue(p.EnableFullRiskAcceptance)
	data.DisableSLABreachNotifications = types.BoolValue(p.DisableSLABreachNotifications)
	data.SLAConfigurationID = types.Int64Value(int64(p.SLAConfiguration))
	data.ProductManagerID = int64OrNull(p.ProductManager)
	data.TechnicalContactID = int64OrNull(p.TechnicalContact)
	data.TeamManagerID = int64OrNull(p.TeamManager)
	data.AuthorizedUsers = int64Set(p.AuthorizedUsers)
	data.RegulationIDs = int64Set(p.Regulations)
	data.Tags = tagSet(p.Tags, tagReference)
	// Imported resources get the default.
	if data.ForceDestroy.IsNull() || data.ForceDestroy.IsUnknown() {
		data.ForceDestroy = types.BoolValue(false)
	}
}
