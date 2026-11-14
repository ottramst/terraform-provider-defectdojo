package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &EngagementDataSource{}
var _ datasource.DataSourceWithConfigure = &EngagementDataSource{}
var _ datasource.DataSourceWithConfigValidators = &EngagementDataSource{}

func NewEngagementDataSource() datasource.DataSource {
	return &EngagementDataSource{}
}

// EngagementDataSource looks up an engagement.
type EngagementDataSource struct {
	client *defectdojo.Client
}

// EngagementDataSourceModel describes the defectdojo_engagement data source.
// It has the attributes of the resource, without force_destroy.
type EngagementDataSourceModel struct {
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
}

func (d *EngagementDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_engagement"
}

func (d *EngagementDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	text := func(description string) schema.Attribute {
		return schema.StringAttribute{MarkdownDescription: description, Computed: true}
	}
	flag := func(description string) schema.Attribute {
		return schema.BoolAttribute{MarkdownDescription: description, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a DefectDojo engagement by ID, or by product and name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the engagement to look up. Exactly one of `id` and `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"product_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the product of the engagement. Required with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the engagement to look up in the product `product_id`, compared exactly and case-sensitively. " +
					"Engagement names need not be unique: the lookup fails when the product has several engagements with the name. " +
					"Exactly one of `id` and `name` must be set.",
				Optional: true,
				Computed: true,
			},
			"target_start":                text("The date the engagement starts, as `YYYY-MM-DD`."),
			"target_end":                  text("The date the engagement ends, as `YYYY-MM-DD`."),
			"engagement_type":             text("The type of the engagement: `Interactive` or `CI/CD`."),
			"status":                      text("The status of the engagement."),
			"active":                      flag("Whether the engagement is open."),
			"description":                 text("The description of the engagement, or null."),
			"reason":                      text("The reason for the engagement, or null."),
			"version":                     text("The version of the product the engagement tests, or null."),
			"build_id":                    text("The ID of the build the engagement tests, or null."),
			"commit_hash":                 text("The commit the engagement tests, or null."),
			"branch_tag":                  text("The branch or tag the engagement tests, or null."),
			"first_contacted":             text("The date of the first contact, as `YYYY-MM-DD`, or null."),
			"tracker":                     text("The link to the epic or ticket of the engagement, or null."),
			"test_strategy":               text("The link to the test strategy of the engagement, or null."),
			"source_code_management_uri":  text("The link to the source code the engagement tests, or null."),
			"deduplication_on_engagement": flag("Whether findings are only deduplicated within the engagement."),
			"threat_model":                flag("Whether the engagement includes threat modelling."),
			"api_test":                    flag("Whether the engagement includes API tests."),
			"pen_test":                    flag("Whether the engagement includes penetration tests."),
			"check_list":                  flag("Whether the engagement includes a check list."),
			"lead_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the user who leads the engagement, or null.",
				Computed:            true,
			},
			"tags": schema.SetAttribute{
				MarkdownDescription: "The tags of the engagement, including those its product passes on.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *EngagementDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		datasourcevalidator.RequiredTogether(path.MatchRoot("name"), path.MatchRoot("product_id")),
	}
}

func (d *EngagementDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureDataSource(req, resp); client != nil {
		d.client = client
	}
}

func (d *EngagementDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data EngagementDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var e *defectdojo.Engagement
	if !data.ID.IsNull() {
		var err error
		e, _, err = d.client.Engagements.Get(ctx, int(data.ID.ValueInt64()))
		if errors.Is(err, defectdojo.ErrNotFound) {
			resp.Diagnostics.AddError("DefectDojo engagement not found",
				fmt.Sprintf("There is no engagement with ID %d, or the provider user may not see it.", data.ID.ValueInt64()))
			return
		}
		if err != nil {
			resp.Diagnostics.AddError("Error reading DefectDojo engagement",
				fmt.Sprintf("Could not read engagement %d: %s", data.ID.ValueInt64(), err))
			return
		}
	} else {
		productID, name := int(data.ProductID.ValueInt64()), data.Name.ValueString()
		what := fmt.Sprintf("name %q in product %d", name, productID)
		// The name filter is exact; two results are enough to tell that the
		// name is ambiguous.
		list, _, err := d.client.Engagements.List(ctx, &defectdojo.ListEngagementsOptions{
			ListOptions: defectdojo.ListOptions{Limit: 2}, Product: productID, Name: name,
		})
		// The product filter rejects products that don't exist with a 400.
		var errResp *defectdojo.ErrorResponse
		missingProduct := errors.As(err, &errResp) && errResp.StatusCode == http.StatusBadRequest && len(errResp.Fields["product"]) > 0
		switch {
		case missingProduct || err == nil && list.Count == 0:
			resp.Diagnostics.AddError("DefectDojo engagement not found",
				"There is no engagement with "+what+", or the provider user may not see it.")
			return
		case err != nil:
			resp.Diagnostics.AddError("Error reading DefectDojo engagement", "Could not look up the engagement with "+what+": "+err.Error())
			return
		case list.Count > 1:
			resp.Diagnostics.AddError("Several DefectDojo engagements found",
				fmt.Sprintf("Product %d has %d engagements named %q. Look the engagement up by id instead.", productID, list.Count, name))
			return
		}
		e = list.Engagements[0]
	}

	var m EngagementResourceModel
	updateEngagementModel(&m, e, e.Tags, types.SetNull(types.StringType))
	data = EngagementDataSourceModel{
		ID:                        m.ID,
		ProductID:                 m.ProductID,
		Name:                      m.Name,
		TargetStart:               m.TargetStart,
		TargetEnd:                 m.TargetEnd,
		EngagementType:            m.EngagementType,
		Status:                    m.Status,
		Active:                    m.Active,
		Description:               m.Description,
		Reason:                    m.Reason,
		Version:                   m.Version,
		BuildID:                   m.BuildID,
		CommitHash:                m.CommitHash,
		BranchTag:                 m.BranchTag,
		FirstContacted:            m.FirstContacted,
		Tracker:                   m.Tracker,
		TestStrategy:              m.TestStrategy,
		SourceCodeManagementURI:   m.SourceCodeManagementURI,
		DeduplicationOnEngagement: m.DeduplicationOnEngagement,
		ThreatModel:               m.ThreatModel,
		APITest:                   m.APITest,
		PenTest:                   m.PenTest,
		CheckList:                 m.CheckList,
		LeadID:                    m.LeadID,
		Tags:                      m.Tags,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
