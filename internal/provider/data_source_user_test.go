package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

func TestAccUserDataSource(t *testing.T) {
	username := randName(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig(username, `
  first_name                = "Lookup"
  configuration_permissions = ["add_product_type"]
`) + `
data "defectdojo_user" "by_id" {
  id = defectdojo_user.test.id
}

data "defectdojo_user" "by_username" {
  username = defectdojo_user.test.username
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.defectdojo_user.by_id", tfjsonpath.New("username"),
						userAddress, tfjsonpath.New("username"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_user.by_username", tfjsonpath.New("id"),
						userAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_user.by_username", tfjsonpath.New("date_joined"),
						userAddress, tfjsonpath.New("date_joined"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_user.by_username", tfjsonpath.New("first_name"), knownvalue.StringExact("Lookup")),
					statecheck.ExpectKnownValue("data.defectdojo_user.by_username", tfjsonpath.New("last_name"), knownvalue.Null()),
					statecheck.ExpectKnownValue("data.defectdojo_user.by_username", tfjsonpath.New("configuration_permissions"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("add_product_type")})),
					statecheck.ExpectKnownValue("data.defectdojo_user.by_username", tfjsonpath.New("last_login"), knownvalue.Null()),
				},
			},
			// Usernames are compared exactly, case included.
			{
				Config: fmt.Sprintf(`
data "defectdojo_user" "missing" {
  username = %q
}
`, strings.ToUpper(username)),
				ExpectError: regexp.MustCompile(`DefectDojo user not found`),
			},
			{
				Config: `
data "defectdojo_user" "both" {
  id       = 1
  username = "admin"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			// A valid config last, for the destroy after the test.
			{
				Config: `
data "defectdojo_user" "admin" {
  username = "admin"
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.defectdojo_user.admin", tfjsonpath.New("id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue("data.defectdojo_user.admin", tfjsonpath.New("is_superuser"), knownvalue.Bool(true)),
				},
			},
		},
	})
}
