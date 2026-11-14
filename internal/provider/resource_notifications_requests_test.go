package provider

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Settings that DefectDojo created are adopted with a PATCH, not created
// again; the template is never sent in a PATCH; scan_added_empty is sent as
// one string; destroying personal settings resets them with a PATCH. Runs
// against a fake DefectDojo, to check the requests.
func TestAccNotifications_adoptedNotSentUnlessConfigured(t *testing.T) {
	for _, tt := range []struct {
		name, scope string
		object      map[string]any
		wantDelete  bool
	}{
		{name: "personal", scope: "  user_id = 5\n", object: map[string]any{"id": 7, "user": 5, "product": nil, "template": false}},
		{name: "template", scope: "  template = true\n", object: map[string]any{"id": 7, "user": nil, "product": nil, "template": true}, wantDelete: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeDefectDojo{collection: "notifications", object: tt.object}
			server := httptest.NewServer(fake.handler(t))
			t.Cleanup(server.Close)
			config := fmt.Sprintf(`
provider "defectdojo" {
  url       = %[1]q
  api_token = "test"
}

resource "defectdojo_notifications" "test" {
%[2]s  other            = ["mail"]
  scan_added_empty = ["alert"]
}
`, server.URL, tt.scope)

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy: func(*terraform.State) error {
					fake.mu.Lock()
					defer fake.mu.Unlock()
					if deleted := fake.object == nil; deleted != tt.wantDelete {
						return fmt.Errorf("deleted = %t, want %t", deleted, tt.wantDelete)
					}
					if !tt.wantDelete {
						reset := fake.patches[len(fake.patches)-1]
						if fmt.Sprint(reset["other"]) != "[alert]" || reset["scan_added_empty"] != "" {
							return fmt.Errorf("reset PATCH %v, want DefectDojo's defaults", reset)
						}
					}
					return nil
				},
				Steps: []resource.TestStep{
					{
						Config: config,
						Check: func(*terraform.State) error {
							if post := fake.lastPost(); post != nil {
								return fmt.Errorf("POST %v, want the existing settings adopted", post)
							}
							patch := fake.lastPatch()
							if patch == nil {
								return fmt.Errorf("no PATCH was sent")
							}
							for _, field := range []string{"template", "user", "product"} {
								if _, sent := patch[field]; sent {
									return fmt.Errorf("PATCH %v sends %s", patch, field)
								}
							}
							if patch["scan_added_empty"] != "alert" || fmt.Sprint(patch["other"]) != "[mail]" || fmt.Sprint(patch["product_added"]) != "[alert]" {
								return fmt.Errorf("PATCH %v, want every event, scan_added_empty as a string", patch)
							}
							return nil
						},
					},
				},
			})
		})
	}
}
