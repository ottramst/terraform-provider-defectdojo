package provider

import (
	"fmt"
	"maps"
	"net/http/httptest"
	"regexp"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// Create and Update send exactly the configured settings, and the secrets
// only when their version is set or changes; Delete sends nothing. Runs
// against a fake DefectDojo, to check the requests.
func TestAccSystemSettings_onlyConfiguredSent(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "system_settings",
		id:         1,
		singleton:  true,
		object: map[string]any{
			"id": 1, "team_name": "", "email_from": "no-reply@example.com", "max_dupes": 10,
			"minimum_password_length": 9, "maximum_password_length": 48, "slack_token": "",
		},
	}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	config := func(body string) string {
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %q
  api_token = "test"
}

resource "defectdojo_system_settings" "test" {
%s}
`, server.URL, body)
	}
	var writes int

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Write-only attributes need Terraform 1.11+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		// Destroy only removes the resource from the state.
		CheckDestroy: func(*terraform.State) error {
			if n := fake.writeCount(); n != writes {
				return fmt.Errorf("destroy sent %d requests, want none", n-writes)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: config(`
  team_name              = "AppSec"
  max_dupes              = 0
  slack_token_wo         = "xoxb-fake-one"
  slack_token_wo_version = 1
`),
				Check: checkPatchExactly(fake, "team_name", "max_dupes", "slack_token"),
			},
			// The configured settings are sent again, the token isn't.
			{
				Config: config(`
  team_name              = "AppSec2"
  max_dupes              = 0
  slack_token_wo         = "xoxb-fake-one"
  slack_token_wo_version = 1
`),
				Check: checkPatchExactly(fake, "team_name", "max_dupes"),
			},
			// A new version sends the token.
			{
				Config: config(`
  team_name              = "AppSec2"
  slack_token_wo         = "xoxb-fake-two"
  slack_token_wo_version = 2
`),
				Check: checkPatchExactly(fake, "team_name", "slack_token"),
			},
			// Removing the token and its version sends nothing for it.
			{
				Config: config(`
  team_name = "AppSec3"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkPatchExactly(fake, "team_name"),
					func(*terraform.State) error {
						writes = fake.writeCount()
						return nil
					},
				),
			},
		},
	})
}

// checkPatchExactly checks that the last PATCH request sent exactly fields.
func checkPatchExactly(fake *fakeDefectDojo, fields ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		body := fake.lastPatch()
		if body == nil {
			return fmt.Errorf("no PATCH request was sent")
		}
		got := slices.Sorted(maps.Keys(body))
		want := slices.Sorted(slices.Values(fields))
		if !slices.Equal(got, want) {
			return fmt.Errorf("PATCH sent %v, want %v", got, want)
		}
		return nil
	}
}

// On create, a configured minimum password length is compared with
// DefectDojo's maximum, which the configuration leaves out.
func TestAccSystemSettings_passwordLengthsOnCreate(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "system_settings",
		id:         1,
		singleton:  true,
		object:     map[string]any{"id": 1, "minimum_password_length": 9, "maximum_password_length": 30},
	}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	config := func(minimum int) string {
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %q
  api_token = "test"
}

resource "defectdojo_system_settings" "test" {
  minimum_password_length = %d
}
`, server.URL, minimum)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// PlanOnly: the create plan must refuse it, before anything is
			// written.
			{
				Config:      config(40),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`minimum_password_length \(40\) exceeds\s+maximum_password_length \(30\)`),
			},
			{
				Config: config(20),
			},
		},
	})
}
