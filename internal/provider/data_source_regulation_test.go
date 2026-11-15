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

func TestAccRegulationDataSource(t *testing.T) {
	name := randName(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRegulationConfig(name, "medical", `  description = "lookup"
`) + `
data "defectdojo_regulation" "by_id" {
  id = defectdojo_regulation.test.id
}

data "defectdojo_regulation" "by_name" {
  name = defectdojo_regulation.test.name
}

data "defectdojo_regulation" "by_acronym" {
  acronym = defectdojo_regulation.test.acronym
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.defectdojo_regulation.by_id", tfjsonpath.New("name"),
						regulationAddress, tfjsonpath.New("name"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_regulation.by_name", tfjsonpath.New("id"),
						regulationAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_regulation.by_acronym", tfjsonpath.New("id"),
						regulationAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_regulation.by_acronym", tfjsonpath.New("category"), knownvalue.StringExact("medical")),
					statecheck.ExpectKnownValue("data.defectdojo_regulation.by_acronym", tfjsonpath.New("description"), knownvalue.StringExact("lookup")),
					statecheck.ExpectKnownValue("data.defectdojo_regulation.by_acronym", tfjsonpath.New("reference"), knownvalue.Null()),
				},
			},
			{
				Config: fmt.Sprintf(`
data "defectdojo_regulation" "missing" {
  acronym = %q
}
`, name+"-missing"),
				ExpectError: regexp.MustCompile(`DefectDojo regulation not found`),
			},
			{
				Config: `
data "defectdojo_regulation" "both" {
  name    = "Health Insurance Portability and Accountability Act"
  acronym = "HIPAA"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			// The fixtures, looked up by acronym. A valid config last, for
			// the destroy after the test.
			{
				Config: `
data "defectdojo_regulation" "pci" {
  acronym = "PCI DSS"
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.defectdojo_regulation.pci", tfjsonpath.New("id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue("data.defectdojo_regulation.pci", tfjsonpath.New("name"),
						knownvalue.StringExact("Payment Card Industry Data Security Standard")),
					statecheck.ExpectKnownValue("data.defectdojo_regulation.pci", tfjsonpath.New("category"), knownvalue.StringExact("finance")),
				},
			},
		},
	})
}
