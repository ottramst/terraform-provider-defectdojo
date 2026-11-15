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

func TestAccCICDInfrastructureDataSource(t *testing.T) {
	name := randName(t)
	objects := testAccCICDInfrastructureConfig(name, "build_server", `  url = "https://ci.example.com"
`) + fmt.Sprintf(`
resource "defectdojo_cicd_infrastructure" "scm" {
  name                = %[1]q
  infrastructure_type = "scm_server"
}

resource "defectdojo_cicd_infrastructure" "unique" {
  name                = "%[1]s-k8s"
  infrastructure_type = "orchestration"
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: objects + `
data "defectdojo_cicd_infrastructure" "by_id" {
  id = defectdojo_cicd_infrastructure.test.id
}

data "defectdojo_cicd_infrastructure" "by_name" {
  name                = defectdojo_cicd_infrastructure.test.name
  infrastructure_type = "build_server"
}

data "defectdojo_cicd_infrastructure" "unique" {
  name = defectdojo_cicd_infrastructure.unique.name
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.defectdojo_cicd_infrastructure.by_id", tfjsonpath.New("name"),
						cicdInfrastructureAddress, tfjsonpath.New("name"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_cicd_infrastructure.by_name", tfjsonpath.New("id"),
						cicdInfrastructureAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_cicd_infrastructure.by_id", tfjsonpath.New("infrastructure_type"), knownvalue.StringExact("build_server")),
					statecheck.ExpectKnownValue("data.defectdojo_cicd_infrastructure.by_id", tfjsonpath.New("url"), knownvalue.StringExact("https://ci.example.com")),
					statecheck.ExpectKnownValue("data.defectdojo_cicd_infrastructure.by_id", tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.CompareValuePairs("data.defectdojo_cicd_infrastructure.unique", tfjsonpath.New("id"),
						"defectdojo_cicd_infrastructure.unique", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_cicd_infrastructure.unique", tfjsonpath.New("infrastructure_type"), knownvalue.StringExact("orchestration")),
				},
			},
			// Two types share the name: the lookup needs the type.
			{
				Config: objects + `
data "defectdojo_cicd_infrastructure" "ambiguous" {
  name = defectdojo_cicd_infrastructure.test.name
}
`,
				ExpectError: regexp.MustCompile(`give\s+the\s+type`),
			},
			{
				Config: objects + fmt.Sprintf(`
data "defectdojo_cicd_infrastructure" "missing" {
  name = %q
}
`, name+"-missing"),
				ExpectError: regexp.MustCompile(`DefectDojo CI/CD infrastructure not found`),
			},
			{
				Config: objects + `
data "defectdojo_cicd_infrastructure" "both" {
  id                  = 1
  infrastructure_type = "scm_server"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			// A valid config last, for the destroy after the test.
			{
				Config: objects,
			},
		},
	})
}
