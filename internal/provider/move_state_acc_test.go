package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/ottramst/go-defectdojo"
)

// doximityProviders runs doximity/defectdojo v0.0.13 under the local name
// defectdojo, configured from the environment like this provider; it reads
// the URL from DEFECTDOJO_BASEURL. doximity/defectdojo is unmaintained: when a
// supported DefectDojo version breaks it, the tests that run it can go, with
// TestUpgradeDoximityState and TestMoveFromDoximity left to cover the
// migration.
func doximityProviders(t *testing.T) map[string]resource.ExternalProvider {
	t.Helper()
	t.Setenv("DEFECTDOJO_BASEURL", os.Getenv("DEFECTDOJO_URL"))
	return map[string]resource.ExternalProvider{
		"defectdojo": {Source: "doximity/defectdojo", VersionConstraint: "0.0.13"},
	}
}

// testAccMigrationGuideConfig returns a configuration of the doximity
// migration guide (examples/guides/migrating-from-doximity), with names made
// unique and the Jira instance jiraInstance.
func testAccMigrationGuideConfig(t *testing.T, file, prefix string, jiraInstance int) string {
	t.Helper()
	b, err := os.ReadFile("../../examples/guides/migrating-from-doximity/" + file)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReplacer(
		`"Web applications"`, strconv.Quote(prefix+"-web"),
		`"Shop"`, strconv.Quote(prefix+"-shop"),
		`jira_instance_id = "1"`, fmt.Sprintf(`jira_instance_id = "%d"`, jiraInstance),
		`jira_instance_id = 1`, fmt.Sprintf(`jira_instance_id = %d`, jiraInstance),
	).Replace(string(b))
}

// testAccCreateJiraInstance creates a Jira instance that is deleted when the
// test ends.
func testAccCreateJiraInstance(t *testing.T, name string) int {
	t.Helper()
	client := testAccClient(t)
	ji, _, err := client.JiraInstances.Create(context.Background(), &defectdojo.JiraInstanceWrite{
		JiraInstance: defectdojo.JiraInstance{
			ConfigurationName: name, URL: "https://" + name + ".example.invalid", Username: "bot",
			EpicNameID: 10011, OpenStatusKey: 11, CloseStatusKey: 31,
			InfoMappingSeverity: "Lowest", LowMappingSeverity: "Low", MediumMappingSeverity: "Medium",
			HighMappingSeverity: "High", CriticalMappingSeverity: "Highest",
		},
		Password: "fake-token",
	})
	testAccNoError(t, err)
	t.Cleanup(func() {
		if _, err := client.JiraInstances.Delete(context.Background(), ji.ID); err != nil {
			t.Errorf("deleting Jira instance %d: %v", ji.ID, err)
		}
	})
	return ji.ID
}

// The configuration of the migration guide, applied with doximity/defectdojo,
// is taken over by this provider: the product type and the product by
// changing the provider source, the Jira product configuration with a moved
// block. The first plan changes nothing.
func TestAccDoximityMigration(t *testing.T) {
	testAccSkipUnlessAcc(t)
	prefix := randName(t)
	jira := testAccCreateJiraInstance(t, prefix)

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		// Moves between providers need Terraform 1.8+, identities 1.12+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.RequireAbove(tfversion.Version1_12_0),
		},
		Steps: []resource.TestStep{
			{
				ExternalProviders: doximityProviders(t),
				Config:            testAccMigrationGuideConfig(t, "before.tf", prefix, jira),
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccMigrationGuideConfig(t, "after.tf", prefix, jira),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("defectdojo_product_type.web", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("defectdojo_product.shop", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("defectdojo_jira_project.shop", plancheck.ResourceActionNoop),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectIdentityValueMatchesState("defectdojo_product_type.web", tfjsonpath.New("id")),
					statecheck.ExpectIdentityValueMatchesState("defectdojo_product.shop", tfjsonpath.New("id")),
					statecheck.ExpectIdentityValueMatchesState("defectdojo_jira_project.shop", tfjsonpath.New("id")),
					statecheck.ExpectKnownValue("defectdojo_product.shop", tfjsonpath.New("user_records"), knownvalue.Int64Exact(25000)),
					statecheck.ExpectKnownValue("defectdojo_product.shop", tfjsonpath.New("revenue"), knownvalue.StringExact("1000000.00")),
					statecheck.ExpectKnownValue("defectdojo_product.shop", tfjsonpath.New("enable_simple_risk_acceptance"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("defectdojo_jira_project.shop", tfjsonpath.New("project_key"), knownvalue.StringExact("SHOP")),
					statecheck.ExpectKnownValue("defectdojo_jira_project.shop", tfjsonpath.New("jira_instance_id"), knownvalue.Int64Exact(int64(jira))),
				},
			},
		},
	})
}

// A product type that doximity/defectdojo created and that was deleted outside
// Terraform is created again: its state has no identity, which Read must set
// before removing it (#63).
func TestAccDoximityMigration_deleted(t *testing.T) {
	testAccSkipUnlessAcc(t)
	name := randName(t)
	// With no description, doximity/defectdojo plans an update after every
	// apply.
	config := fmt.Sprintf(`
resource "defectdojo_product_type" "test" {
  name        = %q
  description = "migrated"
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.RequireAbove(tfversion.Version1_12_0),
		},
		Steps: []resource.TestStep{
			{
				ExternalProviders: doximityProviders(t),
				Config:            config,
			},
			{
				PreConfig: func() {
					client := testAccClient(t)
					pt, _, err := client.ProductTypes.GetByName(context.Background(), name)
					testAccNoError(t, err)
					_, err = client.ProductTypes.Delete(context.Background(), pt.ID)
					testAccNoError(t, err)
				},
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("defectdojo_product_type.test", plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}
