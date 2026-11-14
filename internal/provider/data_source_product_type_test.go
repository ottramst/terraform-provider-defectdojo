package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccProductTypeDataSource(t *testing.T) {
	name := randName(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "defectdojo_product_type" "test" {
  name             = %[1]q
  description      = "lookup"
  critical_product = true
}

data "defectdojo_product_type" "by_id" {
  id = defectdojo_product_type.test.id
}

data "defectdojo_product_type" "by_name" {
  name = defectdojo_product_type.test.name
}
`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.defectdojo_product_type.by_id", tfjsonpath.New("name"),
						productTypeAddress, tfjsonpath.New("name"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_product_type.by_name", tfjsonpath.New("id"),
						productTypeAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_product_type.by_name", tfjsonpath.New("description"), knownvalue.StringExact("lookup")),
					statecheck.ExpectKnownValue("data.defectdojo_product_type.by_name", tfjsonpath.New("critical_product"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("data.defectdojo_product_type.by_name", tfjsonpath.New("authorized_users"), knownvalue.SetSizeExact(0)),
				},
			},
			{
				Config: fmt.Sprintf(`
data "defectdojo_product_type" "missing" {
  name = %q
}
`, name+"-missing"),
				ExpectError: regexp.MustCompile(`DefectDojo product type not found`),
			},
			{
				Config: `
data "defectdojo_product_type" "both" {
  id   = 1
  name = "Research and Development"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			// The fixture product type has a null description. A valid config last,
			// for the destroy after the test.
			{
				Config: `
data "defectdojo_product_type" "fixture" {
  id = 1
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.defectdojo_product_type.fixture", tfjsonpath.New("name"), knownvalue.StringExact("Research and Development")),
					statecheck.ExpectKnownValue("data.defectdojo_product_type.fixture", tfjsonpath.New("description"), knownvalue.Null()),
				},
			},
		},
	})
}
