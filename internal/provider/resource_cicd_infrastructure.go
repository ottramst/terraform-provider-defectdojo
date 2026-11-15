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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &CICDInfrastructureResource{}
var _ resource.ResourceWithConfigure = &CICDInfrastructureResource{}
var _ resource.ResourceWithImportState = &CICDInfrastructureResource{}
var _ resource.ResourceWithIdentity = &CICDInfrastructureResource{}

// cicdInfrastructureTypes are the types DefectDojo accepts.
var cicdInfrastructureTypes = []string{
	defectdojo.CICDInfrastructureTypeSCMServer,
	defectdojo.CICDInfrastructureTypeBuildServer,
	defectdojo.CICDInfrastructureTypeOrchestration,
}

func NewCICDInfrastructureResource() resource.Resource {
	return &CICDInfrastructureResource{}
}

// CICDInfrastructureResource defines the resource implementation.
type CICDInfrastructureResource struct {
	client *defectdojo.Client
}

// CICDInfrastructureModel describes the resource and data source data model.
type CICDInfrastructureModel struct {
	ID                 types.Int64  `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	InfrastructureType types.String `tfsdk:"infrastructure_type"`
	Description        types.String `tfsdk:"description"`
	URL                types.String `tfsdk:"url"`
}

func (r *CICDInfrastructureResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cicd_infrastructure"
}

func (r *CICDInfrastructureResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DefectDojo CI/CD infrastructure object: a source code management server, build server or orchestration engine, " +
			"which engagements link to with `cicd_scm_server_id`, `cicd_build_server_id` and `cicd_orchestration_engine_id`. " +
			"Managing these objects needs a superuser or the CI/CD infrastructure configuration permissions, including the view or change permission: " +
			"without them, DefectDojo ignores `description` and `url`, and the apply fails with an inconsistent result.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the CI/CD infrastructure object.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": requiredText("The name of the object, unique among the objects of its type.", 200),
			"infrastructure_type": schema.StringAttribute{
				MarkdownDescription: "The type of the object: `scm_server`, `build_server` or `orchestration`. " +
					"DefectDojo can't change it, so changing it replaces the object.",
				Required:   true,
				Validators: []validator.String{stringvalidator.OneOf(cicdInfrastructureTypes...)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the object, at most 2000 characters, without leading or trailing whitespace, which DefectDojo removes.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 2000),
					noSurroundingWhitespace(),
				},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The public http, https, ftp or ftps URL of the tool, at most 2000 characters, such as `https://jenkins.example.com`.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 2000),
					stringvalidator.RegexMatches(urlPattern, "must be an http, https, ftp or ftps URL"),
				},
			},
		},
	}
}

func (r *CICDInfrastructureResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("CI/CD infrastructure object")
}

func (r *CICDInfrastructureResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *CICDInfrastructureResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CICDInfrastructureModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, _, err := r.client.CICDInfrastructure.Create(ctx, &defectdojo.CICDInfrastructure{
		Name:               data.Name.ValueString(),
		InfrastructureType: data.InfrastructureType.ValueString(),
		Description:        data.Description.ValueString(),
		URL:                data.URL.ValueString(),
	})
	var errResp *defectdojo.ErrorResponse
	if errors.As(err, &errResp) && errors.Is(err, defectdojo.ErrBadRequest) && len(errResp.Fields["non_field_errors"]) > 0 {
		resp.Diagnostics.AddError(
			"DefectDojo CI/CD infrastructure name is taken",
			fmt.Sprintf("Another CI/CD infrastructure object of type %s is named %q; names are unique within a type. DefectDojo: %s",
				data.InfrastructureType.ValueString(), data.Name.ValueString(), err),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo CI/CD infrastructure",
			"Could not create CI/CD infrastructure object "+strconv.Quote(data.Name.ValueString())+", unexpected error: "+err.Error(),
		)
		return
	}
	updateCICDInfrastructureModel(&data, created)

	tflog.Trace(ctx, "created a CI/CD infrastructure object", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *CICDInfrastructureResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CICDInfrastructureModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ci, _, err := r.client.CICDInfrastructure.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			removeGone(ctx, resp, data.ID)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo CI/CD infrastructure",
			fmt.Sprintf("Could not read CI/CD infrastructure object %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateCICDInfrastructureModel(&data, ci)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *CICDInfrastructureResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CICDInfrastructureModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := data.ID.ValueInt64()

	// The type can't change: a change replaces the object.
	updated, _, err := r.client.CICDInfrastructure.Patch(ctx, int(id), &defectdojo.CICDInfrastructurePatch{
		Name:        new(data.Name.ValueString()),
		Description: new(data.Description.ValueString()),
		URL:         new(data.URL.ValueString()),
	})
	switch {
	case errors.Is(err, defectdojo.ErrConflict):
		resp.Diagnostics.AddError(
			"DefectDojo CI/CD infrastructure name is taken",
			fmt.Sprintf("Another CI/CD infrastructure object of type %s is named %q; names are unique within a type.",
				data.InfrastructureType.ValueString(), data.Name.ValueString()),
		)
		return
	case err != nil:
		resp.Diagnostics.AddError(
			"Error updating DefectDojo CI/CD infrastructure",
			fmt.Sprintf("Could not update CI/CD infrastructure object %d, unexpected error: %s", id, err),
		)
		return
	}
	updateCICDInfrastructureModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *CICDInfrastructureResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CICDInfrastructureModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.CICDInfrastructure.Delete(ctx, int(data.ID.ValueInt64())); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo CI/CD infrastructure",
			fmt.Sprintf("Could not delete CI/CD infrastructure object %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
	}
}

func (r *CICDInfrastructureResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "CI/CD infrastructure object")
}

// updateCICDInfrastructureModel copies the CI/CD infrastructure object read
// from the server into data.
func updateCICDInfrastructureModel(data *CICDInfrastructureModel, ci *defectdojo.CICDInfrastructure) {
	data.ID = types.Int64Value(int64(ci.ID))
	data.Name = types.StringValue(ci.Name)
	data.InfrastructureType = types.StringValue(ci.InfrastructureType)
	data.Description = stringOrNull(ci.Description)
	data.URL = stringOrNull(ci.URL)
}
