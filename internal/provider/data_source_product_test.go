package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/ottramst/go-defectdojo"
)

func TestAccProductDataSource(t *testing.T) {
	testAccSkipUnlessAcc(t)
	name := randName(t)
	// A product whose name differs only in case: name lookups must not
	// match it, although DefectDojo's name_exact filter ignores case.
	upper, _, err := testAccClient(t).Products.Create(context.Background(), &defectdojo.Product{
		Name: strings.ToUpper(name), Description: "same name, other case", ProdType: testAccCreateProductType(t),
	})
	testAccNoError(t, err)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProductConfig(name, "first", `
  platform           = "web"
  user_records       = 0
  revenue            = "100.00"
  product_manager_id = 1
  tags               = ["lookup"]
`) + `
data "defectdojo_product" "by_id" {
  id = defectdojo_product.test.id
}

data "defectdojo_product" "by_name" {
  name = defectdojo_product.test.name
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.defectdojo_product.by_id", tfjsonpath.New("name"),
						productAddress, tfjsonpath.New("name"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_product.by_name", tfjsonpath.New("id"),
						productAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_product.by_name", tfjsonpath.New("product_type_id"),
						productAddress, tfjsonpath.New("product_type_id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_product.by_name", tfjsonpath.New("description"), knownvalue.StringExact("acceptance test")),
					statecheck.ExpectKnownValue("data.defectdojo_product.by_name", tfjsonpath.New("platform"), knownvalue.StringExact("web")),
					statecheck.ExpectKnownValue("data.defectdojo_product.by_name", tfjsonpath.New("life_cycle"), knownvalue.Null()),
					statecheck.ExpectKnownValue("data.defectdojo_product.by_name", tfjsonpath.New("user_records"), knownvalue.Int64Exact(0)),
					statecheck.ExpectKnownValue("data.defectdojo_product.by_name", tfjsonpath.New("revenue"), knownvalue.StringExact("100.00")),
					statecheck.ExpectKnownValue("data.defectdojo_product.by_name", tfjsonpath.New("product_manager_id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue("data.defectdojo_product.by_name", tfjsonpath.New("sla_configuration_id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue("data.defectdojo_product.by_name", tfjsonpath.New("tags"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("lookup")})),
				},
			},
			{
				Config: fmt.Sprintf(`
data "defectdojo_product" "upper" {
  name = %q
}
`, upper.Name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.defectdojo_product.upper", tfjsonpath.New("id"), knownvalue.Int64Exact(int64(upper.ID))),
				},
			},
			{
				Config: fmt.Sprintf(`
data "defectdojo_product" "missing" {
  name = %q
}
`, name),
				ExpectError: regexp.MustCompile(`DefectDojo product not found`),
			},
			// A valid config last, for the destroy after the test.
			{
				Config: fmt.Sprintf(`
data "defectdojo_product" "upper" {
  id = %d
}
`, upper.ID),
			},
		},
	})
}
