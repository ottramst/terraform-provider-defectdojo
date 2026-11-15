package provider

import (
	"context"
	"iter"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

// ProductListModel describes the configuration of the defectdojo_product list
// resource.
type ProductListModel struct {
	ProductTypeID types.Int64  `tfsdk:"product_type_id"`
	NameContains  types.String `tfsdk:"name_contains"`
}

func NewProductListResource() list.ListResource {
	id := func(p *defectdojo.Product) int { return p.ID }
	return &listResource[ProductListModel, defectdojo.Product]{
		typeName: "_product",
		schema: schema.Schema{
			MarkdownDescription: "Lists the DefectDojo products the provider user may see, ordered by name. " +
				"The generated configuration sets `authorized_users`, `regulation_ids`, `tags` and `sla_configuration_id`." + listNotes,
			Attributes: map[string]schema.Attribute{
				"product_type_id": listIDAttribute("Only list the products of the product type with this ID. A product type that doesn't exist lists nothing."),
				"name_contains":   listContainsAttribute("products", "name"),
			},
		},
		objects: "products",
		// Both filters are DefectDojo's: its name filter matches a part of
		// the name, ignoring case.
		all: func(ctx context.Context, client *defectdojo.Client, config ProductListModel) iter.Seq2[*defectdojo.Product, error] {
			return client.Products.ListAll(ctx, &defectdojo.ListProductsOptions{
				Name:     config.NameContains.ValueString(),
				ProdType: int(config.ProductTypeID.ValueInt64()),
			})
		},
		id: id,
		results: modelResults[ProductListModel](id,
			func(p *defectdojo.Product) string { return p.Name },
			// As after an import: no tag spelling is known.
			func(m *ProductResourceModel, p *defectdojo.Product) {
				updateProductModel(m, p, types.SetNull(types.StringType))
			}),
	}
}
