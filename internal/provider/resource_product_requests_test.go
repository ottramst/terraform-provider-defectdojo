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
)

// Updates send sla_configuration_id, authorized_users, regulation_ids and
// tags only when they are configured, so that values managed elsewhere are
// kept. Runs against a fake DefectDojo, to check the requests.
func TestAccProduct_managedElsewhereNotSentUnlessConfigured(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "products",
		defaults: map[string]any{
			"business_criticality": nil, "platform": nil, "lifecycle": nil, "origin": nil,
			"product_manager": nil, "technical_contact": nil, "team_manager": nil, "sla_configuration": 1,
			"authorized_users": []any{}, "regulations": []any{}, "tags": []any{},
		},
	}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)
	managed := []string{"sla_configuration", "authorized_users", "regulations", "tags"}

	config := func(description, extra string) string {
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %[1]q
  api_token = "test"
}

resource "defectdojo_product" "test" {
  name            = "fake"
  description     = %[2]q
  product_type_id = 1
%[3]s}
`, server.URL, description, extra)
	}
	all := `
  sla_configuration_id = 2
  authorized_users     = [1]
  regulation_ids       = [1]
  tags                 = ["a"]
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("one", all),
			},
			// Configured: sent.
			{
				Config: config("two", all),
				Check:  fake.checkPatchSends(true, managed...),
			},
			// Not configured: not sent, and changes made elsewhere are kept.
			{
				PreConfig: func() {
					fake.set("sla_configuration", 3)
					fake.set("authorized_users", []any{1, 2})
					fake.set("regulations", []any{1, 2})
					fake.set("tags", []any{"a", "b"})
				},
				Config: config("three", ""),
				Check:  fake.checkPatchSends(false, managed...),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("sla_configuration_id"), knownvalue.Int64Exact(3)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("authorized_users"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("regulation_ids"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("tags"), knownvalue.SetSizeExact(2)),
				},
			},
			// Configured as empty: sent, and removes them all.
			{
				Config: config("four", `
  authorized_users = []
  regulation_ids   = []
  tags             = []
`),
				Check: fake.checkPatchSends(true, "authorized_users", "regulations", "tags"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("tags"), knownvalue.SetSizeExact(0)),
				},
			},
		},
	})
}

// user_records and revenue are sent on create when they are configured, 0
// included, and as null when they are removed, which clears them. Runs
// against a fake DefectDojo, to check the requests.
func TestAccProduct_userRecordsAndRevenueRequests(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "products",
		defaults: map[string]any{
			"business_criticality": nil, "platform": nil, "lifecycle": nil, "origin": nil,
			"user_records": nil, "revenue": nil,
			"product_manager": nil, "technical_contact": nil, "team_manager": nil, "sla_configuration": 1,
			"authorized_users": []any{}, "regulations": []any{}, "tags": []any{},
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

resource "defectdojo_product" "test" {
  name            = "fake"
  description     = %[2]q
  product_type_id = 1
%[3]s}
`, server.URL, description, extra)
	}
	sends := func(last func() map[string]any, field string, want any) resource.TestCheckFunc {
		return func(*terraform.State) error {
			body := last()
			got, sent := body[field]
			if !sent || got != want {
				return fmt.Errorf("body %v: %s = %v (sent %v), want %v", body, field, got, sent, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("one", "  user_records = 0\n  revenue = \"0.00\"\n"),
				Check: resource.ComposeTestCheckFunc(
					sends(fake.lastPost, "user_records", float64(0)),
					sends(fake.lastPost, "revenue", "0.00"),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("user_records"), knownvalue.Int64Exact(0)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("revenue"), knownvalue.StringExact("0.00")),
				},
			},
			// Removed: sent as null, which clears them.
			{
				Config: config("two", ""),
				Check: resource.ComposeTestCheckFunc(
					sends(fake.lastPatch, "user_records", nil),
					sends(fake.lastPatch, "revenue", nil),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("user_records"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("revenue"), knownvalue.Null()),
				},
			},
		},
	})
}
