package provider

import (
	"fmt"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// The password is sent on create, and on an update only when its version or
// the URL changes; a trailing slash isn't a change. A delete is refused while
// Jira projects use the instance. Runs against a fake DefectDojo, to check
// the requests.
func TestAccJiraInstance_passwordSentOnlyWhenNeeded(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "jira_instances",
		defaults:   map[string]any{"default_issue_type": "Bug", "global_jira_sla_notification": true, "finding_jira_sync": false},
	}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	config := func(url, password, extra string) string {
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %[1]q
  api_token = "test"
}

resource "defectdojo_jira_instance" "test" {
  configuration_name        = "fake"
  url                       = %[2]q
  username                  = "bot"
  password_wo               = %[3]q
  epic_name_id              = 1
  open_status_key           = 2
  close_status_key          = 3
  info_mapping_severity     = "Lowest"
  low_mapping_severity      = "Low"
  medium_mapping_severity   = "Medium"
  high_mapping_severity     = "High"
  critical_mapping_severity = "Highest"
%[4]s}
`, server.URL, url, password, extra)
	}
	checkPassword := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			body := fake.lastPatch()
			got, sent := body["password"]
			if (want == "") == sent || (sent && got != want) {
				return fmt.Errorf("PATCH password = %v (sent %v), want %q", got, sent, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Write-only attributes need Terraform 1.11+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: config("https://jira.invalid", "one", ""),
				Check:  fake.checkPostSends(true, "password"),
			},
			// Another password with the same version isn't sent.
			{
				Config: config("https://jira.invalid", "ignored", `  finding_jira_sync = true
`),
				Check: checkPassword(""),
			},
			// A new version sends it.
			{
				Config: config("https://jira.invalid", "two", `  finding_jira_sync   = true
  password_wo_version = 1
`),
				Check: checkPassword("two"),
			},
			// A trailing slash isn't a URL change.
			{
				Config: config("https://jira.invalid/", "two", `  finding_jira_sync   = true
  password_wo_version = 1
`),
				Check: checkPassword(""),
			},
			// A URL change sends it.
			{
				Config: config("https://other.invalid", "two", `  finding_jira_sync   = true
  password_wo_version = 1
`),
				Check: checkPassword("two"),
			},
			// A Jira project uses the instance: the delete is refused before
			// DefectDojo would answer 500.
			{
				PreConfig: func() { fake.setList("jira_projects", []map[string]any{{"id": 3, "jira_instance": 7}}) },
				Config: config("https://other.invalid", "two", `  finding_jira_sync   = true
  password_wo_version = 1
`),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`1 Jira projects use it`),
			},
			{
				PreConfig: func() { fake.setList("jira_projects", nil) },
				Config: config("https://other.invalid", "two", `  finding_jira_sync   = true
  password_wo_version = 1
`),
			},
		},
	})
}
