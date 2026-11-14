package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &EngagementResource{}
var _ resource.ResourceWithConfigure = &EngagementResource{}
var _ resource.ResourceWithImportState = &EngagementResource{}
var _ resource.ResourceWithIdentity = &EngagementResource{}
var _ resource.ResourceWithConfigValidators = &EngagementResource{}

// engagementDeleteTimeout bounds how long Delete waits for an asynchronous
// delete to finish.
const engagementDeleteTimeout = 5 * time.Minute

// engagementStatuses are the values of an engagement's status.
var engagementStatuses = []string{
	defectdojo.EngagementStatusNotStarted,
	defectdojo.EngagementStatusBlocked,
	defectdojo.EngagementStatusCancelled,
	defectdojo.EngagementStatusCompleted,
	defectdojo.EngagementStatusInProgress,
	defectdojo.EngagementStatusOnHold,
	defectdojo.EngagementStatusScheduled,
	defectdojo.EngagementStatusWaitingForResource,
}

// urlPattern loosely matches the URLs DefectDojo's URL fields accept.
var urlPattern = regexp.MustCompile(`^(https?|ftps?)://\S+$`)

func NewEngagementResource() resource.Resource {
	return &EngagementResource{}
}

// EngagementResource defines the resource implementation.
type EngagementResource struct {
	client *defectdojo.Client
}

// EngagementResourceModel describes the resource data model.
type EngagementResourceModel struct {
	ID                        types.Int64  `tfsdk:"id"`
	ProductID                 types.Int64  `tfsdk:"product_id"`
	Name                      types.String `tfsdk:"name"`
	TargetStart               types.String `tfsdk:"target_start"`
	TargetEnd                 types.String `tfsdk:"target_end"`
	EngagementType            types.String `tfsdk:"engagement_type"`
	Status                    types.String `tfsdk:"status"`
	Active                    types.Bool   `tfsdk:"active"`
	Description               types.String `tfsdk:"description"`
	Reason                    types.String `tfsdk:"reason"`
	Version                   types.String `tfsdk:"version"`
	BuildID                   types.String `tfsdk:"build_id"`
	CommitHash                types.String `tfsdk:"commit_hash"`
	BranchTag                 types.String `tfsdk:"branch_tag"`
	FirstContacted            types.String `tfsdk:"first_contacted"`
	Tracker                   types.String `tfsdk:"tracker"`
	TestStrategy              types.String `tfsdk:"test_strategy"`
	SourceCodeManagementURI   types.String `tfsdk:"source_code_management_uri"`
	DeduplicationOnEngagement types.Bool   `tfsdk:"deduplication_on_engagement"`
	ThreatModel               types.Bool   `tfsdk:"threat_model"`
	APITest                   types.Bool   `tfsdk:"api_test"`
	PenTest                   types.Bool   `tfsdk:"pen_test"`
	CheckList                 types.Bool   `tfsdk:"check_list"`
	LeadID                    types.Int64  `tfsdk:"lead_id"`
	Tags                      types.Set    `tfsdk:"tags"`
	ForceDestroy              types.Bool   `tfsdk:"force_destroy"`
}

func (r *EngagementResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_engagement"
}

func (r *EngagementResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	text := func(description string, maxLength int) schema.StringAttribute {
		return schema.StringAttribute{
			MarkdownDescription: description + ", at most " + strconv.Itoa(maxLength) + " characters, without leading or trailing whitespace, which DefectDojo removes.",
			Optional:            true,
			Validators: []validator.String{
				stringvalidator.UTF8LengthBetween(1, maxLength),
				noSurroundingWhitespace(),
			},
		}
	}
	url := func(description string, maxLength int) schema.StringAttribute {
		return schema.StringAttribute{
			MarkdownDescription: description + ", a URL of at most " + strconv.Itoa(maxLength) + " characters.",
			Optional:            true,
			Validators: []validator.String{
				stringvalidator.UTF8LengthBetween(1, maxLength),
				stringvalidator.RegexMatches(urlPattern, "must be an http, https, ftp or ftps URL"),
			},
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
		MarkdownDescription: "Manages a DefectDojo engagement, such as the long-lived CI/CD engagement that a pipeline or Dependency-Track imports scans into.\n\n" +
			"~> **Deleting an engagement deletes its tests and their findings.** " +
			"The provider refuses to destroy an engagement that has tests unless `force_destroy` is set.\n\n" +
			"-> **Imports move `target_end`.** Every import into a `CI/CD` engagement moves its `target_end` to the scan date when that is later. " +
			"Use a `target_end` far in the future, which also keeps DefectDojo from closing the engagement automatically, " +
			"or `lifecycle { ignore_changes = [target_end] }`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the engagement.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"product_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the product of the engagement. Changing it moves the engagement to another product.",
				Required:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the engagement, at most 300 characters, without leading or trailing whitespace, which DefectDojo removes. " +
					"Names need not be unique; the `defectdojo_engagement` data source finds engagements by product and name.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, 300),
					noSurroundingWhitespace(),
				},
			},
			"target_start": schema.StringAttribute{
				MarkdownDescription: "The date the engagement starts, as `YYYY-MM-DD`. It must not be after `target_end`.",
				Required:            true,
				Validators:          []validator.String{dateValidator{}},
			},
			"target_end": schema.StringAttribute{
				MarkdownDescription: "The date the engagement ends, as `YYYY-MM-DD`. Every import into a `CI/CD` engagement moves it to the scan date when that is later.",
				Required:            true,
				Validators:          []validator.String{dateValidator{}},
			},
			"engagement_type": schema.StringAttribute{
				MarkdownDescription: "The type of the engagement: `Interactive` or `CI/CD`. DefectDojo uses `Interactive` when it is not set.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(defectdojo.EngagementTypeInteractive, defectdojo.EngagementTypeCICD),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "The status of the engagement: one of `Not Started`, `Blocked`, `Cancelled`, `Completed`, `In Progress`, `On Hold`, `Scheduled` and `Waiting for Resource`. " +
					"DefectDojo uses `Not Started` when it is not set. The provider sends it only when the configuration changes it. " +
					"Pipelines and DefectDojo's automatic closing of engagements change it too: when it is configured for such an engagement, use `lifecycle { ignore_changes = [status] }`; when it is not configured, the provider leaves it as it is. " +
					"Setting it doesn't change `active`: an engagement closed elsewhere stays inactive.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.OneOf(engagementStatuses...)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"active": schema.BoolAttribute{
				MarkdownDescription: "Whether the engagement is open. Closing an engagement in DefectDojo makes it inactive, and reopening it active again.",
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"description": text("The description of the engagement", 2000),
			"reason":      text("The reason for the engagement", 2000),
			"version":     text("The version of the product the engagement tests", 100),
			"build_id":    text("The ID of the build the engagement tests", 150),
			"commit_hash": text("The commit the engagement tests", 150),
			"branch_tag":  text("The branch or tag the engagement tests", 150),
			"first_contacted": schema.StringAttribute{
				MarkdownDescription: "The date of the first contact, as `YYYY-MM-DD`.",
				Optional:            true,
				Validators:          []validator.String{dateValidator{}},
			},
			"tracker":                     url("The link to the epic or ticket of the engagement", 200),
			"test_strategy":               url("The link to the test strategy of the engagement", 200),
			"source_code_management_uri":  url("The link to the source code the engagement tests", 600),
			"deduplication_on_engagement": flag("Whether findings are only deduplicated within the engagement, instead of within the product.", false),
			"threat_model":                flag("Whether the engagement includes threat modelling.", true),
			"api_test":                    flag("Whether the engagement includes API tests.", true),
			"pen_test":                    flag("Whether the engagement includes penetration tests.", true),
			"check_list":                  flag("Whether the engagement includes a check list.", true),
			"lead_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the user who leads the engagement, an active user.",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"tags": schema.SetAttribute{
				MarkdownDescription: "The tags of the engagement, each at most 255 characters, without spaces, commas or quotes. " +
					"Tags are compared ignoring case, and the provider keeps the configured spelling. " +
					"When the product passes its tags on (`enable_product_tag_inheritance`, or DefectDojo's system setting), DefectDojo adds them to its engagements. " +
					"The provider can't tell inherited tags apart, so it always hides the tags of the engagement's product unless they are configured. " +
					"A change of the product's tags reaches its engagements later, through DefectDojo's background workers, and a refresh then shows the result. " +
					"Moving an engagement without tags of its own to another product replaces the old product's tags with the new product's. " +
					"When it is not set, the provider leaves the tags as they are, so they can be managed elsewhere. " +
					"An empty set removes them all, including the tags the product passes on.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Validators:  tagValidators(),
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"force_destroy": schema.BoolAttribute{
				MarkdownDescription: "Whether to destroy the engagement even when it has tests, which DefectDojo then deletes too, with their findings. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *EngagementResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{targetDatesValidator{}}
}

func (r *EngagementResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("engagement")
}

func (r *EngagementResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureResource(req, resp); client != nil {
		r.client = client
	}
}

func (r *EngagementResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EngagementResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	e := &defectdojo.Engagement{
		Product:                   int(data.ProductID.ValueInt64()),
		Name:                      data.Name.ValueString(),
		TargetStart:               mustDate(data.TargetStart),
		TargetEnd:                 mustDate(data.TargetEnd),
		FirstContacted:            mustDate(data.FirstContacted),
		Description:               data.Description.ValueString(),
		Reason:                    data.Reason.ValueString(),
		Version:                   data.Version.ValueString(),
		BuildID:                   data.BuildID.ValueString(),
		CommitHash:                data.CommitHash.ValueString(),
		BranchTag:                 data.BranchTag.ValueString(),
		Tracker:                   data.Tracker.ValueString(),
		TestStrategy:              data.TestStrategy.ValueString(),
		SourceCodeManagementURI:   data.SourceCodeManagementURI.ValueString(),
		DeduplicationOnEngagement: data.DeduplicationOnEngagement.ValueBool(),
		ThreatModel:               data.ThreatModel.ValueBool(),
		APITest:                   data.APITest.ValueBool(),
		PenTest:                   data.PenTest.ValueBool(),
		CheckList:                 data.CheckList.ValueBool(),
		Lead:                      int(data.LeadID.ValueInt64()),
		// Unknown (not configured): empty, DefectDojo's default.
		Status:         data.Status.ValueString(),
		EngagementType: data.EngagementType.ValueString(),
	}
	// Not configured: nil, left out, so that the engagement inherits the
	// product's tags.
	if known(data.Tags) {
		e.Tags = stringsFromSet(ctx, data.Tags, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	created, _, err := r.client.Engagements.Create(ctx, e)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating DefectDojo engagement",
			"Could not create engagement "+strconv.Quote(e.Name)+", unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, "created an engagement", map[string]any{"id": created.ID})

	// Read it back for the tags the product passed on, which the create
	// response may lack.
	planned := data.Tags
	data.ID = types.Int64Value(int64(created.ID))
	if !r.read(ctx, &data, planned, &resp.Diagnostics) {
		resp.Diagnostics.AddError(
			"Error reading DefectDojo engagement",
			fmt.Sprintf("Engagement %d was created, but it was gone when the provider read it back.", created.ID),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	keepPlannedTags(&data, planned)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *EngagementResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EngagementResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Product tags in the prior state stay visible.
	if !r.read(ctx, &data, data.Tags, &resp.Diagnostics) {
		// Deleted outside Terraform
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

func (r *EngagementResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state EngagementResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	// The plan holds the prior tags when they are not configured
	// (UseStateForUnknown), so check the configuration.
	var tags types.Set
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("tags"), &tags)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())
	moved := !data.ProductID.Equal(state.ProductID)

	// A partial update, so that unmanaged attributes are kept.
	patch := &defectdojo.EngagementPatch{
		Product:                   new(int(data.ProductID.ValueInt64())),
		Name:                      defectdojo.Value(data.Name.ValueString()),
		TargetStart:               new(mustDate(data.TargetStart)),
		TargetEnd:                 new(mustDate(data.TargetEnd)),
		FirstContacted:            nullableDate(data.FirstContacted),
		Description:               nullableString(data.Description),
		Reason:                    nullableString(data.Reason),
		Version:                   nullableString(data.Version),
		BuildID:                   nullableString(data.BuildID),
		CommitHash:                nullableString(data.CommitHash),
		BranchTag:                 nullableString(data.BranchTag),
		Tracker:                   nullableString(data.Tracker),
		TestStrategy:              nullableString(data.TestStrategy),
		SourceCodeManagementURI:   nullableString(data.SourceCodeManagementURI),
		DeduplicationOnEngagement: new(data.DeduplicationOnEngagement.ValueBool()),
		ThreatModel:               new(data.ThreatModel.ValueBool()),
		APITest:                   new(data.APITest.ValueBool()),
		PenTest:                   new(data.PenTest.ValueBool()),
		CheckList:                 new(data.CheckList.ValueBool()),
		Lead:                      nullableInt(data.LeadID),
	}
	// Status and type are sent only when the plan changes them, so that an
	// unrelated change never reopens an engagement closed elsewhere.
	if known(data.Status) && !data.Status.Equal(state.Status) {
		patch.Status = new(data.Status.ValueString())
	}
	if known(data.EngagementType) && !data.EngagementType.Equal(state.EngagementType) {
		patch.EngagementType = new(data.EngagementType.ValueString())
	}
	// Tags are sent when they are configured, and on a move: any tags make
	// DefectDojo replace the old product's tags with the new product's.
	planned := stringsFromSet(ctx, data.Tags, &resp.Diagnostics)
	switch {
	case known(tags):
		patch.Tags = &planned
	case moved && len(planned) > 0:
		patch.Tags = &planned
	case moved:
		// No tags of its own: an empty list would only remove the old
		// product's tags, so send the new product's when it passes them on.
		patch.Tags = r.inheritedTags(ctx, int(data.ProductID.ValueInt64()), &resp.Diagnostics)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if _, _, err := r.client.Engagements.Patch(ctx, id, patch); err != nil {
		resp.Diagnostics.AddError(
			"Error updating DefectDojo engagement",
			fmt.Sprintf("Could not update engagement %d, unexpected error: %s", id, err),
		)
		return
	}

	plannedTags := data.Tags
	if !r.read(ctx, &data, plannedTags, &resp.Diagnostics) {
		resp.Diagnostics.AddError(
			"Error reading DefectDojo engagement",
			fmt.Sprintf("Engagement %d was updated, but it was gone when the provider read it back.", id),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	keepPlannedTags(&data, plannedTags)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setIDIdentity(ctx, resp.Identity, data.ID)...)
}

// keepPlannedTags sets the tags of data to the planned tags, when they are
// known. After a write, DefectDojo's tags can still differ from the plan for
// a while: a change of the product's tags reaches its engagements later,
// through Celery, and DefectDojo puts back inherited tags a write removes.
// The next refresh reconciles them.
func keepPlannedTags(data *EngagementResourceModel, planned types.Set) {
	if known(planned) {
		data.Tags = planned
	}
}

// inheritedTags returns the tags the product passes on to its engagements:
// its tags, if it has enable_product_tag_inheritance, else none.
func (r *EngagementResource) inheritedTags(ctx context.Context, productID int, diags *diag.Diagnostics) *[]string {
	p, _, err := r.client.Products.Get(ctx, productID)
	if err != nil {
		diags.AddError("Error updating DefectDojo engagement",
			fmt.Sprintf("Could not read product %d, for the tags it passes on, unexpected error: %s", productID, err))
		return nil
	}
	tags := []string{}
	if p.EnableProductTagInheritance {
		tags = append(tags, p.Tags...)
	}
	return &tags
}

// read reads the engagement data.ID into data. It reports false when the
// engagement doesn't exist. The tags of the engagement's product are hidden
// unless they are in visible, ignoring case: DefectDojo adds the product's
// tags to engagements of products that pass their tags on (for the product,
// or system-wide, which only superusers can read), and doesn't say which
// tags are inherited.
func (r *EngagementResource) read(ctx context.Context, data *EngagementResourceModel, visible types.Set, diags *diag.Diagnostics) bool {
	id := int(data.ID.ValueInt64())
	e, _, err := r.client.Engagements.Get(ctx, id)
	if errors.Is(err, defectdojo.ErrNotFound) {
		return false
	}
	if err != nil {
		diags.AddError("Error reading DefectDojo engagement", fmt.Sprintf("Could not read engagement %d, unexpected error: %s", id, err))
		return true
	}
	hidden := map[string]bool{}
	p, _, err := r.client.Products.Get(ctx, e.Product)
	if err != nil {
		diags.AddError("Error reading DefectDojo engagement",
			fmt.Sprintf("Could not read product %d, for the tags it passes on to engagement %d, unexpected error: %s", e.Product, id, err))
		return true
	}
	for _, tag := range p.Tags {
		hidden[strings.ToLower(tag)] = true
	}
	if known(visible) {
		for _, v := range visible.Elements() {
			if s, ok := v.(types.String); ok && known(s) {
				delete(hidden, strings.ToLower(s.ValueString()))
			}
		}
	}
	tags := make([]string, 0, len(e.Tags))
	for _, tag := range e.Tags {
		if !hidden[strings.ToLower(tag)] {
			tags = append(tags, tag)
		}
	}
	updateEngagementModel(data, e, tags, visible)
	return true
}

func (r *EngagementResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data EngagementResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(data.ID.ValueInt64())

	if !data.ForceDestroy.ValueBool() {
		n, _, err := r.client.Engagements.CountTests(ctx, id)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return // Deleted outside Terraform
		}
		if err != nil {
			resp.Diagnostics.AddError(
				"Error deleting DefectDojo engagement",
				fmt.Sprintf("Could not count the tests of engagement %d, unexpected error: %s", id, err),
			)
			return
		}
		if n > 0 {
			resp.Diagnostics.AddError(
				"DefectDojo engagement has tests",
				fmt.Sprintf("Engagement %d (%q) has %d tests. Deleting it would delete them too, with their findings. "+
					"Delete the tests first, or set force_destroy = true and apply before destroying.",
					id, data.Name.ValueString(), n),
			)
			return
		}
	}

	if _, err := r.client.Engagements.Delete(ctx, id); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo engagement",
			fmt.Sprintf("Could not delete engagement %d, unexpected error: %s", id, err),
		)
		return
	}

	waitCtx, cancel := context.WithTimeout(ctx, engagementDeleteTimeout)
	defer cancel()
	err := waitForDeletion(waitCtx, func(ctx context.Context) error {
		_, _, err := r.client.Engagements.Get(ctx, id)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting DefectDojo engagement",
			fmt.Sprintf("Engagement %d was not deleted: %s", id, err),
		)
	}
}

func (r *EngagementResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "engagement")
}

// updateEngagementModel copies the engagement read from the server into
// data, with the given tags. Tags that match one of tagReference ignoring
// case keep its spelling.
func updateEngagementModel(data *EngagementResourceModel, e *defectdojo.Engagement, tags []string, tagReference types.Set) {
	data.ID = types.Int64Value(int64(e.ID))
	data.ProductID = types.Int64Value(int64(e.Product))
	data.Name = types.StringValue(e.Name)
	data.TargetStart = types.StringValue(e.TargetStart.String())
	data.TargetEnd = types.StringValue(e.TargetEnd.String())
	data.FirstContacted = stringOrNull(e.FirstContacted.String())
	data.EngagementType = types.StringValue(e.EngagementType)
	data.Status = types.StringValue(e.Status)
	data.Active = types.BoolValue(e.Active)
	data.Description = stringOrNull(e.Description)
	data.Reason = stringOrNull(e.Reason)
	data.Version = stringOrNull(e.Version)
	data.BuildID = stringOrNull(e.BuildID)
	data.CommitHash = stringOrNull(e.CommitHash)
	data.BranchTag = stringOrNull(e.BranchTag)
	data.Tracker = stringOrNull(e.Tracker)
	data.TestStrategy = stringOrNull(e.TestStrategy)
	data.SourceCodeManagementURI = stringOrNull(e.SourceCodeManagementURI)
	data.DeduplicationOnEngagement = types.BoolValue(e.DeduplicationOnEngagement)
	data.ThreatModel = types.BoolValue(e.ThreatModel)
	data.APITest = types.BoolValue(e.APITest)
	data.PenTest = types.BoolValue(e.PenTest)
	data.CheckList = types.BoolValue(e.CheckList)
	data.LeadID = int64OrNull(e.Lead)
	data.Tags = tagSet(tags, tagReference)
	// Imported resources get the default.
	if data.ForceDestroy.IsNull() || data.ForceDestroy.IsUnknown() {
		data.ForceDestroy = types.BoolValue(false)
	}
}

// mustDate returns the date of a validated date attribute, or the zero Date
// when it is null.
func mustDate(value types.String) defectdojo.Date {
	d, _ := defectdojo.ParseDate(value.ValueString())
	return d
}

// nullableDate returns the value of an optional date attribute for a partial
// update: null clears it.
func nullableDate(value types.String) defectdojo.Nullable[defectdojo.Date] {
	if value.IsNull() {
		return defectdojo.Null[defectdojo.Date]()
	}
	return defectdojo.Value(mustDate(value))
}

// dateValidator accepts dates written as YYYY-MM-DD. DefectDojo also accepts
// other spellings, such as 2026-1-5, but returns them as 2026-01-05, which
// would not match the configuration.
type dateValidator struct{}

func (v dateValidator) Description(_ context.Context) string {
	return "must be a date written as YYYY-MM-DD"
}

func (v dateValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v dateValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if !known(req.ConfigValue) {
		return
	}
	if _, err := time.Parse(time.DateOnly, req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Attribute Value",
			fmt.Sprintf("%q is not a date written as YYYY-MM-DD.", req.ConfigValue.ValueString()))
	}
}

// targetDatesValidator rejects a target_start after target_end. DefectDojo
// checks it only when an engagement is created.
type targetDatesValidator struct{}

func (v targetDatesValidator) Description(_ context.Context) string {
	return "target_start must not be after target_end"
}

func (v targetDatesValidator) MarkdownDescription(ctx context.Context) string {
	return "`target_start` must not be after `target_end`"
}

func (v targetDatesValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var start, end types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("target_start"), &start)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("target_end"), &end)...)
	if resp.Diagnostics.HasError() || !known(start) || !known(end) {
		return
	}
	s, err1 := time.Parse(time.DateOnly, start.ValueString())
	e, err2 := time.Parse(time.DateOnly, end.ValueString())
	if err1 == nil && err2 == nil && s.After(e) {
		resp.Diagnostics.AddAttributeError(path.Root("target_start"), "Invalid Attribute Value",
			fmt.Sprintf("target_start (%s) must not be after target_end (%s).", start.ValueString(), end.ValueString()))
	}
}
