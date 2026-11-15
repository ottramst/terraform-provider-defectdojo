package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/ottramst/go-defectdojo"
)

const jiraProjectAddress = "defectdojo_jira_project.test"

func TestAccJiraProject(t *testing.T) {
	name := randName(t)
	var id, outside int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Resource identity needs Terraform 1.12+, write-only attributes 1.11+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		CheckDestroy: func(*terraform.State) error {
			_, _, err := testAccClient(t).JiraProjects.Get(context.Background(), id, nil)
			if !errors.Is(err, defectdojo.ErrNotFound) {
				return fmt.Errorf("Jira project %d still exists (%v)", id, err)
			}
			return nil
		},
		Steps: []resource.TestStep{
			// Create for a product, with defaults. DefectDojo returns the
			// custom fields with their keys in another order.
			{
				Config: testAccJiraProjectConfig(name, `
  product_id    = defectdojo_product.test.id
  project_key   = "SEC"
  custom_fields = jsonencode({ "customfield_2" = 1, "customfield_1" = [{ name = "8.0.1" }] })
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("project_key"), knownvalue.StringExact("SEC")),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("engagement_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("component"), knownvalue.Null()),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("enabled"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("epic_issue_type_name"), knownvalue.StringExact("Epic")),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("close_transition_fields"), knownvalue.Null()),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("suppress_sla_notification_comments"), knownvalue.Bool(false)),
					statecheck.ExpectIdentityValueMatchesState(jiraProjectAddress, tfjsonpath.New("id")),
				},
				Check: testAccCaptureJiraProjectID(&id),
			},
			// The same custom fields, formatted differently: Terraform plans
			// an update of the text, which leaves no difference after the
			// apply or a refresh.
			{
				Config: testAccJiraProjectConfig(name, `
  product_id    = defectdojo_product.test.id
  project_key   = "SEC"
  custom_fields = "{\"customfield_1\": [{\"name\": \"8.0.1\"}], \"customfield_2\": 1}"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(jiraProjectAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: testAccCheckJiraProjectID(&id),
			},
			// Import with JSON fields: DefectDojo's text of them differs from
			// the configuration's, but not its JSON.
			{
				ResourceName:            jiraProjectAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"custom_fields"},
			},
			// Update every attribute in place.
			{
				Config: testAccJiraProjectConfig(name, `
  product_id                              = defectdojo_product.test.id
  project_key                             = "OPS"
  component                               = "web,api"
  issue_template_dir                      = "issue-trackers/jira_full"
  default_assignee                        = "alice"
  jira_labels                             = "security defectdojo"
  close_transition_fields                 = jsonencode({ resolution = { name = "Won't Fix" } })
  reopen_transition_fields                = jsonencode({ customfield_3 = "reopened" })
  epic_issue_type_name                    = "Initiative"
  enabled                                 = false
  push_all_issues                         = true
  enable_engagement_epic_mapping          = true
  push_notes                              = true
  add_vulnerability_id_to_jira_label      = true
  product_jira_sla_notification           = true
  risk_acceptance_expiration_notification = true
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(jiraProjectAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("project_key"), knownvalue.StringExact("OPS")),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("component"), knownvalue.StringExact("web,api")),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("custom_fields"), knownvalue.Null()),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("epic_issue_type_name"), knownvalue.StringExact("Initiative")),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("enabled"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("product_jira_sla_notification"), knownvalue.Bool(true)),
				},
				Check: testAccCheckJiraProjectID(&id),
			},
			// Removing the text attributes clears them: "" for the key and
			// the component, which aren't nullable.
			{
				Config: testAccJiraProjectConfig(name, `
  product_id = defectdojo_product.test.id
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("project_key"), knownvalue.Null()),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("component"), knownvalue.Null()),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("default_assignee"), knownvalue.Null()),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("close_transition_fields"), knownvalue.Null()),
				},
			},
			// Import by ID
			{
				ResourceName:      jiraProjectAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				Config: testAccJiraProjectConfig(name, `
  product_id = defectdojo_product.test.id
`),
				ResourceName:    jiraProjectAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// The configuration -generate-config-out writes for an import plans no
			// changes. terraform query writes it from the list resource's state,
			// which TestListResourceStates checks is the same.
			{
				Config: testAccJiraProjectConfig(name, `
  product_id = defectdojo_product.test.id
`),
				ResourceName:    jiraProjectAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				GenerateConfig:  true,
			},
			// Changed outside Terraform: the change is reverted.
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).JiraProjects.Patch(context.Background(), id, &defectdojo.JiraProjectPatch{
						ProjectKey: new("CHANGED"), PushNotes: new(true),
					}, nil)
					testAccNoError(t, err)
				},
				Config: testAccJiraProjectConfig(name, `
  product_id = defectdojo_product.test.id
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(jiraProjectAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("project_key"), knownvalue.Null()),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("push_notes"), knownvalue.Bool(false)),
				},
			},
			// A second Jira project for the product is refused.
			{
				Config: testAccJiraProjectConfig(name, `
  product_id = defectdojo_product.test.id
`) + `
resource "defectdojo_jira_project" "second" {
  jira_instance_id = defectdojo_jira_instance.test.id
  product_id       = defectdojo_product.test.id
  project_key      = "SEC2"
  depends_on       = [defectdojo_jira_project.test]
}
`,
				ExpectError: regexp.MustCompile(`already has a Jira project \(ID \d+\); import it`),
			},
			// The Jira instance can't be destroyed while a Jira project made
			// outside Terraform uses it.
			{
				PreConfig: func() { outside = testAccCreateJiraProjectOutside(t, name) },
				Config: testAccJiraProjectConfig(name, `
  product_id = defectdojo_product.test.id
`),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`1 Jira project uses it`),
			},
			// Without it, everything is created again.
			{
				PreConfig: func() {
					_, err := testAccClient(t).JiraProjects.Delete(context.Background(), outside, nil)
					testAccNoError(t, err)
				},
				Config: testAccJiraProjectConfig(name, `
  product_id = defectdojo_product.test.id
`),
				Check: testAccCaptureJiraProjectID(&id),
			},
			// Deleted outside Terraform: re-created.
			{
				PreConfig: func() {
					_, err := testAccClient(t).JiraProjects.Delete(context.Background(), id, nil)
					testAccNoError(t, err)
				},
				Config: testAccJiraProjectConfig(name, `
  product_id = defectdojo_product.test.id
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(jiraProjectAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureJiraProjectID(&id),
			},
		},
	})
}

// testAccCreateJiraProjectOutside creates, outside Terraform, a Jira project
// that uses the test's Jira instance, for a product of a product type that is
// deleted when the test ends, and returns its ID.
func testAccCreateJiraProjectOutside(t *testing.T, name string) int {
	t.Helper()
	ctx := context.Background()
	client := testAccClient(t)
	instance, _, err := client.JiraInstances.GetByName(ctx, name, nil)
	testAccNoError(t, err)
	product, _, err := client.Products.Create(ctx, &defectdojo.Product{
		Name: name + "-outside", Description: "acceptance test", ProdType: testAccCreateProductType(t),
	}, nil)
	testAccNoError(t, err)
	p, _, err := client.JiraProjects.Create(ctx, &defectdojo.JiraProject{JiraInstance: instance.ID, Product: product.ID, ProjectKey: "OUT", Enabled: true}, nil)
	testAccNoError(t, err)
	return p.ID
}

// An engagement's Jira project, and the guards: the SLA notification
// settings, suppress_sla_notification_comments on older DefectDojo, and the
// Jira instance in use.
func TestAccJiraProject_engagement(t *testing.T) {
	name := randName(t)
	engagement := `
resource "defectdojo_engagement" "test" {
  name         = %q
  product_id   = defectdojo_product.test.id
  target_start = "2026-01-01"
  target_end   = "2026-12-31"
}
`
	config := func(extra string) string {
		return testAccJiraProjectConfig(name, `
  engagement_id = defectdojo_engagement.test.id
  project_key   = "ENG"
`+extra) + fmt.Sprintf(engagement, name)
	}
	testAccSkipUnlessAcc(t)
	version, _, err := testAccClient(t).ServerVersion(context.Background(), nil)
	testAccNoError(t, err)
	suppress := resource.TestStep{
		Config: config(`  suppress_sla_notification_comments = true
`),
		ConfigStateChecks: []statecheck.StateCheck{
			statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("suppress_sla_notification_comments"), knownvalue.Bool(true)),
		},
	}
	if compareVersions(version, suppressSLANotificationCommentsSince) < 0 {
		suppress = resource.TestStep{
			Config: config(`  suppress_sla_notification_comments = true
`),
			ExpectError: regexp.MustCompile(`needs DefectDojo 3\.5\.103 or later`),
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		Steps: []resource.TestStep{
			{
				Config: config(""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("product_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(jiraProjectAddress, tfjsonpath.New("project_key"), knownvalue.StringExact("ENG")),
				},
			},
			{
				Config: config(`  product_jira_sla_notification      = true
  suppress_sla_notification_comments = true
`),
				ExpectError: regexp.MustCompile(`Conflicting SLA notification settings`),
			},
			// An engagement that doesn't exist.
			{
				Config: testAccJiraProjectConfig(name, `
  engagement_id = 999999999
`) + fmt.Sprintf(engagement, name),
				ExpectError: regexp.MustCompile(`engagement 999999999 doesn't exist`),
			},
			suppress,
			{
				Config: config(""),
			},
			{
				ResourceName:      jiraProjectAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccJiraProjectConfig(name, body string) string {
	return fmt.Sprintf(`
resource "defectdojo_jira_instance" "test" {
  configuration_name        = %[1]q
  url                       = "https://%[1]s.example.invalid"
  username                  = "bot"
  password_wo               = "fake-token"
  epic_name_id              = 10011
  open_status_key           = 11
  close_status_key          = 31
  info_mapping_severity     = "Lowest"
  low_mapping_severity      = "Low"
  medium_mapping_severity   = "Medium"
  high_mapping_severity     = "High"
  critical_mapping_severity = "Highest"
}

resource "defectdojo_product_type" "test" {
  name          = %[1]q
  force_destroy = true
}

resource "defectdojo_product" "test" {
  name            = %[1]q
  description     = "acceptance test"
  product_type_id = defectdojo_product_type.test.id
  force_destroy   = true
}

resource "defectdojo_jira_project" "test" {
  jira_instance_id = defectdojo_jira_instance.test.id
%[2]s}
`, name, body)
}

// testAccCaptureJiraProjectID stores the ID of the Jira project in id.
func testAccCaptureJiraProjectID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[jiraProjectAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", jiraProjectAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckJiraProjectID checks that the Jira project still has the ID id:
// it was updated in place.
func testAccCheckJiraProjectID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var got int
		if err := testAccCaptureJiraProjectID(&got)(s); err != nil {
			return err
		}
		if got != *id {
			return fmt.Errorf("Jira project ID = %d, want %d", got, *id)
		}
		return nil
	}
}
