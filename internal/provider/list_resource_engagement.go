package provider

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// EngagementListModel describes the configuration of the defectdojo_engagement
// list resource.
type EngagementListModel struct {
	ProductID    types.Int64  `tfsdk:"product_id"`
	NameContains types.String `tfsdk:"name_contains"`
}

func NewEngagementListResource() list.ListResource {
	return &listResource[EngagementListModel, defectdojo.Engagement]{
		typeName: "_engagement",
		schema: schema.Schema{
			MarkdownDescription: "Lists the DefectDojo engagements the provider user may see, ordered by ID, with the display name `<product> / <engagement>`. " +
				"Imports of scan results can create many engagements: filter by `product_id`. " +
				"As after an import of an engagement, the tags of its product, which DefectDojo may add to it, are left out of `tags`." + listNotes,
			Attributes: map[string]schema.Attribute{
				"product_id":    listIDAttribute("Only list the engagements of the product with this ID."),
				"name_contains": listContainsAttribute("engagements", "name"),
			},
		},
		objects: "engagements",
		all: func(ctx context.Context, client *defectdojo.Client, config EngagementListModel) iter.Seq2[*defectdojo.Engagement, error] {
			return client.Engagements.ListAll(ctx, &defectdojo.ListEngagementsOptions{Product: int(config.ProductID.ValueInt64())})
		},
		id: func(e *defectdojo.Engagement) int { return e.ID },
		// DefectDojo's name filter is exact, so names are matched here.
		keep: func(e *defectdojo.Engagement, config EngagementListModel) bool {
			return containsFold(e.Name, config.NameContains)
		},
		results: engagementResults,
		failure: func(err error, config EngagementListModel) diag.Diagnostics {
			// The product filter rejects products that don't exist with a 400.
			var errResp *defectdojo.ErrorResponse
			if errors.As(err, &errResp) && errResp.StatusCode == http.StatusBadRequest && len(errResp.Fields["product"]) > 0 {
				var diags diag.Diagnostics
				diags.AddError("DefectDojo product not found",
					fmt.Sprintf("There is no product with the ID %d, or the provider user may not see it.", config.ProductID.ValueInt64()))
				return diags
			}
			return listError("engagements")(err)
		},
	}
}

// engagementResults builds the results of engagements as Read does after an
// import: the tags of an engagement's product are hidden. Each product is
// read once, for its tags and its name.
func engagementResults(ctx context.Context, client *defectdojo.Client, req list.ListRequest, _ EngagementListModel) func(*defectdojo.Engagement) list.ListResult {
	products := map[int]*defectdojo.Product{} // nil for a product that is gone
	product := func(id int) (*defectdojo.Product, error) {
		if p, ok := products[id]; ok {
			return p, nil
		}
		p, _, err := client.Products.Get(ctx, id, nil)
		if err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
			return nil, err
		}
		products[id] = p
		return p, nil
	}
	return func(e *defectdojo.Engagement) list.ListResult {
		p, err := product(e.Product)
		if err != nil {
			var diags diag.Diagnostics
			diags.AddError("Error listing DefectDojo engagements",
				fmt.Sprintf("Could not read product %d, for the tags it passes on to engagement %d: %s", e.Product, e.ID, err))
			return errorResult(diags)
		}
		productName, productTags := strconv.Itoa(e.Product), []string(nil)
		if p != nil {
			productName, productTags = p.Name, p.Tags
		}
		var state EngagementResourceModel
		noTags := types.SetNull(types.StringType)
		updateEngagementModel(&state, e, visibleTags(e.Tags, productTags, noTags), noTags)
		return newListResult(ctx, req, productName+" / "+e.Name, e.ID, state)
	}
}
