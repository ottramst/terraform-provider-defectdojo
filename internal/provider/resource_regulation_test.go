package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

const regulationAddress = "defectdojo_regulation.test"

func TestAccRegulation(t *testing.T) {
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
		CheckDestroy: testAccCheckRegulationDestroyed(t, &id),
		Steps: []resource.TestStep{
			// Create with the required attributes
			{
				Config: testAccRegulationConfig(name, "privacy", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("acronym"), knownvalue.StringExact(testAccRegulationAcronym(name))),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("category"), knownvalue.StringExact("privacy")),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("jurisdiction"), knownvalue.StringExact("European Union")),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("reference"), knownvalue.Null()),
					statecheck.ExpectIdentityValueMatchesState(regulationAddress, tfjsonpath.New("id")),
				},
				Check: testAccCaptureRegulationID(&id),
			},
			// Update every attribute in place
			{
				Config: testAccRegulationConfig(renamed, "security", `
  jurisdiction = "Estonia"
  description  = "Rules for the handling of card data."
  reference    = "https://example.com/regulation"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(regulationAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("category"), knownvalue.StringExact("security")),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("jurisdiction"), knownvalue.StringExact("Estonia")),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("description"), knownvalue.StringExact("Rules for the handling of card data.")),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("reference"), knownvalue.StringExact("https://example.com/regulation")),
				},
			},
			// Import by ID
			{
				ResourceName:      regulationAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				ResourceName:    regulationAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// Removing the optional attributes clears them
			{
				Config: testAccRegulationConfig(renamed, "security", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("reference"), knownvalue.Null()),
				},
			},
			// Changed outside Terraform: the change is reverted
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).Regulations.Patch(context.Background(), id, &defectdojo.RegulationPatch{
						Jurisdiction: new("Elsewhere"), Description: new("changed"),
					})
					testAccNoError(t, err)
				},
				Config: testAccRegulationConfig(renamed, "security", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(regulationAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("jurisdiction"), knownvalue.StringExact("European Union")),
					statecheck.ExpectKnownValue(regulationAddress, tfjsonpath.New("description"), knownvalue.Null()),
				},
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).Regulations.Delete(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: testAccRegulationConfig(renamed, "security", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(regulationAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureRegulationID(&id),
			},
		},
	})
}

// A product lists a managed regulation; destroying the regulation removes it
// from the product, which then shows a change.
func TestAccRegulation_product(t *testing.T) {
	testAccSkipUnlessAcc(t)
	name := randName(t)
	productTypeID := testAccCreateProductType(t)
	var id int

	product := func(regulations string) string {
		return fmt.Sprintf(`
resource "defectdojo_product" "test" {
  name            = %[1]q
  description     = "regulated"
  product_type_id = %[2]d
  regulation_ids  = %[3]s
}
`, name, productTypeID, regulations)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRegulationDestroyed(t, &id),
		Steps: []resource.TestStep{
			{
				Config: testAccRegulationConfig(name, "finance", "") + product("[defectdojo_regulation.test.id, 1]"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValueCollection("defectdojo_product.test", []tfjsonpath.Path{tfjsonpath.New("regulation_ids")},
						regulationAddress, tfjsonpath.New("id"), compare.ValuesSame()),
				},
				Check: testAccCaptureRegulationID(&id),
			},
			// Deleted outside Terraform: DefectDojo removes it from the
			// product, and Terraform puts both back.
			{
				PreConfig: func() {
					_, err := testAccClient(t).Regulations.Delete(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: testAccRegulationConfig(name, "finance", "") + product("[defectdojo_regulation.test.id, 1]"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(regulationAddress, plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("defectdojo_product.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("defectdojo_product.test", tfjsonpath.New("regulation_ids"), knownvalue.SetSizeExact(2)),
				},
				Check: testAccCaptureRegulationID(&id),
			},
		},
	})
}

// testAccRegulationConfig returns a regulation whose acronym is the random
// part of name, so that it stays within 20 characters when name grows. extra
// may set jurisdiction, which defaults to "European Union".
func testAccRegulationConfig(name, category, extra string) string {
	jurisdiction := ""
	if !strings.Contains(extra, "jurisdiction") {
		jurisdiction = `  jurisdiction = "European Union"` + "\n"
	}
	return fmt.Sprintf(`
resource "defectdojo_regulation" "test" {
  name         = %[1]q
  acronym      = %[2]q
  category     = %[3]q
%[4]s%[5]s}
`, name, testAccRegulationAcronym(name), category, jurisdiction, extra)
}

// testAccRegulationAcronym returns the acronym testAccRegulationConfig uses.
func testAccRegulationAcronym(name string) string {
	return strings.ToUpper(strings.TrimPrefix(strings.SplitN(name, "-renamed", 2)[0], "acc-"))
}

// testAccCaptureRegulationID stores the ID of the regulation in id.
func testAccCaptureRegulationID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[regulationAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", regulationAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckRegulationDestroyed checks that the regulation is gone.
func testAccCheckRegulationDestroyed(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := testAccClient(t).Regulations.Get(context.Background(), *id)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("regulation %d still exists", *id)
	}
}
