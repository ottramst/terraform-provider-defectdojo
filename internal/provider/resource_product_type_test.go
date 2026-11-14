package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/ottramst/go-defectdojo"
)

const productTypeAddress = "defectdojo_product_type.test"

func TestAccProductType(t *testing.T) {
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
		CheckDestroy: testAccCheckProductTypeDestroyed(t, &id),
		Steps: []resource.TestStep{
			// Create with the defaults
			{
				Config: testAccProductTypeConfig(name, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("critical_product"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("key_product"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("authorized_users"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("force_destroy"), knownvalue.Bool(false)),
					statecheck.ExpectIdentityValueMatchesState(productTypeAddress, tfjsonpath.New("id")),
				},
				Check: testAccCaptureProductTypeID(&id),
			},
			// Update every attribute in place
			{
				Config: testAccProductTypeConfig(renamed, `
  description      = "Customer-facing web applications"
  critical_product = true
  key_product      = true
  authorized_users = [1]
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(productTypeAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("description"), knownvalue.StringExact("Customer-facing web applications")),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("critical_product"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("authorized_users"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.Int64Exact(1)})),
				},
			},
			// Import by ID
			{
				ResourceName:            productTypeAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"force_destroy"},
			},
			// Import by identity
			{
				ResourceName:    productTypeAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// Unsetting authorized_users leaves them as they are; unsetting
			// the description clears it.
			{
				Config: testAccProductTypeConfig(renamed, `
  critical_product = true
  key_product      = true
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("authorized_users"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.Int64Exact(1)})),
				},
			},
			// Changed outside Terraform: the change is reverted
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).ProductTypes.Patch(context.Background(), id, &defectdojo.ProductTypePatch{
						Name: new(name + "-changed"), CriticalProduct: new(false),
					})
					testAccNoError(t, err)
				},
				Config: testAccProductTypeConfig(renamed, `
  critical_product = true
  key_product      = true
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(productTypeAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("critical_product"), knownvalue.Bool(true)),
				},
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).ProductTypes.Delete(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: testAccProductTypeConfig(renamed, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(productTypeAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureProductTypeID(&id),
			},
		},
	})
}

// A product type with products is not destroyed unless force_destroy is set,
// because DefectDojo would delete the products too.
func TestAccProductType_forceDestroy(t *testing.T) {
	name := randName(t)
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProductTypeDestroyed(t, &id),
		Steps: []resource.TestStep{
			{
				Config: testAccProductTypeConfig(name, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureProductTypeID(&id),
					func(*terraform.State) error {
						testAccCreateProduct(t, id)
						return nil
					},
				),
			},
			// Destroying is refused while the product type has a product.
			{
				Config:      testAccProductTypeConfig(name, ""),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`has 1 products`),
			},
			// With force_destroy, the product type and its product are deleted
			// (by the test case's destroy).
			{
				Config: testAccProductTypeConfig(name, "  force_destroy = true\n"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("force_destroy"), knownvalue.Bool(true)),
				},
			},
		},
	})
}

func TestAccProductType_invalidImportID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        testAccProductTypeConfig("never-created", ""),
				ResourceName:  productTypeAddress,
				ImportState:   true,
				ImportStateId: "abc",
				ExpectError:   regexp.MustCompile(`Expected the numeric ID of the product type`),
			},
		},
	})
}

func testAccProductTypeConfig(name, extra string) string {
	return fmt.Sprintf(`
resource "defectdojo_product_type" "test" {
  name = %[1]q
%[2]s}
`, name, extra)
}

// testAccCaptureProductTypeID stores the ID of the product type in id.
func testAccCaptureProductTypeID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[productTypeAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", productTypeAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckProductTypeDestroyed checks that the product type is gone.
func testAccCheckProductTypeDestroyed(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := testAccClient(t).ProductTypes.Get(context.Background(), *id)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("product type %d still exists", *id)
	}
}

// testAccCreateProduct creates a product in the product type through the raw
// API, until the client has a Products service.
func testAccCreateProduct(t *testing.T, productTypeID int) {
	t.Helper()
	client := testAccClient(t)
	body := map[string]any{"name": randName(t), "description": "acceptance test", "prod_type": productTypeID}
	req, err := client.NewRequest(context.Background(), http.MethodPost, "/api/v2/products/", body)
	testAccNoError(t, err)
	_, err = client.Do(req, nil)
	testAccNoError(t, err)
}
