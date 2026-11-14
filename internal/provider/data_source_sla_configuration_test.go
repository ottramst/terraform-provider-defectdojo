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

func TestAccSLAConfigurationDataSource(t *testing.T) {
	name := randName(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "defectdojo_sla_configuration" "test" {
  name        = %[1]q
  description = "lookup"
  critical    = 3
  enforce_low = false
}

data "defectdojo_sla_configuration" "by_id" {
  id = defectdojo_sla_configuration.test.id
}

data "defectdojo_sla_configuration" "by_name" {
  name = defectdojo_sla_configuration.test.name
}
`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.defectdojo_sla_configuration.by_id", tfjsonpath.New("name"),
						slaConfigurationAddress, tfjsonpath.New("name"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_sla_configuration.by_name", tfjsonpath.New("id"),
						slaConfigurationAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_sla_configuration.by_name", tfjsonpath.New("description"), knownvalue.StringExact("lookup")),
					statecheck.ExpectKnownValue("data.defectdojo_sla_configuration.by_name", tfjsonpath.New("critical"), knownvalue.Int64Exact(3)),
					statecheck.ExpectKnownValue("data.defectdojo_sla_configuration.by_name", tfjsonpath.New("high"), knownvalue.Int64Exact(30)),
					statecheck.ExpectKnownValue("data.defectdojo_sla_configuration.by_name", tfjsonpath.New("enforce_low"), knownvalue.Bool(false)),
				},
			},
			{
				Config: fmt.Sprintf(`
data "defectdojo_sla_configuration" "missing" {
  name = %q
}
`, name+"-missing"),
				ExpectError: regexp.MustCompile(`DefectDojo SLA configuration not found`),
			},
			{
				Config: `
data "defectdojo_sla_configuration" "both" {
  id   = 1
  name = "Default"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			// The fixtures, looked up by name. A valid config last, for the
			// destroy after the test.
			{
				Config: `
data "defectdojo_sla_configuration" "default" {
  name = "Default"
}

data "defectdojo_sla_configuration" "none" {
  name = "No SLA Enforced"
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.defectdojo_sla_configuration.default", tfjsonpath.New("id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue("data.defectdojo_sla_configuration.default", tfjsonpath.New("enforce_critical"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("data.defectdojo_sla_configuration.none", tfjsonpath.New("enforce_critical"), knownvalue.Bool(false)),
				},
			},
		},
	})
}
