package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/ottramst/go-defectdojo"
)

func TestAccEngagementDataSource(t *testing.T) {
	name := randName(t)
	var productID, duplicateID int
	lookups := `
data "defectdojo_engagement" "by_id" {
  id = defectdojo_engagement.test.id
}

data "defectdojo_engagement" "by_name" {
  product_id = defectdojo_product.first.id
  name       = defectdojo_engagement.test.name
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccEngagementConfig(name, "first", `
  engagement_type = "CI/CD"
  branch_tag      = "main"
  tags            = ["lookup"]
`) + lookups,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.defectdojo_engagement.by_id", tfjsonpath.New("name"),
						engagementAddress, tfjsonpath.New("name"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_engagement.by_name", tfjsonpath.New("id"),
						engagementAddress, tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.defectdojo_engagement.by_name", tfjsonpath.New("engagement_type"), knownvalue.StringExact("CI/CD")),
					statecheck.ExpectKnownValue("data.defectdojo_engagement.by_name", tfjsonpath.New("branch_tag"), knownvalue.StringExact("main")),
					statecheck.ExpectKnownValue("data.defectdojo_engagement.by_name", tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue("data.defectdojo_engagement.by_name", tfjsonpath.New("target_end"), knownvalue.StringExact("2099-12-31")),
					statecheck.ExpectKnownValue("data.defectdojo_engagement.by_name", tfjsonpath.New("tags"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("lookup")})),
				},
				Check: func(s *terraform.State) error {
					_, err := fmt.Sscan(s.RootModule().Resources["defectdojo_product.first"].Primary.ID, &productID)
					return err
				},
			},
			// A second engagement with the same name makes the name lookup
			// ambiguous.
			{
				PreConfig: func() {
					day := defectdojo.Date{Year: 2026, Month: time.January, Day: 5}
					e, _, err := testAccClient(t).Engagements.Create(context.Background(), &defectdojo.Engagement{
						Product: productID, Name: name, TargetStart: day, TargetEnd: day,
					})
					testAccNoError(t, err)
					duplicateID = e.ID
				},
				Config:      testAccEngagementConfig(name, "first", "") + lookups,
				ExpectError: regexp.MustCompile(`has 2 engagements named`),
			},
			{
				Config: testAccEngagementConfig(name, "first", "") + `
data "defectdojo_engagement" "missing" {
  product_id = defectdojo_product.first.id
  name       = "missing"
}
`,
				ExpectError: regexp.MustCompile(`DefectDojo engagement not found`),
			},
			{
				Config: testAccEngagementConfig(name, "first", "") + `
data "defectdojo_engagement" "no_product" {
  name = "x"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			// A valid config last, for the destroy after the test, without the
			// second engagement, which would keep the product from being
			// destroyed.
			{
				PreConfig: func() {
					_, err := testAccClient(t).Engagements.Delete(context.Background(), duplicateID)
					testAccNoError(t, err)
				},
				Config: testAccEngagementConfig(name, "first", ""),
			},
		},
	})
}
