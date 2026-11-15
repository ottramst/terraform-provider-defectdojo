package provider

import (
	"context"
	"iter"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// ProductTypeListModel describes the configuration of the
// defectdojo_product_type list resource.
type ProductTypeListModel struct {
	NameContains types.String `tfsdk:"name_contains"`
}

func NewProductTypeListResource() list.ListResource {
	id := func(pt *defectdojo.ProductType) int { return pt.ID }
	return &listResource[ProductTypeListModel, defectdojo.ProductType]{
		typeName: "_product_type",
		schema: schema.Schema{
			MarkdownDescription: "Lists the DefectDojo product types the provider user may see, ordered by name." + listNotes,
			Attributes: map[string]schema.Attribute{
				"name_contains": listContainsAttribute("product types", "name"),
			},
		},
		objects: "product types",
		all: func(ctx context.Context, client *defectdojo.Client, _ ProductTypeListModel) iter.Seq2[*defectdojo.ProductType, error] {
			return client.ProductTypes.ListAll(ctx, nil)
		},
		id: id,
		// DefectDojo's name filter is exact, so names are matched here.
		keep: func(pt *defectdojo.ProductType, config ProductTypeListModel) bool {
			return containsFold(pt.Name, config.NameContains)
		},
		results: modelResults[ProductTypeListModel](id,
			func(pt *defectdojo.ProductType) string { return pt.Name },
			updateProductTypeModel),
	}
}
