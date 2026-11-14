package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/ottramst/go-defectdojo"
)

const engagementAddress = "defectdojo_engagement.test"

func TestAccEngagement(t *testing.T) {
	name := randName(t)
	renamed := name + "-renamed"
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Resource identity needs Terraform 1.12+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		CheckDestroy: testAccCheckEngagementDestroyed(t, &id),
		Steps: []resource.TestStep{
			// Create with the defaults
			{
				Config: testAccEngagementConfig(name, "first", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.CompareValuePairs(engagementAddress, tfjsonpath.New("product_id"),
						"defectdojo_product.first", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("target_start"), knownvalue.StringExact("2026-01-05")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("target_end"), knownvalue.StringExact("2099-12-31")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("engagement_type"), knownvalue.StringExact("Interactive")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("status"), knownvalue.StringExact("Not Started")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("active"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("first_contacted"), knownvalue.Null()),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("threat_model"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("deduplication_on_engagement"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("lead_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("tags"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("force_destroy"), knownvalue.Bool(false)),
					statecheck.ExpectIdentityValueMatchesState(engagementAddress, tfjsonpath.New("id")),
				},
				Check: testAccCaptureEngagementID(&id),
			},
			// Update every attribute in place
			{
				Config: testAccEngagementConfig(renamed, "first", `
  engagement_type             = "CI/CD"
  status                      = "In Progress"
  description                 = "Scans of the main branch"
  reason                      = "Continuous scanning"
  version                     = "1.4.0"
  build_id                    = "42"
  commit_hash                 = "abc123"
  branch_tag                  = "main"
  first_contacted             = "2025-12-01"
  tracker                     = "https://tracker.example/T-1"
  test_strategy               = "https://wiki.example/strategy"
  source_code_management_uri  = "https://git.example/shop"
  deduplication_on_engagement = true
  threat_model                = false
  api_test                    = false
  pen_test                    = false
  check_list                  = false
  lead_id                     = 1
  tags                        = ["ci", "Main"]
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(engagementAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("engagement_type"), knownvalue.StringExact("CI/CD")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("status"), knownvalue.StringExact("In Progress")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("first_contacted"), knownvalue.StringExact("2025-12-01")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("source_code_management_uri"), knownvalue.StringExact("https://git.example/shop")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("deduplication_on_engagement"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("check_list"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("lead_id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("tags"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("ci"), knownvalue.StringExact("Main")})),
				},
				Check: testAccCheckEngagementID(&id),
			},
			// Import by ID
			{
				ResourceName:      engagementAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				ResourceName:    engagementAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// Move to another product. Unset optional attributes are cleared;
			// unset status, type and tags are kept.
			{
				Config: testAccEngagementConfig(renamed, "second", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(engagementAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(engagementAddress, tfjsonpath.New("product_id"),
						"defectdojo_product.second", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("first_contacted"), knownvalue.Null()),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("lead_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("threat_model"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("status"), knownvalue.StringExact("In Progress")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("engagement_type"), knownvalue.StringExact("CI/CD")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("tags"), knownvalue.SetSizeExact(2)),
				},
				Check: testAccCheckEngagementID(&id),
			},
			// Changed outside Terraform, including an import moving the
			// target end: the changes are reverted
			{
				PreConfig: func() {
					later := defectdojo.Date{Year: 2100, Month: time.January, Day: 15}
					_, _, err := testAccClient(t).Engagements.Patch(context.Background(), id, &defectdojo.EngagementPatch{
						Name: defectdojo.Value(name + "-changed"), TargetEnd: &later, Version: defectdojo.Value("9"),
					})
					testAccNoError(t, err)
				},
				Config: testAccEngagementConfig(renamed, "second", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(engagementAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("target_end"), knownvalue.StringExact("2099-12-31")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("version"), knownvalue.Null()),
				},
			},
			// Closed outside Terraform: with ignore_changes on the configured
			// status, a change of another attribute doesn't reopen it
			{
				PreConfig: func() {
					_, err := testAccClient(t).Engagements.Close(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: testAccEngagementConfig(renamed, "second", `
  status      = "In Progress"
  description = "closed elsewhere"
  lifecycle {
    ignore_changes = [status]
  }
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("description"), knownvalue.StringExact("closed elsewhere")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("status"), knownvalue.StringExact("Completed")),
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("active"), knownvalue.Bool(false)),
				},
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).Engagements.Delete(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: testAccEngagementConfig(renamed, "second", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(engagementAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureEngagementID(&id),
			},
		},
	})
}

// An engagement with tests is not destroyed unless force_destroy is set,
// because DefectDojo would delete the tests too.
func TestAccEngagement_forceDestroy(t *testing.T) {
	name := randName(t)
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEngagementDestroyed(t, &id),
		Steps: []resource.TestStep{
			{
				Config: testAccEngagementConfig(name, "first", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureEngagementID(&id),
					func(*terraform.State) error {
						testAccCreateTest(t, id)
						return nil
					},
				),
			},
			// Destroying is refused while the engagement has a test.
			{
				Config:      testAccEngagementConfig(name, "first", ""),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`has 1 tests`),
			},
			// With force_destroy, the engagement and its test are deleted (by
			// the test case's destroy).
			{
				Config: testAccEngagementConfig(name, "first", "  force_destroy = true\n"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("force_destroy"), knownvalue.Bool(true)),
				},
			},
		},
	})
}

// DefectDojo adds the tags of a product that passes them on to its
// engagements. The provider hides them unless they are configured, also
// across moves between products.
func TestAccEngagement_inheritedTags(t *testing.T) {
	name := randName(t)
	suffix := name[len("acc-"):]
	tagA, tagB := "inh-a-"+suffix, "inh-b-"+suffix
	var id int
	config := func(product, extra string) string {
		return fmt.Sprintf(`
resource "defectdojo_product_type" "test" {
  name = %[1]q
}

resource "defectdojo_product" "first" {
  name                           = "%[1]s-first"
  description                    = "acceptance test"
  product_type_id                = defectdojo_product_type.test.id
  enable_product_tag_inheritance = true
  tags                           = [%[2]q]
}

resource "defectdojo_product" "second" {
  name                           = "%[1]s-second"
  description                    = "acceptance test"
  product_type_id                = defectdojo_product_type.test.id
  enable_product_tag_inheritance = true
  tags                           = [%[3]q]
}

resource "defectdojo_engagement" "test" {
  product_id   = defectdojo_product.%[4]s.id
  name         = %[1]q
  target_start = "2026-01-01"
  target_end   = "2099-12-31"
%[5]s}
`, name, tagA, tagB, product, extra)
	}
	tags := func(want ...string) statecheck.StateCheck {
		checks := make([]knownvalue.Check, len(want))
		for i, tag := range want {
			checks[i] = knownvalue.StringExact(tag)
		}
		return statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("tags"), knownvalue.SetExact(checks))
	}
	// inDefectDojo checks the engagement's tags in DefectDojo, ignoring case:
	// DefectDojo may store a configured spelling of an inherited tag.
	inDefectDojo := func(want ...string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			e, _, err := testAccClient(t).Engagements.Get(context.Background(), id)
			if err != nil {
				return err
			}
			lower := func(tags []string) []string {
				out := make([]string, len(tags))
				for i, tag := range tags {
					out[i] = strings.ToLower(tag)
				}
				slices.Sort(out)
				return out
			}
			if got := lower(e.Tags); !slices.Equal(got, lower(want)) {
				return fmt.Errorf("DefectDojo tags = %v, want %v (ignoring case)", e.Tags, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Not configured: the engagement inherits the product's tag, which
			// is hidden.
			{
				Config:            config("first", ""),
				ConfigStateChecks: []statecheck.StateCheck{tags()},
				Check:             resource.ComposeAggregateTestCheckFunc(testAccCaptureEngagementAddressID(&id), inDefectDojo(tagA)),
			},
			// Configured, also the inherited tag: both are shown.
			{
				Config:            config("first", fmt.Sprintf("  tags = [\"own\", %q]\n", strings.ToUpper(tagA))),
				ConfigStateChecks: []statecheck.StateCheck{tags("own", strings.ToUpper(tagA))},
				Check:             inDefectDojo("own", tagA),
			},
			// The inherited tag is no longer configured: DefectDojo keeps it,
			// and it is hidden again.
			{
				Config:            config("first", "  tags = [\"own\"]\n"),
				ConfigStateChecks: []statecheck.StateCheck{tags("own")},
				Check:             inDefectDojo("own", tagA),
			},
			// Moved with tags configured: the new product's tag replaces the
			// old one's.
			{
				Config:            config("second", "  tags = [\"own\"]\n"),
				ConfigStateChecks: []statecheck.StateCheck{tags("own")},
				Check:             inDefectDojo("own", tagB),
			},
			// Moved back without tags configured: the same.
			{
				Config:            config("first", ""),
				ConfigStateChecks: []statecheck.StateCheck{tags("own")},
				Check:             inDefectDojo("own", tagA),
			},
			// Import: the product's tags are hidden.
			{
				ResourceName:            engagementAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"tags"},
			},
		},
	})
}

func TestAccEngagement_validation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "defectdojo_engagement" "test" {
  product_id   = 1
  name         = "never created"
  target_start = "2026-02-01"
  target_end   = "2026-01-31"
}
`,
				ExpectError: regexp.MustCompile(`must not be after target_end`),
			},
			{
				Config: `
resource "defectdojo_engagement" "test" {
  product_id   = 1
  name         = "never created"
  target_start = "2026-1-5"
  target_end   = "2026-12-31"
}
`,
				ExpectError: regexp.MustCompile(`not a date written as YYYY-MM-DD`),
			},
			{
				Config: `
resource "defectdojo_engagement" "test" {
  product_id   = 1
  name         = "never created"
  target_start = "2026-01-05"
  target_end   = "2026-12-31"
  tracker      = "tracker.example/T-1"
}
`,
				ExpectError: regexp.MustCompile(`must be an http, https, ftp or ftps URL`),
			},
		},
	})
}

// testAccEngagementConfig returns a configuration with a product type, two
// products, first and second, and an engagement of name in the product
// product.
func testAccEngagementConfig(name, product, extra string) string {
	return fmt.Sprintf(`
resource "defectdojo_product_type" "test" {
  name = %[1]q
}

resource "defectdojo_product" "first" {
  name            = "%[1]s-first"
  description     = "acceptance test"
  product_type_id = defectdojo_product_type.test.id
}

resource "defectdojo_product" "second" {
  name            = "%[1]s-second"
  description     = "acceptance test"
  product_type_id = defectdojo_product_type.test.id
}

resource "defectdojo_engagement" "test" {
  product_id   = defectdojo_product.%[2]s.id
  name         = %[1]q
  target_start = "2026-01-05"
  target_end   = "2099-12-31"
%[3]s}
`, name, product, extra)
}

// testAccCaptureEngagementID stores the ID of the engagement in id.
func testAccCaptureEngagementID(id *int) resource.TestCheckFunc {
	return testAccCaptureEngagementAddressID(id)
}

func testAccCaptureEngagementAddressID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[engagementAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", engagementAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckEngagementID checks that the engagement still has the ID id:
// it was updated in place.
func testAccCheckEngagementID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var got int
		if err := testAccCaptureEngagementID(&got)(s); err != nil {
			return err
		}
		if got != *id {
			return fmt.Errorf("engagement ID = %d, want %d (updated in place)", got, *id)
		}
		return nil
	}
}

// testAccCheckEngagementDestroyed checks that the engagement is gone.
func testAccCheckEngagementDestroyed(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := testAccClient(t).Engagements.Get(context.Background(), *id)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("engagement %d still exists", *id)
	}
}

// testAccCreateTest creates a test in the engagement through the raw API;
// the client has no tests service.
func testAccCreateTest(t *testing.T, engagementID int) {
	t.Helper()
	client := testAccClient(t)
	body := map[string]any{
		"engagement": engagementID, "test_type": 1, "title": "acceptance test",
		"target_start": "2026-01-01T00:00:00Z", "target_end": "2026-01-02T00:00:00Z",
	}
	req, err := client.NewRequest(context.Background(), http.MethodPost, "/api/v2/tests/", body)
	testAccNoError(t, err)
	_, err = client.Do(req, nil)
	testAccNoError(t, err)
}

// Regressions with products that pass their tags on, for an engagement
// without tags of its own:
//   - a move to another product replaces the old product's tags with the
//     new product's, and a refresh after it shows no tags;
//   - a change of the product's tags and of the engagement in one apply: the
//     product's change reaches the engagement later (never in the test
//     stack, which has no Celery), so the engagement still has the old
//     product tag after its update, which must not fail the apply.
func TestAccEngagement_inheritedTagsWithoutOwnTags(t *testing.T) {
	name := randName(t)
	suffix := name[len("acc-"):]
	tagA, tagB, tagB2 := "inh-a-"+suffix, "inh-b-"+suffix, "inh-b2-"+suffix
	var id int
	config := func(secondTag, product, description string) string {
		return fmt.Sprintf(`
resource "defectdojo_product_type" "test" {
  name = %[1]q
}

resource "defectdojo_product" "first" {
  name                           = "%[1]s-first"
  description                    = "acceptance test"
  product_type_id                = defectdojo_product_type.test.id
  enable_product_tag_inheritance = true
  tags                           = [%[2]q]
}

resource "defectdojo_product" "second" {
  name                           = "%[1]s-second"
  description                    = "acceptance test"
  product_type_id                = defectdojo_product_type.test.id
  enable_product_tag_inheritance = true
  tags                           = [%[3]q]
}

resource "defectdojo_engagement" "test" {
  product_id   = defectdojo_product.%[4]s.id
  name         = %[1]q
  description  = %[5]q
  target_start = "2026-01-01"
  target_end   = "2099-12-31"
}
`, name, tagA, secondTag, product, description)
	}
	noTags := statecheck.ExpectKnownValue(engagementAddress, tfjsonpath.New("tags"), knownvalue.SetSizeExact(0))
	inDefectDojo := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			e, _, err := testAccClient(t).Engagements.Get(context.Background(), id)
			if err != nil {
				return err
			}
			if len(e.Tags) != 1 || !strings.EqualFold(e.Tags[0], want) {
				return fmt.Errorf("DefectDojo tags = %v, want [%s]", e.Tags, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:            config(tagB, "first", "one"),
				ConfigStateChecks: []statecheck.StateCheck{noTags},
				Check:             resource.ComposeAggregateTestCheckFunc(testAccCaptureEngagementAddressID(&id), inDefectDojo(tagA)),
			},
			// Moved without tags of its own: the new product's tag replaces
			// the old one, and stays hidden, also after a refresh.
			{
				Config:            config(tagB, "second", "one"),
				ConfigStateChecks: []statecheck.StateCheck{noTags},
				Check:             inDefectDojo(tagB),
			},
			{
				Config:            config(tagB, "second", "two"),
				ConfigStateChecks: []statecheck.StateCheck{noTags},
			},
			// The product's tag and the engagement change in one apply. The
			// engagement keeps the old tag in DefectDojo until Celery passes
			// the change on; a refresh then shows it (Optional+Computed, so no
			// diff).
			{
				Config: config(tagB2, "second", "three"),
				Check:  inDefectDojo(tagB),
			},
		},
	})
}
