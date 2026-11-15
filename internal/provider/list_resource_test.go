package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// TestAccListResources lists objects created in the first step with
// terraform query. The filters match the random prefix of their names, in
// another case, so that other objects stay out of the results.
func TestAccListResources(t *testing.T) {
	testAccSkipUnlessAcc(t)
	prefix := randName(t)
	upper := strings.ToUpper(prefix)
	ids := map[string]int64{}

	// identity checks the identity of the object created in the first step
	// at address.
	identity := func(address string) map[string]knownvalue.Check {
		return map[string]knownvalue.Check{"id": int64ExactFunc(func() int64 { return ids[address] })}
	}
	byIdentity := func(address string) queryfilter.QueryFilter {
		return queryfilter.ByResourceIdentity(identity(address))
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// terraform query needs Terraform 1.14+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.RequireAbove(tfversion.Version1_14_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccListResourcesConfig(prefix),
				Check: func(s *terraform.State) error {
					for address, rs := range s.RootModule().Resources {
						var id int64
						if _, err := fmt.Sscan(rs.Primary.ID, &id); err != nil {
							return fmt.Errorf("%s: %w", address, err)
						}
						ids[address] = id
					}
					return nil
				},
			},
			{
				Query:  true,
				Config: testAccListResourcesQuery(upper),
				QueryResultChecks: []querycheck.QueryResultCheck{
					// Product types
					querycheck.ExpectLength("defectdojo_product_type.test", 2),
					querycheck.ExpectIdentity("defectdojo_product_type.test", identity("defectdojo_product_type.a")),
					querycheck.ExpectResourceDisplayName("defectdojo_product_type.test",
						byIdentity("defectdojo_product_type.b"), knownvalue.StringExact(prefix+"-type-b")),
					querycheck.ExpectResourceKnownValues("defectdojo_product_type.test",
						byIdentity("defectdojo_product_type.a"), []querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("critical_product"), KnownValue: knownvalue.Bool(true)},
							{Path: tfjsonpath.New("description"), KnownValue: knownvalue.Null()},
							{Path: tfjsonpath.New("force_destroy"), KnownValue: knownvalue.Bool(false)},
						}),

					// Products: only those of product type a
					querycheck.ExpectLength("defectdojo_product.test", 2),
					querycheck.ExpectIdentity("defectdojo_product.test", identity("defectdojo_product.a")),
					querycheck.ExpectIdentity("defectdojo_product.test", identity("defectdojo_product.b")),
					querycheck.ExpectResourceKnownValues("defectdojo_product.test",
						byIdentity("defectdojo_product.a"), []querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(prefix + "-product-a")},
							{Path: tfjsonpath.New("user_records"), KnownValue: knownvalue.Int64Exact(0)},
							{Path: tfjsonpath.New("tags"), KnownValue: knownvalue.SetExact([]knownvalue.Check{
								knownvalue.StringExact(prefix + "-inherited"),
							})},
						}),
					querycheck.ExpectLength("defectdojo_product.other_type", 0),

					// Engagements: the product's tag, which DefectDojo adds,
					// is hidden as after an import.
					querycheck.ExpectLength("defectdojo_engagement.test", 1),
					querycheck.ExpectIdentity("defectdojo_engagement.test", identity("defectdojo_engagement.a")),
					querycheck.ExpectResourceDisplayName("defectdojo_engagement.test",
						byIdentity("defectdojo_engagement.a"), knownvalue.StringExact(prefix+"-product-a / "+prefix+"-engagement-a")),
					querycheck.ExpectResourceKnownValues("defectdojo_engagement.test",
						byIdentity("defectdojo_engagement.a"), []querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("tags"), KnownValue: knownvalue.SetExact([]knownvalue.Check{
								knownvalue.StringExact("own"),
							})},
							{Path: tfjsonpath.New("target_start"), KnownValue: knownvalue.StringExact("2026-01-05")},
						}),
					querycheck.ExpectLength("defectdojo_engagement.product", 2),

					// Users
					querycheck.ExpectLength("defectdojo_user.test", 2),
					querycheck.ExpectResourceDisplayName("defectdojo_user.test",
						byIdentity("defectdojo_user.a"), knownvalue.StringExact(prefix+"-user-a")),
					querycheck.ExpectResourceKnownValues("defectdojo_user.test",
						byIdentity("defectdojo_user.a"), []querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("configuration_permissions"), KnownValue: knownvalue.SetExact([]knownvalue.Check{
								knownvalue.StringExact("add_product_type"),
							})},
							{Path: tfjsonpath.New("password_wo"), KnownValue: knownvalue.Null()},
						}),
					querycheck.ExpectResourceKnownValues("defectdojo_user.test",
						byIdentity("defectdojo_user.b"), []querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("configuration_permissions"), KnownValue: knownvalue.SetSizeExact(0)},
						}),
				},
			},
			// An engagement filter by a product that doesn't exist.
			{
				Query:       true,
				Config:      listMissingProductQuery,
				ExpectError: regexp.MustCompile(`DefectDojo product not found`),
			},
		},
	})
}

func testAccListResourcesConfig(prefix string) string {
	return fmt.Sprintf(`
resource "defectdojo_product_type" "a" {
  name             = "%[1]s-type-a"
  critical_product = true
}

resource "defectdojo_product_type" "b" {
  name = "%[1]s-type-b"
}

resource "defectdojo_product" "a" {
  name                           = "%[1]s-product-a"
  description                    = "list"
  product_type_id                = defectdojo_product_type.a.id
  user_records                   = 0
  enable_product_tag_inheritance = true
  tags                           = ["%[1]s-inherited"]
}

resource "defectdojo_product" "b" {
  name            = "%[1]s-product-b"
  description     = "list"
  product_type_id = defectdojo_product_type.a.id
}

resource "defectdojo_engagement" "a" {
  product_id   = defectdojo_product.a.id
  name         = "%[1]s-engagement-a"
  target_start = "2026-01-05"
  target_end   = "2099-12-31"
  tags         = ["own"]
}

# DefectDojo's tag inheritance fails (500) when two engagements of a product
# that passes its tags on are created at the same time.
resource "defectdojo_engagement" "b" {
  product_id   = defectdojo_product.a.id
  name         = "%[1]s-other"
  target_start = "2026-01-05"
  target_end   = "2099-12-31"
  depends_on   = [defectdojo_engagement.a]
}

resource "defectdojo_user" "a" {
  username                  = "%[1]s-user-a"
  email                     = "%[1]s-a@example.com"
  password_wo               = %[2]q
  configuration_permissions = ["add_product_type"]
}

resource "defectdojo_user" "b" {
  username    = "%[1]s-user-b"
  email       = "%[1]s-b@example.com"
  password_wo = %[2]q
}
`, prefix, testAccPassword)
}

// testAccListResourcesQuery lists the objects of testAccListResourcesConfig,
// filtered by their prefix in upper case.
func testAccListResourcesQuery(upper string) string {
	return fmt.Sprintf(`
list "defectdojo_product_type" "test" {
  provider         = defectdojo
  include_resource = true
  config {
    name_contains = %[1]q
  }
}

list "defectdojo_product" "test" {
  provider         = defectdojo
  include_resource = true
  config {
    product_type_id = defectdojo_product_type.a.id
    name_contains   = %[1]q
  }
}

list "defectdojo_product" "other_type" {
  provider = defectdojo
  config {
    product_type_id = defectdojo_product_type.b.id
  }
}

list "defectdojo_engagement" "test" {
  provider         = defectdojo
  include_resource = true
  config {
    name_contains = "%[1]s-ENGAGEMENT"
  }
}

list "defectdojo_engagement" "product" {
  provider = defectdojo
  config {
    product_id = defectdojo_product.a.id
  }
}

list "defectdojo_user" "test" {
  provider         = defectdojo
  include_resource = true
  config {
    username_contains = "%[1]s-USER"
  }
}
`, upper)
}

const listMissingProductQuery = `
list "defectdojo_engagement" "missing" {
  provider = defectdojo
  config {
    product_id = 2147483647
  }
}
`

// int64ExactFunc checks that a value is the number value returns when the
// check runs, such as an ID captured in an earlier step.
type int64ExactFunc func() int64

func (f int64ExactFunc) CheckValue(other any) error {
	return knownvalue.Int64Exact(f()).CheckValue(other)
}

func (f int64ExactFunc) String() string {
	return fmt.Sprint(f())
}
