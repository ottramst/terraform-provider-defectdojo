package provider

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// The password is only sent on create, configuration_permissions only when
// they are configured, and a removed name as "" (DefectDojo refuses null).
// Runs against a fake DefectDojo, to check the requests.
func TestAccUser_permissionsNotSentUnlessConfigured(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "users",
		defaults: map[string]any{
			"first_name": "", "last_name": "", "is_active": true, "is_staff": false, "is_superuser": false,
			"configuration_permissions": []any{}, "date_joined": "2026-11-14T12:00:00Z",
		},
		lists: map[string][]map[string]any{
			"configuration_permissions": {
				{"id": 9, "name": "Can add user", "codename": "add_user"},
				{"id": 389, "name": "Can add sl a_ configuration", "codename": "add_sla_configuration"},
			},
		},
	}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	config := func(email, extra string) string {
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %[1]q
  api_token = "test"
}

resource "defectdojo_user" "test" {
  username    = "fake"
  email       = %[2]q
  password_wo = "Fake-Passw0rd!"
%[3]s}
`, server.URL, email, extra)
	}
	firstNameSent := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			if got := fake.lastPatch()["first_name"]; got != want {
				return fmt.Errorf("PATCH first_name = %#v, want %q", got, want)
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
				Config: config("one@example.com", `
  first_name                = "A"
  configuration_permissions = ["add_user"]
`),
				Check: fake.checkPostSends(true, "password", "configuration_permissions"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("configuration_permissions"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("add_user")})),
				},
			},
			// Configured: sent; the password never is.
			{
				Config: config("two@example.com", `
  first_name                = "A"
  configuration_permissions = ["add_user", "add_sla_configuration"]
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					fake.checkPatchSends(true, "configuration_permissions"),
					fake.checkPatchSends(false, "password"),
				),
			},
			// Not configured: not sent, and a change made elsewhere is kept;
			// the removed first name is sent as "".
			{
				PreConfig: func() { fake.set("configuration_permissions", []any{9}) },
				Config:    config("three@example.com", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					fake.checkPatchSends(false, "configuration_permissions", "password"),
					firstNameSent(""),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("first_name"), knownvalue.Null()),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("configuration_permissions"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("add_user")})),
				},
			},
			// Configured as empty: sent, and removes them all.
			{
				Config: config("four@example.com", "  configuration_permissions = []\n"),
				Check:  fake.checkPatchSends(true, "configuration_permissions"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("configuration_permissions"), knownvalue.SetSizeExact(0)),
				},
			},
		},
	})
}

// Reading the codenames of configuration permissions needs a superuser:
// without one, the planned or prior value is kept, with a warning, instead of
// failing every plan. Runs against a fake DefectDojo.
func TestAccUser_permissionsUnreadable(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "users",
		defaults: map[string]any{
			"first_name": "", "last_name": "", "is_active": true, "is_staff": false, "is_superuser": false,
			// Granted by a superuser.
			"configuration_permissions": []any{9}, "date_joined": "2026-11-14T12:00:00Z",
		},
		forbidden: map[string]bool{"configuration_permissions": true},
	}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	config := func(email string) string {
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %[1]q
  api_token = "test"
}

resource "defectdojo_user" "test" {
  username = "fake"
  email    = %[2]q
}
`, server.URL, email)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("one@example.com"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("configuration_permissions"), knownvalue.Null()),
				},
			},
			// Refresh and update keep working.
			{
				Config: config("two@example.com"),
				Check:  fake.checkPatchSends(false, "configuration_permissions"),
			},
		},
	})
}
