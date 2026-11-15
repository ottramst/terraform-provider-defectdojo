package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccNoteTypeDataSource(t *testing.T) {
	name := randName(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNoteTypeConfig(name, "  is_active = false\n  is_single = true\n") + `
data "defectdojo_note_type" "by_id" {
  id = defectdojo_note_type.test.id
}

data "defectdojo_note_type" "by_name" {
  name = defectdojo_note_type.test.name
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.defectdojo_note_type.by_id", tfjsonpath.New("name"),
						noteTypeAddress, tfjsonpath.New("name"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_note_type.by_name", tfjsonpath.New("id"),
						noteTypeAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_note_type.by_name", tfjsonpath.New("description"), knownvalue.StringExact("acceptance test")),
					statecheck.ExpectKnownValue("data.defectdojo_note_type.by_name", tfjsonpath.New("is_single"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("data.defectdojo_note_type.by_name", tfjsonpath.New("is_active"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue("data.defectdojo_note_type.by_name", tfjsonpath.New("is_mandatory"), knownvalue.Bool(true)),
				},
			},
			{
				Config: fmt.Sprintf(`
data "defectdojo_note_type" "missing" {
  name = %q
}
`, name+"-missing"),
				ExpectError: regexp.MustCompile(`DefectDojo note type not found`),
			},
			{
				Config: `
data "defectdojo_note_type" "both" {
  id   = 1
  name = "x"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			// A valid config last, for the destroy after the test.
			{
				Config: testAccNoteTypeConfig(name, "  is_active = false\n"),
			},
		},
	})
}
