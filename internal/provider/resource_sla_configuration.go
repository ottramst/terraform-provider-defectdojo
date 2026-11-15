package provider

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &SLAConfigurationResource{}
var _ resource.ResourceWithConfigure = &SLAConfigurationResource{}
var _ resource.ResourceWithImportState = &SLAConfigurationResource{}
var _ resource.ResourceWithIdentity = &SLAConfigurationResource{}

// defaultSLAConfigurationID is the ID of DefectDojo's "Default" SLA
// configuration, which can't be deleted.
const defaultSLAConfigurationID = 1

func NewSLAConfigurationResource() resource.Resource {
	return &SLAConfigurationResource{}
}

// SLAConfigurationResource defines the resource implementation.
type SLAConfigurationResource struct {
	client *defectdojo.Client
}

// SLAConfigurationResourceModel describes the resource data model.
type SLAConfigurationResourceModel struct {
	ID                       types.Int64  `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	Description              types.String `tfsdk:"description"`
	Critical                 types.Int64  `tfsdk:"critical"`
	EnforceCritical          types.Bool   `tfsdk:"enforce_critical"`
	High                     types.Int64  `tfsdk:"high"`
	EnforceHigh              types.Bool   `tfsdk:"enforce_high"`
	Medium                   types.Int64  `tfsdk:"medium"`
	EnforceMedium            types.Bool   `tfsdk:"enforce_medium"`
	Low                      types.Int64  `tfsdk:"low"`
	EnforceLow               types.Bool   `tfsdk:"enforce_low"`
	RestartSLAOnReactivation types.Bool   `tfsdk:"restart_sla_on_reactivation"`
}

func (r *SLAConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sla_configuration"
}

// slaRecalculationNote describes the SLA lock in the descriptions of the
// attributes it covers.
const slaRecalculationNote = "A change makes DefectDojo recalculate the SLA expiration dates of the findings of the products " +
	"that use the configuration, which its Celery workers do in the background; until that has finished, " +
	"DefectDojo refuses another change of the days or `enforce_*` attributes of the configuration, and of the SLA configuration of those products. " +
	"With the system setting `enable_finding_sla` off, DefectDojo never finishes, so the days can then change only once."

func slaDaysAttribute(severity string, days int64) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: fmt.Sprintf("The number of days in which %s findings must be remediated, at least 1. Defaults to `%d`. ", severity, days) +
			slaRecalculationNote,
		Optional: true,
		Computed: true,
		Default:  int64default.StaticInt64(days),
		Validators: []validator.Int64{
			int64validator.Between(1, math.MaxInt32),
		},
	}
}

func slaEnforceAttribute(severity string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: fmt.Sprintf("Whether %s findings get an SLA expiration date. Defaults to `true`. ", severity) + slaRecalculationNote,
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(true),
	}
}

func (r *SLAConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DefectDojo SLA configuration: the number of days in which findings of each severity must be remediated. " +
			"Products use it through `sla_configuration_id`.\n\n" +
			"DefectDojo refuses to delete an SLA configuration that products use: move them to another one first. " +
			"The `Default` SLA configuration (ID 1), which DefectDojo creates and gives products that don't name another, can't be deleted: " +
			"import it to manage its days, and destroying it only removes it from the Terraform state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the SLA configuration.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the SLA configuration, at most 128 characters, without leading or trailing whitespace, which DefectDojo removes. " +
					"Names are unique, compared case-sensitively.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 128),
					noSurroundingWhitespace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the SLA configuration, at most 512 characters, without leading or trailing whitespace, which DefectDojo removes.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 512),
					noSurroundingWhitespace(),
				},
			},
			"critical":         slaDaysAttribute("critical", 7),
			"enforce_critical": slaEnforceAttribute("critical"),
			"high":             slaDaysAttribute("high", 30),
			"enforce_high":     slaEnforceAttribute("high"),
			"medium":           slaDaysAttribute("medium", 90),
			"enforce_medium":   slaEnforceAttribute("medium"),
			"low":              slaDaysAttribute("low", 120),
			"enforce_low":      slaEnforceAttribute("low"),
			"restart_sla_on_reactivation": schema.BoolAttribute{
				MarkdownDescription: "Whether findings that a reimport reactivates get a new SLA period. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *SLAConfigurationResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("SLA configuration")
}

func (r *SLAConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *SLAConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SLAConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, _, err := r.client.SLAConfigurations.Create(ctx, &defectdojo.SLAConfiguration{
		Name:                     data.Name.ValueString(),
		Description:              data.Description.ValueString(),
		Critical:                 int(data.Critical.ValueInt64()),
		EnforceCritical:          data.EnforceCritical.ValueBool(),
		High:                     int(data.High.ValueInt64()),
		EnforceHigh:              data.EnforceHigh.ValueBool(),
		Medium:                   int(data.Medium.ValueInt64()),
		EnforceMedium:            data.EnforceMedium.ValueBool(),
		Low:                      int(data.Low.ValueInt64()),
		EnforceLow:               data.EnforceLow.ValueBool(),
		RestartSLAOnReactivation: data.RestartSLAOnReactivation.ValueBool(),
	}, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo SLA configuration",
			"Could not create SLA configuration "+strconv.Quote(data.Name.ValueString())+", unexpected error: "+err.Error(),
		)
		return
	}
	updateSLAConfigurationModel(&data, created)

	tflog.Trace(ctx, "created an SLA configuration", map[string]any{"id": created.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *SLAConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SLAConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sla, _, err := r.client.SLAConfigurations.Get(ctx, int(data.ID.ValueInt64()), nil)
	if err != nil {
		// Deleted outside Terraform
		if errors.Is(err, defectdojo.ErrNotFound) {
			removeGone(ctx, resp, data.ID)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading DefectDojo SLA configuration",
			fmt.Sprintf("Could not read SLA configuration %d, unexpected error: %s", data.ID.ValueInt64(), err),
		)
		return
	}
	updateSLAConfigurationModel(&data, sla)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *SLAConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SLAConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Every attribute is sent: DefectDojo refuses only days and enforce
	// flags that differ from the stored ones while it recalculates.
	patch := &defectdojo.SLAConfigurationPatch{
		Name:                     new(data.Name.ValueString()),
		Description:              defectdojo.Value(data.Description.ValueString()),
		Critical:                 new(int(data.Critical.ValueInt64())),
		EnforceCritical:          new(data.EnforceCritical.ValueBool()),
		High:                     new(int(data.High.ValueInt64())),
		EnforceHigh:              new(data.EnforceHigh.ValueBool()),
		Medium:                   new(int(data.Medium.ValueInt64())),
		EnforceMedium:            new(data.EnforceMedium.ValueBool()),
		Low:                      new(int(data.Low.ValueInt64())),
		EnforceLow:               new(data.EnforceLow.ValueBool()),
		RestartSLAOnReactivation: new(data.RestartSLAOnReactivation.ValueBool()),
	}
	if data.Description.IsNull() {
		patch.Description = defectdojo.Null[string]()
	}

	updated, _, err := r.client.SLAConfigurations.Patch(ctx, int(data.ID.ValueInt64()), patch, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo SLA configuration",
			fmt.Sprintf("Could not update SLA configuration %d, unexpected error: %s", data.ID.ValueInt64(), err)+slaRecalculationHint(err),
		)
		return
	}
	updateSLAConfigurationModel(&data, updated)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *SLAConfigurationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SLAConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())

	if id == defaultSLAConfigurationID {
		resp.Diagnostics.AddWarning(
			"DefectDojo's Default SLA configuration was not deleted",
			"DefectDojo can't delete its Default SLA configuration (ID 1). It was removed from the Terraform state and keeps its current settings.",
		)
		return
	}

	_, err := r.client.SLAConfigurations.Delete(ctx, id, nil)
	switch {
	case err == nil, errors.Is(err, defectdojo.ErrNotFound):
	case errors.Is(err, defectdojo.ErrConflict):
		resp.Diagnostics.AddError(
			"DefectDojo SLA configuration is in use",
			fmt.Sprintf("SLA configuration %d (%q) can't be deleted while products use it: %s\n\n"+
				"Move the products to another SLA configuration (their sla_configuration_id) first.", id, data.Name.ValueString(), err),
		)
	default:
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo SLA configuration",
			fmt.Sprintf("Could not delete SLA configuration %d, unexpected error: %s", id, err),
		)
	}
}

func (r *SLAConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "SLA configuration")
}

// updateSLAConfigurationModel copies the SLA configuration read from the
// server into data.
func updateSLAConfigurationModel(data *SLAConfigurationResourceModel, sla *defectdojo.SLAConfiguration) {
	data.ID = types.Int64Value(int64(sla.ID))
	data.Name = types.StringValue(sla.Name)
	data.Description = stringOrNull(sla.Description)
	data.Critical = types.Int64Value(int64(sla.Critical))
	data.EnforceCritical = types.BoolValue(sla.EnforceCritical)
	data.High = types.Int64Value(int64(sla.High))
	data.EnforceHigh = types.BoolValue(sla.EnforceHigh)
	data.Medium = types.Int64Value(int64(sla.Medium))
	data.EnforceMedium = types.BoolValue(sla.EnforceMedium)
	data.Low = types.Int64Value(int64(sla.Low))
	data.EnforceLow = types.BoolValue(sla.EnforceLow)
	data.RestartSLAOnReactivation = types.BoolValue(sla.RestartSLAOnReactivation)
}
