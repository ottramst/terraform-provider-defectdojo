package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// The test types the fixtures load, read only.
func TestAccTestTypesDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "defectdojo_test_types" "all" {}

data "defectdojo_test_types" "api" {
  name = "API Test"
}

data "defectdojo_test_types" "other_case" {
  name = "api test"
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.defectdojo_test_types.api", tfjsonpath.New("names"),
						knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("API Test")})),
					statecheck.ExpectKnownValue("data.defectdojo_test_types.api", tfjsonpath.New("test_types"),
						knownvalue.ListExact([]knownvalue.Check{knownvalue.ObjectExact(map[string]knownvalue.Check{
							"id":           knownvalue.Int64Exact(1),
							"name":         knownvalue.StringExact("API Test"),
							"static_tool":  knownvalue.Bool(false),
							"dynamic_tool": knownvalue.Bool(false),
							"active":       knownvalue.Bool(true),
						})})),
					statecheck.ExpectKnownValue("data.defectdojo_test_types.other_case", tfjsonpath.New("names"), knownvalue.ListSizeExact(0)),
					statecheck.ExpectKnownValue("data.defectdojo_test_types.other_case", tfjsonpath.New("test_types"), knownvalue.ListSizeExact(0)),
				},
				Check: testAccCheckAllTestTypes,
			},
		},
	})
}

// testAccCheckAllTestTypes checks that the unfiltered list has the scanners'
// test types, sorted by name, with as many names as objects.
func testAccCheckAllTestTypes(s *terraform.State) error {
	rs, ok := s.RootModule().Resources["data.defectdojo_test_types.all"]
	if !ok {
		return fmt.Errorf("data.defectdojo_test_types.all not found in state")
	}
	attrs := rs.Primary.Attributes
	var n int
	if _, err := fmt.Sscan(attrs["names.#"], &n); err != nil {
		return err
	}
	if n < 100 || attrs["test_types.#"] != attrs["names.#"] {
		return fmt.Errorf("names.# = %s, test_types.# = %s, want the same count above 100", attrs["names.#"], attrs["test_types.#"])
	}
	found := map[string]bool{}
	previous := ""
	for i := range n {
		name := attrs[fmt.Sprintf("names.%d", i)]
		if name < previous {
			return fmt.Errorf("names are not sorted: %q after %q", name, previous)
		}
		if attrs[fmt.Sprintf("test_types.%d.name", i)] != name {
			return fmt.Errorf("test_types.%d.name = %q, want %q", i, attrs[fmt.Sprintf("test_types.%d.name", i)], name)
		}
		previous = name
		found[name] = true
	}
	for _, name := range []string{"API Test", "Trivy Scan", "ZAP Scan"} {
		if !found[name] {
			return fmt.Errorf("names lacks %q", name)
		}
	}
	return nil
}
