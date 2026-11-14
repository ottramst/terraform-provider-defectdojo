package provider

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// Updates send tags only when they are configured, and status and type only
// when the plan changes them, so that an engagement closed elsewhere isn't
// reopened by an unrelated change. Runs against a fake DefectDojo, to check
// the requests.
func TestAccEngagement_statusAndTagsNotSentUnlessConfiguredOrChanged(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "engagements",
		defaults: map[string]any{
			"status": "Not Started", "engagement_type": "Interactive", "active": true, "tags": []any{},
		},
	}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	config := func(description, extra string) string {
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %[1]q
  api_token = "test"
}

resource "defectdojo_engagement" "test" {
  product_id   = 1
  name         = "fake"
  target_start = "2026-01-01"
  target_end   = "2099-12-31"
  description  = %[2]q
%[3]s}
`, server.URL, description, extra)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("one", `
  status = "In Progress"
  tags   = ["a"]
`),
			},
			// Configured but unchanged: tags are sent, status isn't.
			{
				Config: config("two", `
  status = "In Progress"
  tags   = ["a"]
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					fake.checkPatchSends(true, "tags"),
					fake.checkPatchSends(false, "status", "engagement_type"),
				),
			},
			// Changed: status is sent.
			{
				Config: config("three", `
  status          = "On Hold"
  engagement_type = "CI/CD"
  tags            = ["a"]
`),
				Check: fake.checkPatchSends(true, "status", "engagement_type", "tags"),
			},
			// Closed elsewhere, and status and tags not configured: neither is
			// sent, and the changes made elsewhere are kept.
			{
				PreConfig: func() {
					fake.set("status", "Completed")
					fake.set("active", false)
					fake.set("tags", []any{"a", "b"})
				},
				Config: config("four", `  engagement_type = "CI/CD"`+"\n"),
				Check:  fake.checkPatchSends(false, "status", "engagement_type", "tags"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("status"), knownvalue.StringExact("Completed")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("active"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("tags"), knownvalue.SetSizeExact(2)),
				},
			},
		},
	})
}
