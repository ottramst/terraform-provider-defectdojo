package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ProductTypeResource{}
var _ resource.ResourceWithConfigure = &ProductTypeResource{}
var _ resource.ResourceWithImportState = &ProductTypeResource{}
var _ resource.ResourceWithIdentity = &ProductTypeResource{}

// productTypeDeleteTimeout bounds how long Delete waits for an asynchronous
// delete to finish.
const productTypeDeleteTimeout = 5 * time.Minute

func NewProductTypeResource() resource.Resource {
	return &ProductTypeResource{}
}

// ProductTypeResource defines the resource implementation.
type ProductTypeResource struct {
	client *defectdojo.Client
}

// ProductTypeResourceModel describes the resource data model.
type ProductTypeResourceModel struct {
	ID              types.Int64  `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	CriticalProduct types.Bool   `tfsdk:"critical_product"`
	KeyProduct      types.Bool   `tfsdk:"key_product"`
	AuthorizedUsers types.Set    `tfsdk:"authorized_users"`
	ForceDestroy    types.Bool   `tfsdk:"force_destroy"`
}

func (r *ProductTypeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product_type"
}

func (r *ProductTypeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DefectDojo product type. Product types group products; DefectDojo 3.x shows them as *organizations*.\n\n" +
			"~> **Deleting a product type deletes its products**, with their engagements, tests and findings. " +
			"The provider refuses to destroy a product type that has products unless `force_destroy` is set.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the product type.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the product type, without leading or trailing whitespace, which DefectDojo removes. Names are unique, compared case-sensitively.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 255),
					noSurroundingWhitespace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the product type, at most 4000 characters, without leading or trailing whitespace, which DefectDojo removes. Trim heredocs with `chomp()` or `trimspace()`.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 4000),
					noSurroundingWhitespace(),
				},
			},
			"critical_product": schema.BoolAttribute{
				MarkdownDescription: "Whether the products of this type are critical. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"key_product": schema.BoolAttribute{
				MarkdownDescription: "Whether the products of this type are key products. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"authorized_users": schema.SetAttribute{
				MarkdownDescription: "The IDs of the users who may access the product type and its products, in addition to superusers and staff. " +
					"When it is not set, the provider leaves the authorized users as they are, so they can be managed elsewhere; an empty set removes them all. " +
					"If they change elsewhere between the plan and the apply of another change to the product type, the apply fails with an inconsistent result; plan and apply again.",
				ElementType: types.Int64Type,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"force_destroy": schema.BoolAttribute{
				MarkdownDescription: "Whether to destroy the product type even when it has products, which DefectDojo then deletes too, " +
					"with their engagements, tests and findings. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
		},
	}
}

func (r *ProductTypeResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("product type")
}

func (r *ProductTypeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *ProductTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ProductTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pt := &defectdojo.ProductType{
		Name:            data.Name.ValueString(),
		Description:     data.Description.ValueString(),
		CriticalProduct: data.CriticalProduct.ValueBool(),
		KeyProduct:      data.KeyProduct.ValueBool(),
	}
	if !data.AuthorizedUsers.IsNull() && !data.AuthorizedUsers.IsUnknown() {
		pt.AuthorizedUsers = intsFromSet(ctx, data.AuthorizedUsers, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	created, _, err := r.client.ProductTypes.Create(ctx, pt)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo product type",
			"Could not create product type "+strconv.Quote(pt.Name)+", unexpected error: "+err.Error(),
		)
		return
	}
	updateProductTypeModel(&data, created)

	tflog.Trace(ctx, "created a product type", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *ProductTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ProductTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pt, _, err := r.client.ProductTypes.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo product type",
			fmt.Sprintf("Could not read product type %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateProductTypeModel(&data, pt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *ProductTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ProductTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The plan holds the prior authorized users when they are not configured
	// (UseStateForUnknown), so check the configuration.
	var configuredUsers types.Set
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("authorized_users"), &configuredUsers)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A partial update, so that unmanaged authorized users are kept.
	patch := &defectdojo.ProductTypePatch{
		Name:            new(data.Name.ValueString()),
		Description:     defectdojo.Value(data.Description.ValueString()),
		CriticalProduct: new(data.CriticalProduct.ValueBool()),
		KeyProduct:      new(data.KeyProduct.ValueBool()),
	}
	if data.Description.IsNull() {
		patch.Description = defectdojo.Null[string]()
	}
	if !configuredUsers.IsNull() && !configuredUsers.IsUnknown() {
		patch.AuthorizedUsers = new(intsFromSet(ctx, configuredUsers, &resp.Diagnostics))
		if resp.Diagnostics.HasError() {
			return
		}
	}

	updated, _, err := r.client.ProductTypes.Patch(ctx, int(data.ID.ValueInt64()), patch)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo product type",
			fmt.Sprintf("Could not update product type %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateProductTypeModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *ProductTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ProductTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())

	if !data.ForceDestroy.ValueBool() {
		n, _, err := r.client.ProductTypes.CountProducts(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error deleting DefectDojo product type",
				fmt.Sprintf("Could not count the products of product type %d, unexpected error: %s", id, err),
			)
			return
		}
		if n > 0 {
			resp.Diagnostics.AddError(
				"DefectDojo product type has products",
				fmt.Sprintf("Product type %d (%q) has %d products. Deleting it would delete them too, with their engagements, tests and findings. "+
					"Delete or move the products first, or set force_destroy = true and apply before destroying.",
					id, data.Name.ValueString(), n),
			)
			return
		}
	}

	if _, err := r.client.ProductTypes.Delete(ctx, id); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo product type",
			fmt.Sprintf("Could not delete product type %d, unexpected error: %s", id, err),
		)
		return
	}

	waitCtx, cancel := context.WithTimeout(ctx, productTypeDeleteTimeout)
	defer cancel()
	err := waitForDeletion(waitCtx, func(ctx context.Context) error {
		_, _, err := r.client.ProductTypes.Get(ctx, id)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo product type",
			fmt.Sprintf("Product type %d was not deleted: %s", id, err),
		)
	}
}

func (r *ProductTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "product type")
}

// updateProductTypeModel copies the product type read from the server into
// data.
func updateProductTypeModel(data *ProductTypeResourceModel, pt *defectdojo.ProductType) {
	data.ID = types.Int64Value(int64(pt.ID))
	data.Name = types.StringValue(pt.Name)
	data.Description = stringOrNull(pt.Description)
	data.CriticalProduct = types.BoolValue(pt.CriticalProduct)
	data.KeyProduct = types.BoolValue(pt.KeyProduct)
	data.AuthorizedUsers = int64Set(pt.AuthorizedUsers)
	// Imported resources get the default.
	if data.ForceDestroy.IsNull() || data.ForceDestroy.IsUnknown() {
		data.ForceDestroy = types.BoolValue(false)
	}
}

// IDIdentityModel is the resource identity of an object with a numeric ID.
type IDIdentityModel struct {
	ID types.Int64 `tfsdk:"id"`
}

// idIdentitySchema returns the identity schema of an object with a numeric ID.
func idIdentitySchema(object string) identityschema.Schema {
	return identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.Int64Attribute{
				Description:       "ID of the " + object,
				RequiredForImport: true,
			},
		},
	}
}

// setIDIdentity sets the identity of an object with a numeric ID. The
// framework only passes a nil identity to resources without an identity
// schema.
func setIDIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, id types.Int64) diag.Diagnostics {
	if identity == nil {
		return nil
	}
	return identity.Set(ctx, IDIdentityModel{ID: id})
}

// importByID reads the numeric ID from the import ID, or from the identity
// when the resource is imported with an identity, and sets it in the state and
// the identity.
func importByID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, object string) {
	var id int64
	if req.ID != "" {
		var ok bool
		if id, ok = parseImportID(req.ID, object, &resp.Diagnostics); !ok {
			return
		}
	} else {
		var identity IDIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		id = identity.ID.ValueInt64()
		if id <= 0 {
			resp.Diagnostics.AddError(
				"Invalid import identity",
				fmt.Sprintf("Expected the positive numeric ID of the %s, got: %d", object, id),
			)
			return
		}
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, types.Int64Value(id))...)
}
