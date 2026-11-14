package provider

import (
	"context"
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

const systemSettingsAddress = "defectdojo_system_settings.test"

// testAccRestoreSystemSettings restores, when the test ends, the system
// settings that the system settings tests change. The tests never change
// read_only_mode, enable_finding_sla, the password rules or other settings
// that would affect other tests.
func testAccRestoreSystemSettings(t *testing.T) *defectdojo.SystemSettings {
	t.Helper()
	testAccSkipUnlessAcc(t)
	client := testAccClient(t)
	before, _, err := client.SystemSettings.Get(context.Background())
	testAccNoError(t, err)
	t.Cleanup(func() {
		maxDupes := defectdojo.Null[int]()
		if before.MaxDupes != nil {
			maxDupes = defectdojo.Value(*before.MaxDupes)
		}
		_, _, err := client.SystemSettings.Patch(context.Background(), before.ID, &defectdojo.SystemSettingsPatch{
			TeamName:                new(before.TeamName),
			EmailFrom:               new(before.EmailFrom),
			DisclaimerNotes:         new(before.DisclaimerNotes),
			EngagementAutoCloseDays: new(before.EngagementAutoCloseDays),
			MaxDupes:                maxDupes,
			SlackToken:              new(before.SlackToken),
		})
		if err != nil {
			t.Errorf("restoring the system settings: %v", err)
		}
	})
	return before
}

func TestAccSystemSettings(t *testing.T) {
	before := testAccRestoreSystemSettings(t)
	name := randName(t)
	days := before.EngagementAutoCloseDays + 1

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Resource identity needs Terraform 1.12+, write-only attributes 1.11+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		CheckDestroy: testAccCheckSystemSettings(t, func(s *defectdojo.SystemSettings) error {
			if s.TeamName != "" || s.DisclaimerNotes != "acceptance test" {
				return fmt.Errorf("destroy changed the settings: %+v", s)
			}
			return nil
		}),
		Steps: []resource.TestStep{
			// Create changes only the configured settings, and reads the
			// others.
			{
				Config: testAccSystemSettingsConfig(fmt.Sprintf(`
  team_name                  = %q
  disclaimer_notes           = "acceptance test"
  engagement_auto_close_days = %d
  max_dupes                  = 5
`, name, days)),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("id"), knownvalue.Int64Exact(int64(before.ID))),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("team_name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("max_dupes"), knownvalue.Int64Exact(5)),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("engagement_auto_close_days"), knownvalue.Int64Exact(int64(days))),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("email_from"), knownvalue.StringExact(before.EmailFrom)),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("enable_finding_sla"), knownvalue.Bool(before.EnableFindingSLA)),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("minimum_password_length"), knownvalue.Int64Exact(int64(before.MinimumPasswordLength))),
					statecheck.ExpectIdentityValueMatchesState(systemSettingsAddress, tfjsonpath.New("id")),
				},
			},
			// Update, and send the Slack token.
			{
				Config: testAccSystemSettingsConfig(fmt.Sprintf(`
  team_name                  = "%[1]s-renamed"
  disclaimer_notes           = "acceptance test"
  engagement_auto_close_days = %[2]d
  max_dupes                  = 0
  slack_token_wo             = "xoxb-fake-one"
  slack_token_wo_version     = 1
`, name, days)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(systemSettingsAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("team_name"), knownvalue.StringExact(name+"-renamed")),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("max_dupes"), knownvalue.Int64Exact(0)),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("slack_token_wo"), knownvalue.Null()),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("slack_token_wo_version"), knownvalue.Int64Exact(1)),
				},
				Check: testAccCheckSystemSettings(t, func(s *defectdojo.SystemSettings) error {
					if s.SlackToken != "xoxb-fake-one" {
						return fmt.Errorf("SlackToken = %q, want xoxb-fake-one", s.SlackToken)
					}
					return nil
				}),
			},
			// A new value with the same version isn't sent.
			{
				Config: testAccSystemSettingsConfig(fmt.Sprintf(`
  team_name                  = "%[1]s-renamed"
  disclaimer_notes           = "acceptance test"
  engagement_auto_close_days = %[2]d
  max_dupes                  = 0
  slack_token_wo             = "xoxb-fake-ignored"
  slack_token_wo_version     = 1
`, name, days)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			// Removing settings from the configuration changes nothing.
			{
				Config: testAccSystemSettingsConfig(`
  disclaimer_notes       = "acceptance test"
  slack_token_wo         = "xoxb-fake-one"
  slack_token_wo_version = 1
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("team_name"), knownvalue.StringExact(name+"-renamed")),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("max_dupes"), knownvalue.Int64Exact(0)),
				},
			},
			// Removing the Slack token and its version leaves the token in
			// DefectDojo.
			{
				Config: testAccSystemSettingsConfig(`
  disclaimer_notes = "acceptance test"
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("team_name"), knownvalue.StringExact(name+"-renamed")),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("max_dupes"), knownvalue.Int64Exact(0)),
				},
				Check: testAccCheckSystemSettings(t, func(s *defectdojo.SystemSettings) error {
					if s.SlackToken != "xoxb-fake-one" {
						return fmt.Errorf("SlackToken = %q, want it kept", s.SlackToken)
					}
					return nil
				}),
			},
			// "" clears text.
			{
				Config: testAccSystemSettingsConfig(`
  team_name        = ""
  disclaimer_notes = "acceptance test"
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("team_name"), knownvalue.StringExact("")),
				},
			},
			// Import by ID
			{
				ResourceName:            systemSettingsAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"slack_token_wo_version"},
			},
			// Import by identity
			{
				Config: testAccSystemSettingsConfig(`
  team_name        = ""
  disclaimer_notes = "acceptance test"
`),
				ResourceName:    systemSettingsAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// An import of another ID fails.
			{
				ResourceName:  systemSettingsAddress,
				ImportState:   true,
				ImportStateId: "42",
				ExpectError:   regexp.MustCompile(`(?i)non-existent|not found|cannot import`),
			},
			// Changed outside Terraform: a configured setting is changed back,
			// another one is taken over.
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).SystemSettings.Patch(context.Background(), before.ID, &defectdojo.SystemSettingsPatch{
						DisclaimerNotes: new("changed"), EmailFrom: new("changed@example.invalid"),
					})
					testAccNoError(t, err)
				},
				Config: testAccSystemSettingsConfig(`
  team_name        = ""
  disclaimer_notes = "acceptance test"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(systemSettingsAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("disclaimer_notes"), knownvalue.StringExact("acceptance test")),
					statecheck.ExpectKnownValue(systemSettingsAddress, tfjsonpath.New("email_from"), knownvalue.StringExact("changed@example.invalid")),
				},
			},
			// A minimum password length above the maximum, which DefectDojo
			// would accept, is refused.
			{
				Config: testAccSystemSettingsConfig(`
  minimum_password_length = 20
  maximum_password_length = 15
`),
				ExpectError: regexp.MustCompile(`minimum_password_length \(20\) exceeds\s+maximum_password_length \(15\)`),
			},
			{
				Config: testAccSystemSettingsConfig(`
  team_name        = ""
  disclaimer_notes = "acceptance test"
`),
			},
		},
	})
}

func TestAccSystemSettingsDataSource(t *testing.T) {
	before := testAccRestoreSystemSettings(t)
	const address = "data.defectdojo_system_settings.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "defectdojo_system_settings" "test" {}`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("id"), knownvalue.Int64Exact(int64(before.ID))),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("install_id"), knownvalue.StringExact(before.InstallID)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("email_from"), knownvalue.StringExact(before.EmailFrom)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("enable_finding_sla"), knownvalue.Bool(before.EnableFindingSLA)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("product_grade_a"), knownvalue.Int64Exact(int64(before.ProductGradeA))),
					testAccReadOnlyModeCheck(address, before),
				},
			},
		},
	})
}

// testAccReadOnlyModeCheck checks read_only_mode, which DefectDojo has from
// 3.5.100: false, or null before.
func testAccReadOnlyModeCheck(address string, before *defectdojo.SystemSettings) statecheck.StateCheck {
	if before.ReadOnlyMode == nil {
		return statecheck.ExpectKnownValue(address, tfjsonpath.New("read_only_mode"), knownvalue.Null())
	}
	return statecheck.ExpectKnownValue(address, tfjsonpath.New("read_only_mode"), knownvalue.Bool(false))
}

func testAccSystemSettingsConfig(body string) string {
	return "resource \"defectdojo_system_settings\" \"test\" {" + body + "}\n"
}

// testAccCheckSystemSettings checks the system settings in DefectDojo.
func testAccCheckSystemSettings(t *testing.T, check func(*defectdojo.SystemSettings) error) resource.TestCheckFunc {
	return func(*terraform.State) error {
		s, _, err := testAccClient(t).SystemSettings.Get(context.Background())
		if err != nil {
			return err
		}
		return check(s)
	}
}
