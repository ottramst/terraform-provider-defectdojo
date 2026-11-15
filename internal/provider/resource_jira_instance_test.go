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
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/ottramst/go-defectdojo"
)

const jiraInstanceAddress = "defectdojo_jira_instance.test"

func TestAccJiraInstance(t *testing.T) {
	name := randName(t)
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Resource identity needs Terraform 1.12+, write-only attributes 1.11+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		CheckDestroy: func(*terraform.State) error {
			_, _, err := testAccClient(t).JiraInstances.Get(context.Background(), id)
			if !errors.Is(err, defectdojo.ErrNotFound) {
				return fmt.Errorf("Jira instance %d still exists (%v)", id, err)
			}
			return nil
		},
		Steps: []resource.TestStep{
			// Create with defaults. DefectDojo doesn't contact Jira.
			{
				Config: testAccJiraInstanceConfig(name, "https://"+name+".example.invalid", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("configuration_name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("default_issue_type"), knownvalue.StringExact("Bug")),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("global_jira_sla_notification"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("finding_jira_sync"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("finding_text"), knownvalue.Null()),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("password_wo"), knownvalue.Null()),
					statecheck.ExpectIdentityValueMatchesState(jiraInstanceAddress, tfjsonpath.New("id")),
				},
				Check: testAccCaptureJiraInstanceID(&id),
			},
			// Update every attribute, with a new password version.
			{
				Config: testAccJiraInstanceConfig(name+"-renamed", "https://"+name+".example.invalid", `
  password_wo_version               = 1
  default_issue_type                = "Security"
  issue_template_dir                = "issue-trackers/jira_full"
  finding_text                      = "Reported by DefectDojo"
  accepted_mapping_resolution       = "Won't Fix"
  false_positive_mapping_resolution = "Not a Bug"
  global_jira_sla_notification      = false
  finding_jira_sync                 = true
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(jiraInstanceAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("configuration_name"), knownvalue.StringExact(name+"-renamed")),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("default_issue_type"), knownvalue.StringExact("Security")),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("finding_text"), knownvalue.StringExact("Reported by DefectDojo")),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("false_positive_mapping_resolution"), knownvalue.StringExact("Not a Bug")),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("global_jira_sla_notification"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("password_wo_version"), knownvalue.Int64Exact(1)),
				},
			},
			// The keys and severity mappings change in place.
			{
				Config: testAccJiraInstanceConfig(name+"-renamed", "https://"+name+".example.invalid", `
  password_wo_version = 1
`, "10012", "12", "32", "Low"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("epic_name_id"), knownvalue.Int64Exact(10012)),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("open_status_key"), knownvalue.Int64Exact(12)),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("close_status_key"), knownvalue.Int64Exact(32)),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("info_mapping_severity"), knownvalue.StringExact("Low")),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("critical_mapping_severity"), knownvalue.StringExact("Critical")),
				},
				Check: testAccCheckJiraInstanceID(&id),
			},
			// A trailing slash, which DefectDojo doesn't count as a URL
			// change, needs no password.
			{
				Config: testAccJiraInstanceConfig(name+"-renamed", "https://"+name+".example.invalid/", `
  password_wo_version = 1
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("url"), knownvalue.StringExact("https://"+name+".example.invalid/")),
				},
			},
			// A URL change sends the password, without a version change,
			// which DefectDojo requires.
			{
				Config: testAccJiraInstanceConfig(name+"-renamed", "https://other-"+name+".example.invalid", `
  password_wo_version = 1
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("url"), knownvalue.StringExact("https://other-"+name+".example.invalid")),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("finding_text"), knownvalue.Null()),
				},
			},
			// Import by ID
			{
				ResourceName:            jiraInstanceAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password_wo_version"},
			},
			// Import by identity. The configuration needs the password, but
			// no version, which an imported instance doesn't have.
			{
				Config:          testAccJiraInstanceConfig(name+"-renamed", "https://other-"+name+".example.invalid", ""),
				ResourceName:    jiraInstanceAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// The configuration -generate-config-out writes for an import lacks the
			// required password_wo, which DefectDojo never returns.
			{
				Config:          testAccJiraInstanceConfig(name+"-renamed", "https://other-"+name+".example.invalid", ""),
				ResourceName:    jiraInstanceAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				GenerateConfig:  true,
				ExpectError:     regexp.MustCompile(`password_wo`),
			},
			// Changed outside Terraform: the change is reverted.
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).JiraInstances.Patch(context.Background(), id, &defectdojo.JiraInstancePatch{
						Username: new("changed"), FindingJiraSync: new(true),
					})
					testAccNoError(t, err)
				},
				Config: testAccJiraInstanceConfig(name+"-renamed", "https://other-"+name+".example.invalid", `
  password_wo_version = 1
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(jiraInstanceAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("username"), knownvalue.StringExact("bot")),
					statecheck.ExpectKnownValue(jiraInstanceAddress, tfjsonpath.New("finding_jira_sync"), knownvalue.Bool(false)),
				},
			},
			// Deleted outside Terraform: re-created.
			{
				PreConfig: func() {
					_, err := testAccClient(t).JiraInstances.Delete(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: testAccJiraInstanceConfig(name+"-renamed", "https://other-"+name+".example.invalid", `
  password_wo_version = 1
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(jiraInstanceAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureJiraInstanceID(&id),
			},
		},
	})
}

func TestAccJiraInstanceDataSource(t *testing.T) {
	name := randName(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccJiraInstanceConfig(name, "https://"+name+".example.invalid", `
  finding_text = "Reported by DefectDojo"
`) + `
data "defectdojo_jira_instance" "by_id" {
  id = defectdojo_jira_instance.test.id
}

data "defectdojo_jira_instance" "by_name" {
  configuration_name = defectdojo_jira_instance.test.configuration_name
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.defectdojo_jira_instance.by_name", tfjsonpath.New("id"), jiraInstanceAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_jira_instance.by_id", tfjsonpath.New("configuration_name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue("data.defectdojo_jira_instance.by_id", tfjsonpath.New("finding_text"), knownvalue.StringExact("Reported by DefectDojo")),
					statecheck.ExpectKnownValue("data.defectdojo_jira_instance.by_name", tfjsonpath.New("critical_mapping_severity"), knownvalue.StringExact("Highest")),
				},
			},
		},
	})
}

// testAccJiraInstanceConfig returns a Jira instance's configuration. The
// optional keys are the epic name ID, the open and close status keys, and the
// info severity mapping, which also changes the critical one to "Critical".
func testAccJiraInstanceConfig(name, url, extra string, keys ...string) string {
	epic, open, closing, info, critical := "10011", "11", "31", "Lowest", "Highest"
	if len(keys) == 4 {
		epic, open, closing, info, critical = keys[0], keys[1], keys[2], keys[3], "Critical"
	}
	return fmt.Sprintf(`
resource "defectdojo_jira_instance" "test" {
  configuration_name        = %q
  url                       = %q
  username                  = "bot"
  password_wo               = "fake-token"
  epic_name_id              = %s
  open_status_key           = %s
  close_status_key          = %s
  info_mapping_severity     = %q
  low_mapping_severity      = "Low"
  medium_mapping_severity   = "Medium"
  high_mapping_severity     = "High"
  critical_mapping_severity = %q
%s}
`, name, url, epic, open, closing, info, critical, extra)
}

// testAccCheckJiraInstanceID checks that the Jira instance still has the ID
// id: it was updated in place.
func testAccCheckJiraInstanceID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var got int
		if err := testAccCaptureJiraInstanceID(&got)(s); err != nil {
			return err
		}
		if got != *id {
			return fmt.Errorf("Jira instance ID = %d, want %d", got, *id)
		}
		return nil
	}
}

// testAccCaptureJiraInstanceID stores the ID of the Jira instance in id.
func testAccCaptureJiraInstanceID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[jiraInstanceAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", jiraInstanceAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}
