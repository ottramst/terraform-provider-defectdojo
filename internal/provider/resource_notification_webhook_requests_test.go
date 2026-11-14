package provider

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// The header value is sent on create and when its version changes, and only
// then. Runs against a fake DefectDojo, to check the requests.
func TestAccNotificationWebhook_headerValueNotSentUnlessConfiguredVersionChanges(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "notification_webhooks",
		defaults:   map[string]any{"status": "active", "note": "", "first_error": nil, "last_error": nil, "owner": nil},
	}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	config := func(url, value string, version int) string {
		header := ""
		if version > 0 {
			header = fmt.Sprintf("  header_value_wo         = %q\n  header_value_wo_version = %d\n", value, version)
		}
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %[1]q
  api_token = "test"
}

resource "defectdojo_notification_webhook" "test" {
  name        = "fake"
  url         = %[2]q
  header_name = "Authorization"
%[3]s}
`, server.URL, url, header)
	}
	headerValueSent := func(want any) resource.TestCheckFunc {
		return func(*terraform.State) error {
			patch := fake.lastPatch()
			got, sent := patch["header_value"]
			if want == nil && sent || want != nil && got != want {
				return fmt.Errorf("PATCH body %v: header_value = %#v (sent %t), want %#v", patch, got, sent, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: config("https://hooks.invalid/one", "Bearer one", 1),
				Check: func(*terraform.State) error {
					if got := fake.lastPost()["header_value"]; got != "Bearer one" {
						return fmt.Errorf("POST header_value = %#v, want the value", got)
					}
					return nil
				},
			},
			// Another change, same version: the value isn't sent.
			{
				Config: config("https://hooks.invalid/two", "Bearer ignored", 1),
				Check:  headerValueSent(nil),
			},
			// A new version: sent.
			{
				Config: config("https://hooks.invalid/two", "Bearer two", 2),
				Check:  headerValueSent("Bearer two"),
			},
			// Removed: cleared.
			{
				Config: config("https://hooks.invalid/two", "", 0),
				Check: func(*terraform.State) error {
					patch := fake.lastPatch()
					if v, sent := patch["header_value"]; !sent || v != nil {
						return fmt.Errorf("PATCH body %v: want header_value null", patch)
					}
					return nil
				},
			},
		},
	})
}
