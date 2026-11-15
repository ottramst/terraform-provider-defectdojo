package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/ottramst/go-defectdojo"
)

func TestAccDevelopmentEnvironmentDataSource(t *testing.T) {
	name := randName(t)
	var twinID int
	t.Cleanup(func() {
		if twinID != 0 {
			_, err := testAccClient(t).DevelopmentEnvironments.Delete(context.Background(), twinID, nil)
			if err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
				t.Logf("deleting development environment %d: %v", twinID, err)
			}
		}
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDevelopmentEnvironmentConfig(name) + `
data "defectdojo_development_environment" "by_id" {
  id = defectdojo_development_environment.test.id
}

data "defectdojo_development_environment" "by_name" {
  name = defectdojo_development_environment.test.name
}

data "defectdojo_development_environment" "seeded" {
  name = "Production"
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.defectdojo_development_environment.by_id", tfjsonpath.New("name"),
						developmentEnvironmentAddress, tfjsonpath.New("name"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_development_environment.by_name", tfjsonpath.New("id"),
						developmentEnvironmentAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_development_environment.seeded", tfjsonpath.New("id"), knownvalue.NotNull()),
				},
			},
			{
				Config: fmt.Sprintf(`
data "defectdojo_development_environment" "missing" {
  name = %q
}
`, name+"-missing"),
				ExpectError: regexp.MustCompile(`DefectDojo development environment not found`),
			},
			// A second environment with the name, created outside Terraform:
			// the lookup by name fails.
			{
				PreConfig: func() {
					twin, _, err := testAccClient(t).DevelopmentEnvironments.Create(context.Background(), &defectdojo.DevelopmentEnvironment{Name: name}, nil)
					testAccNoError(t, err)
					twinID = twin.ID
				},
				Config: testAccDevelopmentEnvironmentConfig(name) + `
data "defectdojo_development_environment" "by_name" {
  name = defectdojo_development_environment.test.name
}
`,
				ExpectError: regexp.MustCompile(`(?s)2\s+development\s+environments\s+are\s+named`),
			},
			// A valid config last, for the destroy after the test.
			{
				PreConfig: func() {
					_, err := testAccClient(t).DevelopmentEnvironments.Delete(context.Background(), twinID, nil)
					testAccNoError(t, err)
				},
				Config: testAccDevelopmentEnvironmentConfig(name),
			},
		},
	})
}
