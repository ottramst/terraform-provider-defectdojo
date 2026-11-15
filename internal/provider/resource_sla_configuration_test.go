package provider

import (
	"context"
	"errors"
	"fmt"
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

const slaConfigurationAddress = "defectdojo_sla_configuration.test"

// The days of an SLA configuration change at most once per configuration:
// DefectDojo then recalculates the findings' SLA dates in the background, and
// the test stack has no Celery worker to finish that.
func TestAccSLAConfiguration(t *testing.T) {
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
		CheckDestroy: testAccCheckSLAConfigurationDestroyed(t, &id),
		Steps: []resource.TestStep{
			// Create with the defaults
			{
				Config: testAccSLAConfigurationConfig(name, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("critical"), knownvalue.Int64Exact(7)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("high"), knownvalue.Int64Exact(30)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("medium"), knownvalue.Int64Exact(90)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("low"), knownvalue.Int64Exact(120)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("enforce_critical"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("enforce_low"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("restart_sla_on_reactivation"), knownvalue.Bool(false)),
					statecheck.ExpectIdentityValueMatchesState(slaConfigurationAddress, tfjsonpath.New("id")),
				},
				Check: testAccCaptureSLAConfigurationID(&id),
			},
			// Update the attributes that the SLA lock doesn't cover
			{
				Config: testAccSLAConfigurationConfig(renamed, `
  description                 = "Internet-facing products"
  restart_sla_on_reactivation = true
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(slaConfigurationAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("description"), knownvalue.StringExact("Internet-facing products")),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("restart_sla_on_reactivation"), knownvalue.Bool(true)),
				},
			},
			// Change the days and enforce flags, once
			{
				Config: testAccSLAConfigurationConfig(renamed, testAccSLAConfigurationDays),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(slaConfigurationAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("critical"), knownvalue.Int64Exact(3)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("high"), knownvalue.Int64Exact(14)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("medium"), knownvalue.Int64Exact(60)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("low"), knownvalue.Int64Exact(90)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("enforce_low"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("restart_sla_on_reactivation"), knownvalue.Bool(false)),
				},
			},
			// Import by ID
			{
				ResourceName:      slaConfigurationAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				ResourceName:    slaConfigurationAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// The configuration -generate-config-out writes for an import plans no
			// changes. terraform query writes it from the list resource's state,
			// which TestListResourceStates checks is the same.
			{
				ResourceName:    slaConfigurationAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				GenerateConfig:  true,
			},
			// Changed outside Terraform: the change is reverted. The days are
			// locked now, so the change is to the name.
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).SLAConfigurations.Patch(context.Background(), id, &defectdojo.SLAConfigurationPatch{
						Name: new(name + "-changed"), RestartSLAOnReactivation: new(true),
					}, nil)
					testAccNoError(t, err)
				},
				Config: testAccSLAConfigurationConfig(renamed, testAccSLAConfigurationDays),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(slaConfigurationAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("restart_sla_on_reactivation"), knownvalue.Bool(false)),
				},
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).SLAConfigurations.Delete(context.Background(), id, nil)
					testAccNoError(t, err)
				},
				Config: testAccSLAConfigurationConfig(renamed, testAccSLAConfigurationDays),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(slaConfigurationAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureSLAConfigurationID(&id),
			},
		},
	})
}

// A second change of the days before DefectDojo has recalculated the SLA
// dates of the first fails with an explanation.
func TestAccSLAConfiguration_daysLocked(t *testing.T) {
	name := randName(t)
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSLAConfigurationDestroyed(t, &id),
		Steps: []resource.TestStep{
			{
				Config: testAccSLAConfigurationConfig(name, ""),
				Check:  testAccCaptureSLAConfigurationID(&id),
			},
			{
				Config: testAccSLAConfigurationConfig(name, testAccSLAConfigurationDays),
			},
			{
				Config:      testAccSLAConfigurationConfig(name, "  critical = 1\n"),
				ExpectError: regexp.MustCompile(`(?s)DefectDojo\s+is\s+still\s+recalculating\s+the\s+SLA.*Apply\s+again\s+once`),
			},
			// The configuration stays locked: the days applied last make a
			// valid config for the destroy.
			{
				Config: testAccSLAConfigurationConfig(name, testAccSLAConfigurationDays),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// An SLA configuration that a product uses can't be deleted; the error says
// what to do.
func TestAccSLAConfiguration_inUse(t *testing.T) {
	testAccSkipUnlessAcc(t)
	name := randName(t)
	var id, productID int
	// Created before the product type, so that it is deleted after the
	// product type and its product, which refers to it.
	otherID := testAccCreateSLAConfiguration(t)
	productTypeID := testAccCreateProductType(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSLAConfigurationDestroyed(t, &id),
		Steps: []resource.TestStep{
			// A product created with the configuration, outside Terraform.
			{
				Config: testAccSLAConfigurationConfig(name, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureSLAConfigurationID(&id),
					func(*terraform.State) error {
						p, _, err := testAccClient(t).Products.Create(context.Background(), &defectdojo.Product{
							Name: randName(t), Description: "acceptance test", ProdType: productTypeID, SLAConfiguration: id,
						}, nil)
						productID = p.ID
						return err
					},
				),
			},
			{
				Config:      testAccSLAConfigurationConfig(name, ""),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`(?s)can't\s+be\s+deleted\s+while\s+products\s+use\s+it.*Move\s+the\s+products`),
			},
			// Once the product uses another configuration, the destroy after
			// the test succeeds.
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).Products.Patch(context.Background(), productID, &defectdojo.ProductPatch{
						SLAConfiguration: &otherID,
					}, nil)
					testAccNoError(t, err)
				},
				Config: testAccSLAConfigurationConfig(name, ""),
			},
		},
	})
}

// DefectDojo's Default SLA configuration can be imported and managed, and
// destroying it only removes it from the state.
func TestAccSLAConfiguration_default(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			sla, _, err := testAccClient(t).SLAConfigurations.Get(context.Background(), 1, nil)
			if err != nil {
				return fmt.Errorf("the Default SLA configuration: %w", err)
			}
			if sla.Name != "Default" {
				return fmt.Errorf("the Default SLA configuration is named %q", sla.Name)
			}
			return nil
		},
		Steps: []resource.TestStep{
			// Its current values, so that nothing changes or locks its days.
			{
				Config: `
import {
  to = defectdojo_sla_configuration.test
  id = "1"
}

resource "defectdojo_sla_configuration" "test" {
  name        = "Default"
  description = "The Default SLA Configuration. Products not using an explicit SLA Configuration will use this one."
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(slaConfigurationAddress, plancheck.ResourceActionNoop),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(slaConfigurationAddress, tfjsonpath.New("critical"), knownvalue.Int64Exact(7)),
				},
			},
		},
	})
}

const testAccSLAConfigurationDays = `
  critical    = 3
  high        = 14
  medium      = 60
  low         = 90
  enforce_low = false
`

func testAccSLAConfigurationConfig(name, extra string) string {
	return fmt.Sprintf(`
resource "defectdojo_sla_configuration" "test" {
  name = %[1]q
%[2]s}
`, name, extra)
}

// testAccCaptureSLAConfigurationID stores the ID of the SLA configuration in
// id.
func testAccCaptureSLAConfigurationID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[slaConfigurationAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", slaConfigurationAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckSLAConfigurationDestroyed checks that the SLA configuration is
// gone.
func testAccCheckSLAConfigurationDestroyed(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := testAccClient(t).SLAConfigurations.Get(context.Background(), *id, nil)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("SLA configuration %d still exists", *id)
	}
}
