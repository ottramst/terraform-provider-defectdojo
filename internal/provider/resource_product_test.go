package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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

const productAddress = "defectdojo_product.test"

func TestAccProduct(t *testing.T) {
	testAccSkipUnlessAcc(t)
	name := randName(t)
	renamed := name + "-renamed"
	tag := "Acc-" + name // a new tag, so DefectDojo keeps its spelling
	sla := testAccCreateSLAConfiguration(t)
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Resource identity needs Terraform 1.12+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		CheckDestroy: testAccCheckProductDestroyed(t, &id),
		Steps: []resource.TestStep{
			// Create with the defaults
			{
				Config: testAccProductConfig(name, "first", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("description"), knownvalue.StringExact("acceptance test")),
					statecheck.CompareValuePairs(productAddress, tfjsonpath.New("product_type_id"),
						"defectdojo_product_type.first", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("business_criticality"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("platform"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("user_records"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("revenue"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("external_audience"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("enable_full_risk_acceptance"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("sla_configuration_id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("product_manager_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("authorized_users"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("regulation_ids"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("tags"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("force_destroy"), knownvalue.Bool(false)),
					statecheck.ExpectIdentityValueMatchesState(productAddress, tfjsonpath.New("id")),
				},
				Check: testAccCaptureProductID(&id),
			},
			// Update every attribute in place, except the SLA configuration
			{
				Config: testAccProductConfig(renamed, "first", fmt.Sprintf(`
  business_criticality             = "high"
  platform                         = "web"
  life_cycle                       = "production"
  origin                           = "internal"
  user_records                     = 0
  revenue                          = "1234567890123.45"
  external_audience                = true
  internet_accessible              = true
  enable_product_tag_inheritance   = true
  enable_simple_risk_acceptance    = true
  enable_full_risk_acceptance      = false
  disable_sla_breach_notifications = true
  product_manager_id               = 1
  technical_contact_id             = 1
  team_manager_id                  = 1
  authorized_users                 = [1]
  regulation_ids                   = [1, 2]
  tags                             = [%q, "pci"]
`, tag)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(productAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("business_criticality"), knownvalue.StringExact("high")),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("platform"), knownvalue.StringExact("web")),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("life_cycle"), knownvalue.StringExact("production")),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("origin"), knownvalue.StringExact("internal")),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("user_records"), knownvalue.Int64Exact(0)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("revenue"), knownvalue.StringExact("1234567890123.45")),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("internet_accessible"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("enable_full_risk_acceptance"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("disable_sla_breach_notifications"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("team_manager_id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("authorized_users"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.Int64Exact(1)})),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("regulation_ids"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.Int64Exact(1), knownvalue.Int64Exact(2)})),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("tags"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact(tag), knownvalue.StringExact("pci")})),
				},
			},
			// Import by ID
			{
				ResourceName:      productAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				ResourceName:    productAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// The configuration -generate-config-out writes for an import plans no
			// changes. terraform query writes it from the list resource's state,
			// which TestListResourceStates checks is the same.
			{
				ResourceName:    productAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				GenerateConfig:  true,
			},
			// Change the SLA configuration (once: DefectDojo then recalculates
			// in the background, which never finishes without Celery), and
			// move the product to another product type.
			{
				Config: testAccProductConfig(renamed, "second", fmt.Sprintf(`
  sla_configuration_id = %d
`, sla)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(productAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("sla_configuration_id"), knownvalue.Int64Exact(int64(sla))),
					statecheck.CompareValuePairs(productAddress, tfjsonpath.New("product_type_id"),
						"defectdojo_product_type.second", tfjsonpath.New("id"), compare.ValuesSame()),
					// Unset optional attributes are cleared; unset sets are kept.
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("business_criticality"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("platform"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("user_records"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("revenue"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("team_manager_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("enable_full_risk_acceptance"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("authorized_users"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("regulation_ids"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("tags"), knownvalue.SetSizeExact(2)),
				},
				Check: testAccCheckProductID(&id),
			},
			// Changed outside Terraform: the change is reverted
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).Products.Patch(context.Background(), id, &defectdojo.ProductPatch{
						Name: new(name + "-changed"), Platform: defectdojo.Value("mobile"),
					})
					testAccNoError(t, err)
				},
				Config: testAccProductConfig(renamed, "second", fmt.Sprintf("  sla_configuration_id = %d\n", sla)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(productAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("platform"), knownvalue.Null()),
				},
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).Products.Delete(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: testAccProductConfig(renamed, "second", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(productAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureProductID(&id),
			},
		},
	})
}

// A product with engagements is not destroyed unless force_destroy is set,
// because DefectDojo would delete the engagements too.
func TestAccProduct_forceDestroy(t *testing.T) {
	name := randName(t)
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProductDestroyed(t, &id),
		Steps: []resource.TestStep{
			{
				Config: testAccProductConfig(name, "first", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureProductID(&id),
					func(*terraform.State) error {
						testAccCreateEngagement(t, id)
						return nil
					},
				),
			},
			// Destroying is refused while the product has an engagement.
			{
				Config:      testAccProductConfig(name, "first", ""),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`has 1 engagements`),
			},
			// With force_destroy, the product and its engagement are deleted
			// (by the test case's destroy).
			{
				Config: testAccProductConfig(name, "first", "  force_destroy = true\n"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("force_destroy"), knownvalue.Bool(true)),
				},
			},
		},
	})
}

// DefectDojo gives a tag the spelling of an existing product tag that matches
// it ignoring case; the provider keeps the configured spelling, so there is
// no diff. Invalid tags fail at plan time.
func TestAccProduct_tags(t *testing.T) {
	testAccSkipUnlessAcc(t)
	name := randName(t)
	existing := "acc-tag-" + name[len("acc-"):]
	client := testAccClient(t)
	ptID := testAccCreateProductType(t)
	_, _, err := client.Products.Create(context.Background(), &defectdojo.Product{
		Name: randName(t), Description: "holds the tag", ProdType: ptID, Tags: []string{existing},
	})
	testAccNoError(t, err)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccProductConfig(name, "first", `  tags = ["foo bar"]`+"\n"),
				ExpectError: regexp.MustCompile(`must not contain spaces, commas or quotes`),
			},
			{
				Config:      testAccProductConfig(name, "first", `  tags = ["X", "x"]`+"\n"),
				ExpectError: regexp.MustCompile(`differ only in case`),
			},
			{
				Config: testAccProductConfig(name, "first", fmt.Sprintf("  tags = [%q]\n", strings.ToUpper(existing))),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productAddress, tfjsonpath.New("tags"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact(strings.ToUpper(existing))})),
				},
				Check: func(s *terraform.State) error {
					var id int
					if err := testAccCaptureProductID(&id)(s); err != nil {
						return err
					}
					p, _, err := client.Products.Get(context.Background(), id)
					if err != nil {
						return err
					}
					if len(p.Tags) != 1 || p.Tags[0] != existing {
						return fmt.Errorf("DefectDojo tags = %v, want [%s] (the existing spelling)", p.Tags, existing)
					}
					return nil
				},
			},
		},
	})
}

func TestAccProduct_invalidImportID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        testAccProductConfig("never-created", "first", ""),
				ResourceName:  productAddress,
				ImportState:   true,
				ImportStateId: "0",
				ExpectError:   regexp.MustCompile(`Expected the numeric ID of the product`),
			},
		},
	})
}

// testAccProductConfig returns a configuration with two product types, first
// and second, and a product of name in the product type productType.
func testAccProductConfig(name, productType, extra string) string {
	return fmt.Sprintf(`
resource "defectdojo_product_type" "first" {
  name = "%[1]s-first"
}

resource "defectdojo_product_type" "second" {
  name = "%[1]s-second"
}

resource "defectdojo_product" "test" {
  name            = %[1]q
  description     = "acceptance test"
  product_type_id = defectdojo_product_type.%[2]s.id
%[3]s}
`, name, productType, extra)
}

// testAccCaptureProductID stores the ID of the product in id.
func testAccCaptureProductID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[productAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", productAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckProductID checks that the product still has the ID id: it was
// updated in place.
func testAccCheckProductID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var got int
		if err := testAccCaptureProductID(&got)(s); err != nil {
			return err
		}
		if got != *id {
			return fmt.Errorf("product ID = %d, want %d (updated in place)", got, *id)
		}
		return nil
	}
}

// testAccCheckProductDestroyed checks that the product is gone.
func testAccCheckProductDestroyed(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := testAccClient(t).Products.Get(context.Background(), *id)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("product %d still exists", *id)
	}
}

// testAccCreateProductType creates a product type for a test and deletes it,
// with its products, when the test ends.
func testAccCreateProductType(t *testing.T) int {
	t.Helper()
	client := testAccClient(t)
	pt, _, err := client.ProductTypes.Create(context.Background(), &defectdojo.ProductType{Name: randName(t)})
	testAccNoError(t, err)
	t.Cleanup(func() {
		if _, err := client.ProductTypes.Delete(context.Background(), pt.ID); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	return pt.ID
}

// testAccCreateSLAConfiguration creates an SLA configuration with
// DefectDojo's default days. It is deleted when the test ends, after the
// test's destroy, which deletes the products that refer to it.
func testAccCreateSLAConfiguration(t *testing.T) int {
	t.Helper()
	client := testAccClient(t)
	sla, _, err := client.SLAConfigurations.Create(context.Background(), &defectdojo.SLAConfiguration{
		Name: randName(t), Critical: 7, High: 30, Medium: 90, Low: 120,
		EnforceCritical: true, EnforceHigh: true, EnforceMedium: true, EnforceLow: true,
	})
	testAccNoError(t, err)
	t.Cleanup(func() {
		if _, err := client.SLAConfigurations.Delete(context.Background(), sla.ID); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
			t.Logf("cleanup: %v", err)
		}
	})
	return sla.ID
}

// testAccCreateEngagement creates an engagement in the product. It is
// deleted with the product.
func testAccCreateEngagement(t *testing.T, productID int) {
	t.Helper()
	day := defectdojo.Date{Year: 2026, Month: time.January, Day: 1}
	_, _, err := testAccClient(t).Engagements.Create(context.Background(), &defectdojo.Engagement{
		Name: randName(t), Product: productID, TargetStart: day, TargetEnd: day,
	})
	testAccNoError(t, err)
}
