package provider

import (
	"fmt"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// The JSON fields are sent as objects, and as null when removed; the project
// key and component as "" when removed, since DefectDojo can't store null.
// An existing Jira project of the product blocks a create. Runs against a
// fake DefectDojo, to check the requests.
func TestAccJiraProject_requests(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "jira_projects",
		defaults: map[string]any{
			"epic_issue_type_name": "Epic", "enabled": true, "push_all_issues": false, "enable_engagement_epic_mapping": false,
			"push_notes": false, "add_vulnerability_id_to_jira_label": false, "product_jira_sla_notification": false,
			"suppress_sla_notification_comments": false, "risk_acceptance_expiration_notification": false,
		},
	}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	config := func(name, body string) string {
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %q
  api_token = "test"
}

resource "defectdojo_jira_project" %q {
  jira_instance_id = 1
  product_id       = 2
%s}
`, server.URL, name, body)
	}
	checkBody := func(last func() map[string]any, field string, want any) resource.TestCheckFunc {
		return func(*terraform.State) error {
			body := last()
			got, sent := body[field]
			if !sent || fmt.Sprint(got) != fmt.Sprint(want) {
				return fmt.Errorf("%s = %v (sent %v), want %v", field, got, sent, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("test", `  project_key   = "SEC"
  custom_fields = jsonencode({ customfield_1 = "x" })
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkBody(fake.lastPost, "custom_fields", map[string]any{"customfield_1": "x"}),
					checkBody(fake.lastPost, "project_key", "SEC"),
				),
			},
			{
				Config: config("test", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkBody(fake.lastPatch, "custom_fields", nil),
					checkBody(fake.lastPatch, "project_key", ""),
					checkBody(fake.lastPatch, "component", ""),
				),
			},
			// A second Jira project of the product is refused before a POST.
			{
				Config: config("test", "") + `
resource "defectdojo_jira_project" "second" {
  jira_instance_id = 1
  product_id       = 2
  depends_on       = [defectdojo_jira_project.test]
}
`,
				ExpectError: regexp.MustCompile(`product 2 already has a Jira project`),
			},
			{
				Config: config("test", ""),
			},
		},
	})
}
