package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ProductAttributeResource{}
var _ resource.ResourceWithConfigure = &ProductAttributeResource{}
var _ resource.ResourceWithImportState = &ProductAttributeResource{}
var _ resource.ResourceWithIdentity = &ProductAttributeResource{}

// productAttributeKind describes one of DefectDojo's three lookup tables of
// product attributes, which share one resource implementation.
type productAttributeKind struct {
	// typeName is the resource type's suffix, such as "product_platform".
	typeName string

	// object names an entry, such as "product platform".
	object string

	// productAttribute is the defectdojo_product attribute that takes the
	// entry's value, such as "platform".
	productAttribute string

	// seeded lists the values DefectDojo comes with.
	seeded string

	// service returns the client's service for the table.
	service func(*defectdojo.Client) *defectdojo.ProductAttributesService
}

var (
	productPlatformKind = productAttributeKind{
		typeName: "product_platform", object: "product platform", productAttribute: "platform",
		seeded:  "`web service`, `desktop`, `iot`, `mobile` and `web`",
		service: func(c *defectdojo.Client) *defectdojo.ProductAttributesService { return c.ProductPlatforms },
	}
	productLifecycleKind = productAttributeKind{
		typeName: "product_lifecycle", object: "product lifecycle", productAttribute: "life_cycle",
		seeded:  "`construction`, `production` and `retirement`",
		service: func(c *defectdojo.Client) *defectdojo.ProductAttributesService { return c.ProductLifecycles },
	}
	productOriginKind = productAttributeKind{
		typeName: "product_origin", object: "product origin", productAttribute: "origin",
		seeded:  "`third party library`, `purchased`, `contractor`, `internal`, `open source` and `outsourced`",
		service: func(c *defectdojo.Client) *defectdojo.ProductAttributesService { return c.ProductOrigins },
	}
)

func NewProductPlatformResource() resource.Resource {
	return &ProductAttributeResource{kind: productPlatformKind}
}

func NewProductLifecycleResource() resource.Resource {
	return &ProductAttributeResource{kind: productLifecycleKind}
}

func NewProductOriginResource() resource.Resource {
	return &ProductAttributeResource{kind: productOriginKind}
}

// ProductAttributeResource defines the implementation of the product
// platform, lifecycle and origin resources.
type ProductAttributeResource struct {
	kind    productAttributeKind
	service *defectdojo.ProductAttributesService
}

// ProductAttributeResourceModel describes the resource data model.
type ProductAttributeResourceModel struct {
	ID           types.Int64  `tfsdk:"id"`
	Value        types.String `tfsdk:"value"`
	Name         types.String `tfsdk:"name"`
	Icon         types.String `tfsdk:"icon"`
	DisplayOrder types.Int64  `tfsdk:"display_order"`
}

func (r *ProductAttributeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.kind.typeName
}

func (r *ProductAttributeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	k := r.kind
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("Manages an entry of DefectDojo's %ss, the values of a product's `%s`. "+
			"DefectDojo comes with %s. Managing them needs a superuser or a staff user: they have no configuration permissions that `defectdojo_user` could grant.\n\n"+
			"-> DefectDojo can't change `value`, so changing it replaces the entry, and DefectDojo refuses to delete an entry that products use. "+
			"Set `lifecycle { create_before_destroy = true }` so that the new entry is created, and the products move to it, before the old one is deleted. "+
			"With `create_before_destroy`, `terraform apply -replace` fails, since the new entry would have the same value as the old one.",
			k.object, k.productAttribute, k.seeded),
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the " + k.object + ".",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf("The value that products use in `%s`, at most 50 characters, unique among the %ss, "+
					"without leading or trailing whitespace, which DefectDojo removes. "+
					"DefectDojo can't change it, so changing it replaces the entry; see the note on `create_before_destroy`.", k.productAttribute, k.object),
				Required: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 50),
					noSurroundingWhitespace(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": requiredText("The label DefectDojo's UI shows.", 200),
			"icon": schema.StringAttribute{
				MarkdownDescription: "A Font Awesome icon class for the classic UI, such as `desktop`, at most 100 characters, without leading or trailing whitespace, which DefectDojo removes.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 100),
					noSurroundingWhitespace(),
				},
			},
			"display_order": schema.Int64Attribute{
				MarkdownDescription: "The position in the UI's lists, lower first, a 32-bit integer. Defaults to `0`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Validators:          []validator.Int64{int32Range()},
			},
		},
	}
}

func (r *ProductAttributeResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema(r.kind.object)
}

func (r *ProductAttributeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.service = r.kind.service(client)
	}
}

func (r *ProductAttributeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ProductAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, _, err := r.service.Create(ctx, &defectdojo.ProductAttribute{
		Value:        data.Value.ValueString(),
		Name:         data.Name.ValueString(),
		Icon:         data.Icon.ValueString(),
		DisplayOrder: int(data.DisplayOrder.ValueInt64()),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo "+r.kind.object,
			"Could not create "+r.kind.object+" "+strconv.Quote(data.Value.ValueString())+", unexpected error: "+err.Error(),
		)
		return
	}
	updateProductAttributeModel(&data, created)

	tflog.Trace(ctx, "created a "+r.kind.object, map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *ProductAttributeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ProductAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, _, err := r.service.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			removeGone(ctx, resp, data.ID)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo "+r.kind.object,
			fmt.Sprintf("Could not read %s %d, unexpected error: %s", r.kind.object, data.ID.ValueInt64(), err),
		)
		return
	}
	updateProductAttributeModel(&data, a)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *ProductAttributeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ProductAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The value can't change: a change replaces the entry.
	updated, _, err := r.service.Patch(ctx, int(data.ID.ValueInt64()), &defectdojo.ProductAttributePatch{
		Name:         new(data.Name.ValueString()),
		Icon:         new(data.Icon.ValueString()),
		DisplayOrder: new(int(data.DisplayOrder.ValueInt64())),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo "+r.kind.object,
			fmt.Sprintf("Could not update %s %d, unexpected error: %s", r.kind.object, data.ID.ValueInt64(), err),
		)
		return
	}
	updateProductAttributeModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *ProductAttributeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ProductAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())

	_, err := r.service.Delete(ctx, id)
	switch {
	case err == nil, errors.Is(err, defectdojo.ErrNotFound):
	case errors.Is(err, defectdojo.ErrConflict):
		resp.Diagnostics.AddError(
			"DefectDojo "+r.kind.object+" is in use",
			fmt.Sprintf("The %s %q (ID %d) can't be deleted while products use it in %s. "+
				"Move the products to another value first. For a change of value, set lifecycle { create_before_destroy = true } on the resource, "+
				"so that the new entry exists before the products move to it. DefectDojo: %s",
				r.kind.object, data.Value.ValueString(), id, r.kind.productAttribute, err),
		)
	default:
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo "+r.kind.object,
			fmt.Sprintf("Could not delete %s %d, unexpected error: %s", r.kind.object, id, err),
		)
	}
}

func (r *ProductAttributeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, r.kind.object)
}

// updateProductAttributeModel copies the entry read from the server into data.
func updateProductAttributeModel(data *ProductAttributeResourceModel, a *defectdojo.ProductAttribute) {
	data.ID = types.Int64Value(int64(a.ID))
	data.Value = types.StringValue(a.Value)
	data.Name = types.StringValue(a.Name)
	data.Icon = stringOrNull(a.Icon)
	data.DisplayOrder = types.Int64Value(int64(a.DisplayOrder))
}
