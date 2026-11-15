package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// upgradeFrom is the latest release, whose states this build must take over
// without changes. The first pull request after each release raises it, with
// the DefectDojo versions that release was tested on: the release pull
// request runs before its tag exists.
const upgradeFrom = "0.5.0"

// upgradeFromDefectDojo are the DefectDojo versions upgradeFrom was tested
// on. On other versions the released provider may not work, so the test is
// skipped: a pull request that adds a DefectDojo version can't fail because
// of an old release.
var upgradeFromDefectDojo = []string{"3.4.304", "3.5.103"}

// upgradeAddresses are the resources of testAccUpgradeConfig.
var upgradeAddresses = []string{
	"defectdojo_product_type.test",
	"defectdojo_sla_configuration.test",
	"defectdojo_user.test",
	"defectdojo_product.test",
	"defectdojo_engagement.test",
	"defectdojo_note_type.test",
	"defectdojo_notification_webhook.test",
	"defectdojo_notifications.test",
	"defectdojo_jira_instance.test",
	"defectdojo_jira_project.test",
	"defectdojo_system_settings.test",
}

// The objects the latest release created plan no changes with this build:
// from v1.0.0, a minor or patch release must take over a state as it is.
func TestAccUpgradeFromLatestRelease(t *testing.T) {
	testAccSkipUnlessAcc(t)
	version, _, err := testAccClient(t).ServerVersion(context.Background(), nil)
	testAccNoError(t, err)
	if !slices.Contains(upgradeFromDefectDojo, version) {
		t.Skipf("provider %s was not tested on DefectDojo %s", upgradeFrom, version)
	}
	// A dev_overrides block in the CLI configuration would run this build in
	// the first step instead of the release.
	cliConfig := filepath.Join(t.TempDir(), "terraformrc")
	if err := os.WriteFile(cliConfig, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TF_CLI_CONFIG_FILE", cliConfig)

	config := testAccUpgradeConfig(randName(t))
	var identities []statecheck.StateCheck
	for _, address := range upgradeAddresses {
		identities = append(identities, statecheck.ExpectIdentityValueMatchesState(address, tfjsonpath.New("id")))
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		// Write-only attributes need Terraform 1.11, identities 1.12.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.RequireAbove(tfversion.Version1_12_0),
		},
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"defectdojo": {Source: "ottramst/defectdojo", VersionConstraint: upgradeFrom},
				},
				Config: config,
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: identities,
			},
		},
	})
}

// testAccUpgradeConfig returns a configuration with the resources whose
// state is the most involved: authorized users, tags, regulations,
// configuration permissions, write-only attributes, notification events, a
// Jira project and the system settings.
func testAccUpgradeConfig(name string) string {
	return fmt.Sprintf(`
resource "defectdojo_product_type" "test" {
  name        = "%[1]s-type"
  description = "Upgrade test"
}

resource "defectdojo_sla_configuration" "test" {
  name     = "%[1]s-sla"
  critical = 3
}

resource "defectdojo_user" "test" {
  username                  = "%[1]s-user"
  password_wo               = %[2]q
  email                     = "%[1]s@example.invalid"
  configuration_permissions = ["add_product_type"]
}

resource "defectdojo_product" "test" {
  name                 = "%[1]s-product"
  description          = "Upgrade test"
  product_type_id      = defectdojo_product_type.test.id
  sla_configuration_id = defectdojo_sla_configuration.test.id
  product_manager_id   = defectdojo_user.test.id
  authorized_users     = [defectdojo_user.test.id]
  tags                 = ["upgrade", "Test"]
  regulation_ids       = [1]
  user_records         = 0
  revenue              = "1000.00"
}

resource "defectdojo_engagement" "test" {
  product_id   = defectdojo_product.test.id
  name         = "%[1]s-engagement"
  target_start = "2026-01-01"
  target_end   = "2099-12-31"
  lead_id      = defectdojo_user.test.id
  tags         = ["pipeline"]
}

resource "defectdojo_note_type" "test" {
  name        = "%[1]s-note"
  description = "Upgrade test"
  is_active   = false
}

resource "defectdojo_notification_webhook" "test" {
  name                    = "%[1]s-webhook"
  url                     = "https://hooks.invalid/upgrade"
  header_name             = "Authorization"
  header_value_wo         = "Bearer upgrade"
  header_value_wo_version = 1
}

resource "defectdojo_notifications" "test" {
  user_id          = defectdojo_user.test.id
  product_id       = defectdojo_product.test.id
  product_added    = ["mail", "alert"]
  scan_added_empty = ["mail"]
}

resource "defectdojo_jira_instance" "test" {
  configuration_name        = "%[1]s-jira"
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

resource "defectdojo_jira_project" "test" {
  jira_instance_id = defectdojo_jira_instance.test.id
  product_id       = defectdojo_product.test.id
  project_key      = "UPG"
  custom_fields    = jsonencode({ customfield_10010 = "x" })
}

# Without settings, nothing is written, and destroying it changes nothing.
resource "defectdojo_system_settings" "test" {}
`, name, testAccPassword)
}
