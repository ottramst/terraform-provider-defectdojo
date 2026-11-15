package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

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

func TestAccProductPlatform(t *testing.T)  { testAccProductAttribute(t, productPlatformKind) }
func TestAccProductLifecycle(t *testing.T) { testAccProductAttribute(t, productLifecycleKind) }
func TestAccProductOrigin(t *testing.T)    { testAccProductAttribute(t, productOriginKind) }

// testAccProductAttribute tests the lifecycle of one kind of product
// attribute.
func testAccProductAttribute(t *testing.T, kind productAttributeKind) {
	value := randName(t)
	address := "defectdojo_" + kind.typeName + ".test"
	var id int
	config := func(value, extra string) string {
		if extra == "" {
			extra = `  name = "Acceptance"` + "\n"
		}
		return fmt.Sprintf(`
resource "defectdojo_%s" "test" {
  value = %q
%s}
`, kind.typeName, value, extra)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Resource identity needs Terraform 1.12+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		CheckDestroy: testAccCheckProductAttributeDestroyed(t, kind, &id),
		Steps: []resource.TestStep{
			// Create with the defaults
			{
				Config: config(value, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("value"), knownvalue.StringExact(value)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact("Acceptance")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("icon"), knownvalue.Null()),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("display_order"), knownvalue.Int64Exact(0)),
					statecheck.ExpectIdentityValueMatchesState(address, tfjsonpath.New("id")),
				},
				Check: testAccCaptureProductAttributeID(address, &id),
			},
			// Update the other attributes in place
			{
				Config: config(value, `  name          = "Renamed"
  icon          = "flask"
  display_order = 99
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact("Renamed")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("icon"), knownvalue.StringExact("flask")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("display_order"), knownvalue.Int64Exact(99)),
				},
			},
			// Import by ID
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				ResourceName:    address,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// The configuration -generate-config-out writes for an import plans no
			// changes. terraform query writes it from the list resource's state,
			// which TestListResourceStates checks is the same.
			{
				ResourceName:    address,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				GenerateConfig:  true,
			},
			// Removing the optional attributes clears them
			{
				Config: config(value, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("icon"), knownvalue.Null()),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("display_order"), knownvalue.Int64Exact(0)),
				},
			},
			// Changed outside Terraform: the change is reverted
			{
				PreConfig: func() {
					_, _, err := kind.service(testAccClient(t)).Patch(context.Background(), id, &defectdojo.ProductAttributePatch{
						Name: new("Changed"), Icon: new("star"),
					}, nil)
					testAccNoError(t, err)
				},
				Config: config(value, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact("Acceptance")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("icon"), knownvalue.Null()),
				},
			},
			// The value can't change: a change replaces the entry
			{
				Config: config(value+"-changed", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("value"), knownvalue.StringExact(value+"-changed")),
				},
				Check: testAccCaptureProductAttributeID(address, &id),
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := kind.service(testAccClient(t)).Delete(context.Background(), id, nil)
					testAccNoError(t, err)
				},
				Config: config(value+"-changed", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureProductAttributeID(address, &id),
			},
		},
	})
}

// A product uses a managed platform. Destroying the platform fails while the
// product uses it, and a change of value with create_before_destroy moves the
// product to the new entry before the old one is deleted.
func TestAccProductPlatform_inUse(t *testing.T) {
	testAccSkipUnlessAcc(t)
	value := randName(t)
	productTypeID := testAccCreateProductType(t)
	var id, outsideID int
	config := func(value string) string {
		return fmt.Sprintf(`
resource "defectdojo_product_platform" "test" {
  value = %[1]q
  name  = "Kiosk"

  lifecycle {
    create_before_destroy = true
  }
}

resource "defectdojo_product" "test" {
  name            = %[2]q
  description     = "acceptance test"
  product_type_id = %[3]d
  platform        = defectdojo_product_platform.test.value
}
`, value, value, productTypeID)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProductAttributeDestroyed(t, productPlatformKind, &id),
		Steps: []resource.TestStep{
			{
				Config: config(value),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("defectdojo_product.test", tfjsonpath.New("platform"),
						"defectdojo_product_platform.test", tfjsonpath.New("value"), compare.ValuesSame()),
				},
				Check: testAccCaptureProductAttributeID("defectdojo_product_platform.test", &id),
			},
			// A new value: the entry is replaced, new one first, and the
			// product moves to it.
			{
				Config: config(value + "-v2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("defectdojo_product_platform.test", plancheck.ResourceActionCreateBeforeDestroy),
						plancheck.ExpectResourceAction("defectdojo_product.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("defectdojo_product.test", tfjsonpath.New("platform"), knownvalue.StringExact(value+"-v2")),
				},
				Check: testAccCaptureProductAttributeID("defectdojo_product_platform.test", &id),
			},
			// Destroying the entry while a product made outside Terraform
			// uses it fails; Terraform destroys its own product first.
			{
				PreConfig: func() {
					p, _, err := testAccClient(t).Products.Create(context.Background(), &defectdojo.Product{
						Name: randName(t), Description: "acceptance test", ProdType: productTypeID, Platform: value + "-v2",
					}, nil)
					testAccNoError(t, err)
					outsideID = p.ID
				},
				Config:      config(value + "-v2"),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`(?s)can't\s+be\s+deleted\s+while\s+products\s+use\s+it\s+in\s+platform`),
			},
			// Once that product is gone, the destroy after the test succeeds.
			{
				PreConfig: func() {
					_, err := testAccClient(t).Products.Delete(context.Background(), outsideID, nil)
					testAccNoError(t, err)
				},
				Config: config(value + "-v2"),
			},
		},
	})
}

// testAccCaptureProductAttributeID stores the ID of the entry at address in
// id.
func testAccCaptureProductAttributeID(address string, id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("%s not found in state", address)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckProductAttributeDestroyed checks that the entry is gone.
func testAccCheckProductAttributeDestroyed(t *testing.T, kind productAttributeKind, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := kind.service(testAccClient(t)).Get(context.Background(), *id, nil)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("%s %d still exists", kind.object, *id)
	}
}
