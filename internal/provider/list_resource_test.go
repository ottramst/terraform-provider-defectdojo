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

// TestAccListResourcesConfiguration lists the configuration objects created in
// the first step with terraform query, filtered by the random prefix of their
// names, in another case.
func TestAccListResourcesConfiguration(t *testing.T) {
	testAccSkipUnlessAcc(t)
	prefix := randName(t)
	upper := strings.ToUpper(prefix)
	ids := map[string]int64{}
	identity := func(address string) map[string]knownvalue.Check {
		return map[string]knownvalue.Check{"id": int64ExactFunc(func() int64 { return ids[address] })}
	}
	byIdentity := func(address string) queryfilter.QueryFilter {
		return queryfilter.ByResourceIdentity(identity(address))
	}
	// one checks that list block address finds only the object created at
	// resource, with the display name displayName.
	one := func(address, resource string, displayName knownvalue.Check) []querycheck.QueryResultCheck {
		return []querycheck.QueryResultCheck{
			querycheck.ExpectLength(address, 1),
			querycheck.ExpectIdentity(address, identity(resource)),
			querycheck.ExpectResourceDisplayName(address, byIdentity(resource), displayName),
		}
	}
	name := knownvalue.StringExact
	var checks []querycheck.QueryResultCheck
	for _, c := range [][]querycheck.QueryResultCheck{
		one("defectdojo_sla_configuration.test", "defectdojo_sla_configuration.test", name(prefix)),
		one("defectdojo_jira_instance.test", "defectdojo_jira_instance.test", name(prefix)),
		one("defectdojo_jira_project.test", "defectdojo_jira_project.test", name("LST / "+prefix)),
		one("defectdojo_notification_webhook.test", "defectdojo_notification_webhook.test", name(prefix)),
		one("defectdojo_notifications.test", "defectdojo_notifications.test", stringExactFunc(func() string { return fmt.Sprintf("user 1, product %d", ids["defectdojo_product.test"]) })),
		one("defectdojo_regulation.test", "defectdojo_regulation.test", name(prefix)),
		one("defectdojo_note_type.test", "defectdojo_note_type.test", name(prefix)),
		one("defectdojo_development_environment.test", "defectdojo_development_environment.test", name(prefix)),
		one("defectdojo_cicd_infrastructure.test", "defectdojo_cicd_infrastructure.test", name(prefix)),
		one("defectdojo_product_platform.test", "defectdojo_product_platform.test", name(prefix+"-platform")),
		one("defectdojo_product_lifecycle.test", "defectdojo_product_lifecycle.test", name(prefix+"-lifecycle")),
		one("defectdojo_product_origin.test", "defectdojo_product_origin.test", name(prefix+"-origin")),
		{
			querycheck.ExpectLength("defectdojo_cicd_infrastructure.other_type", 0),
			querycheck.ExpectResourceKnownValues("defectdojo_note_type.test", byIdentity("defectdojo_note_type.test"), []querycheck.KnownValueCheck{
				{Path: tfjsonpath.New("is_active"), KnownValue: knownvalue.Bool(false)},
				{Path: tfjsonpath.New("force_destroy"), KnownValue: knownvalue.Bool(false)},
			}),
			querycheck.ExpectResourceKnownValues("defectdojo_jira_instance.test", byIdentity("defectdojo_jira_instance.test"), []querycheck.KnownValueCheck{
				{Path: tfjsonpath.New("password_wo"), KnownValue: knownvalue.Null()},
				{Path: tfjsonpath.New("url"), KnownValue: knownvalue.StringExact("https://" + prefix + ".example.invalid")},
			}),
		},
	} {
		checks = append(checks, c...)
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
				Config: testAccListResourcesConfigurationConfig(prefix),
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
				Query:             true,
				Config:            testAccListResourcesConfigurationQuery(upper),
				QueryResultChecks: checks,
			},
		},
	})
}

func testAccListResourcesConfigurationConfig(prefix string) string {
	return fmt.Sprintf(`
resource "defectdojo_sla_configuration" "test" {
  name = %[1]q
}

resource "defectdojo_jira_instance" "test" {
  configuration_name        = %[1]q
  url                       = "https://%[1]s.example.invalid"
  username                  = "bot"
  password_wo               = "fake-token"
  epic_name_id              = 10011
  open_status_key           = 11
  close_status_key          = 31
  info_mapping_severity     = "Lowest"
  low_mapping_severity      = "Low"
  medium_mapping_severity   = "Medium"
  high_mapping_severity     = "High"
  critical_mapping_severity = "Highest"
}

resource "defectdojo_product_type" "test" {
  name = %[1]q
}

resource "defectdojo_product" "test" {
  name            = %[1]q
  description     = "list"
  product_type_id = defectdojo_product_type.test.id
}

resource "defectdojo_jira_project" "test" {
  jira_instance_id = defectdojo_jira_instance.test.id
  product_id       = defectdojo_product.test.id
  project_key      = "LST"
}

resource "defectdojo_notification_webhook" "test" {
  name = %[1]q
  url  = "http://%[1]s.invalid/hook"
}

resource "defectdojo_notifications" "test" {
  user_id    = 1
  product_id = defectdojo_product.test.id
  other      = ["alert"]
}

resource "defectdojo_regulation" "test" {
  name         = %[1]q
  acronym      = %[2]q
  category     = "other"
  jurisdiction = "Nowhere"
}

# Inactive, so that it never keeps findings from being closed.
resource "defectdojo_note_type" "test" {
  name         = %[1]q
  description  = "list"
  is_active    = false
  is_mandatory = false
}

resource "defectdojo_development_environment" "test" {
  name = %[1]q
}

resource "defectdojo_cicd_infrastructure" "test" {
  name                = %[1]q
  infrastructure_type = "build_server"
}

resource "defectdojo_product_platform" "test" {
  value = "%[1]s-platform"
  name  = %[1]q
}

resource "defectdojo_product_lifecycle" "test" {
  value = "%[1]s-lifecycle"
  name  = %[1]q
}

resource "defectdojo_product_origin" "test" {
  value = "%[1]s-origin"
  name  = %[1]q
}
`, prefix, testAccRegulationAcronym(prefix))
}

// testAccListResourcesConfigurationQuery lists the objects of
// testAccListResourcesConfigurationConfig, filtered by their prefix in upper
// case, or by their product.
func testAccListResourcesConfigurationQuery(upper string) string {
	named := ""
	for _, typ := range []string{"sla_configuration", "notification_webhook", "regulation", "note_type", "development_environment",
		"product_platform", "product_lifecycle", "product_origin"} {
		named += fmt.Sprintf(`
list "defectdojo_%s" "test" {
  provider         = defectdojo
  include_resource = true
  config {
    name_contains = %q
  }
}
`, typ, upper)
	}
	return named + fmt.Sprintf(`
list "defectdojo_jira_instance" "test" {
  provider         = defectdojo
  include_resource = true
  config {
    configuration_name_contains = %[1]q
  }
}

list "defectdojo_jira_project" "test" {
  provider = defectdojo
  config {
    product_id = defectdojo_product.test.id
  }
}

list "defectdojo_notifications" "test" {
  provider = defectdojo
  config {
    product_id = defectdojo_product.test.id
  }
}

list "defectdojo_cicd_infrastructure" "test" {
  provider = defectdojo
  config {
    infrastructure_type = "build_server"
    name_contains       = %[1]q
  }
}

list "defectdojo_cicd_infrastructure" "other_type" {
  provider = defectdojo
  config {
    infrastructure_type = "scm_server"
    name_contains       = %[1]q
  }
}
`, upper)
}

// stringExactFunc checks that a value is the string value returns when the
// check runs, such as one with an ID captured in an earlier step.
type stringExactFunc func() string

func (f stringExactFunc) CheckValue(other any) error {
	return knownvalue.StringExact(f()).CheckValue(other)
}

func (f stringExactFunc) String() string {
	return f()
}
