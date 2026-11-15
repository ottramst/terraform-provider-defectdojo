package provider

import (
	"fmt"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// An existing announcement blocks a create before any request that writes,
// and a change of dismissable sends only dismissable. Runs against a fake
// DefectDojo, to check the requests.
func TestAccAnnouncement_requests(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "announcements",
		id:         1,
		object:     map[string]any{"id": 1, "message": "Set up elsewhere", "style": "info", "dismissable": false},
	}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	config := func(body string) string {
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %q
  api_token = "test"
}

resource "defectdojo_announcement" "test" {
%s}
`, server.URL, body)
	}
	noWrites := func(*terraform.State) error {
		if n := fake.writeCount(); n != 0 {
			return fmt.Errorf("%d writes, want none", n)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config(`  message = "Maintenance"` + "\n"),
				ExpectError: regexp.MustCompile(`(?s)already\s+has\s+an\s+announcement\s+\(ID\s+1\)`),
			},
			// The import, then a change of dismissable: a PATCH of it alone.
			{
				Config: config(`  message = "Set up elsewhere"`+"\n") + `
import {
  to = defectdojo_announcement.test
  id = "1"
}
`,
				Check: noWrites,
			},
			{
				Config: config(`  message     = "Set up elsewhere"` + "\n" + `  dismissable = true` + "\n"),
				Check:  checkPatchExactly(fake, "dismissable"),
			},
		},
	})
}
