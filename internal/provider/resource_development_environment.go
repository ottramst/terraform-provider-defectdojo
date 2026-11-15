package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &DevelopmentEnvironmentResource{}
var _ resource.ResourceWithConfigure = &DevelopmentEnvironmentResource{}
var _ resource.ResourceWithImportState = &DevelopmentEnvironmentResource{}
var _ resource.ResourceWithIdentity = &DevelopmentEnvironmentResource{}

func NewDevelopmentEnvironmentResource() resource.Resource {
	return &DevelopmentEnvironmentResource{}
}

// DevelopmentEnvironmentResource defines the resource implementation.
type DevelopmentEnvironmentResource struct {
	client *defectdojo.Client
}

// DevelopmentEnvironmentModel describes the resource and data source data
// model.
type DevelopmentEnvironmentModel struct {
	ID   types.Int64  `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (r *DevelopmentEnvironmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_development_environment"
}

func (r *DevelopmentEnvironmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DefectDojo development environment, the environment a test ran in, such as `Production`. " +
			"DefectDojo comes with seven: Development, Staging, Production, Test, Pre-prod, Lab and Default. " +
			"Managing environments needs a superuser or the development environment configuration permissions.\n\n" +
			"-> DefectDojo's imports find the environment of a scan by name (`Development` when the import names none), " +
			"so keep names unique, and keep an environment named `Development` for imports that name none. " +
			"Creating one with a name that an environment already has fails; import that environment instead. " +
			"An environment that tests use can't be destroyed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the development environment.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": requiredText("The name of the development environment. DefectDojo doesn't require names to be unique, "+
				"but imports that name an environment fail when several have the name.", 200),
		},
	}
}

func (r *DevelopmentEnvironmentResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("development environment")
}

func (r *DevelopmentEnvironmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *DevelopmentEnvironmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DevelopmentEnvironmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A second environment with the name would break the imports that name
	// it, so refuse one, and suggest importing the existing environment.
	existing, _, err := r.client.DevelopmentEnvironments.GetByName(ctx, data.Name.ValueString())
	switch {
	case err == nil:
		resp.Diagnostics.AddError(
			"DefectDojo development environment exists",
			fmt.Sprintf("DefectDojo already has a development environment named %q (ID %d). "+
				"A second one would break the imports that name it; import the existing one instead.", existing.Name, existing.ID),
		)
		return
	case !errors.Is(err, defectdojo.ErrNotFound):
		resp.Diagnostics.AddError(
			"Error creating DefectDojo development environment",
			"Could not check for a development environment named "+strconv.Quote(data.Name.ValueString())+": "+err.Error(),
		)
		return
	}

	created, _, err := r.client.DevelopmentEnvironments.Create(ctx, &defectdojo.DevelopmentEnvironment{Name: data.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo development environment",
			"Could not create development environment "+strconv.Quote(data.Name.ValueString())+", unexpected error: "+err.Error(),
		)
		return
	}
	updateDevelopmentEnvironmentModel(&data, created)

	tflog.Trace(ctx, "created a development environment", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *DevelopmentEnvironmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DevelopmentEnvironmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	env, _, err := r.client.DevelopmentEnvironments.Get(ctx, int(data.ID.ValueInt64()))
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo development environment",
			fmt.Sprintf("Could not read development environment %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateDevelopmentEnvironmentModel(&data, env)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *DevelopmentEnvironmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DevelopmentEnvironmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, _, err := r.client.DevelopmentEnvironments.Patch(ctx, int(data.ID.ValueInt64()), &defectdojo.DevelopmentEnvironmentPatch{
		Name: new(data.Name.ValueString()),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo development environment",
			fmt.Sprintf("Could not update development environment %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateDevelopmentEnvironmentModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *DevelopmentEnvironmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DevelopmentEnvironmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())

	_, err := r.client.DevelopmentEnvironments.Delete(ctx, id)
	switch {
	case err == nil, errors.Is(err, defectdojo.ErrNotFound):
	case errors.Is(err, defectdojo.ErrConflict):
		resp.Diagnostics.AddError(
			"DefectDojo development environment is in use",
			fmt.Sprintf("Development environment %d (%q) can't be deleted while tests use it. "+
				"Move or delete the tests first, or keep the environment and remove it from the configuration with a removed block. DefectDojo: %s",
				id, data.Name.ValueString(), err),
		)
	default:
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo development environment",
			fmt.Sprintf("Could not delete development environment %d, unexpected error: %s", id, err),
		)
	}
}

func (r *DevelopmentEnvironmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "development environment")
}

// updateDevelopmentEnvironmentModel copies the development environment read
// from the server into data.
func updateDevelopmentEnvironmentModel(data *DevelopmentEnvironmentModel, env *defectdojo.DevelopmentEnvironment) {
	data.ID = types.Int64Value(int64(env.ID))
	data.Name = types.StringValue(env.Name)
}
