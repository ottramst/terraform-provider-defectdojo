package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
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

const developmentEnvironmentAddress = "defectdojo_development_environment.test"

func TestAccDevelopmentEnvironment(t *testing.T) {
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
		CheckDestroy: testAccCheckDevelopmentEnvironmentDestroyed(t, &id),
		Steps: []resource.TestStep{
			{
				Config: testAccDevelopmentEnvironmentConfig(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(developmentEnvironmentAddress, tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectIdentityValueMatchesState(developmentEnvironmentAddress, tfjsonpath.New("id")),
				},
				Check: testAccCaptureDevelopmentEnvironmentID(&id),
			},
			// Rename in place
			{
				Config: testAccDevelopmentEnvironmentConfig(renamed),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(developmentEnvironmentAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(developmentEnvironmentAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
				},
			},
			// Import by ID
			{
				ResourceName:      developmentEnvironmentAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				ResourceName:    developmentEnvironmentAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// The configuration -generate-config-out writes for an import plans no
			// changes. terraform query writes it from the list resource's state,
			// which TestListResourceStates checks is the same.
			{
				ResourceName:    developmentEnvironmentAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				GenerateConfig:  true,
			},
			// Changed outside Terraform: the change is reverted
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).DevelopmentEnvironments.Patch(context.Background(), id, &defectdojo.DevelopmentEnvironmentPatch{
						Name: new(name + "-changed"),
					})
					testAccNoError(t, err)
				},
				Config: testAccDevelopmentEnvironmentConfig(renamed),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(developmentEnvironmentAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(developmentEnvironmentAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
				},
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).DevelopmentEnvironments.Delete(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: testAccDevelopmentEnvironmentConfig(renamed),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(developmentEnvironmentAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureDevelopmentEnvironmentID(&id),
			},
		},
	})
}

// A name that an environment already has is refused, on create and on a
// rename, with the import hint.
func TestAccDevelopmentEnvironment_nameTaken(t *testing.T) {
	name := randName(t)
	var id int
	taken := regexp.MustCompile(`(?s)already\s+has\s+a\s+development\s+environment\s+named\s+"Development".*import\s+the\s+existing`)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDevelopmentEnvironmentDestroyed(t, &id),
		Steps: []resource.TestStep{
			{
				Config:      testAccDevelopmentEnvironmentConfig("Development"),
				ExpectError: taken,
			},
			{
				Config: testAccDevelopmentEnvironmentConfig(name),
				Check:  testAccCaptureDevelopmentEnvironmentID(&id),
			},
			{
				Config:      testAccDevelopmentEnvironmentConfig("Development"),
				ExpectError: taken,
			},
			// A valid config last, for the destroy after the test.
			{
				Config: testAccDevelopmentEnvironmentConfig(name),
			},
		},
	})
}

// An environment that a test uses can't be destroyed; the error says why.
func TestAccDevelopmentEnvironment_inUse(t *testing.T) {
	testAccSkipUnlessAcc(t)
	name := randName(t)
	var id, testID int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDevelopmentEnvironmentDestroyed(t, &id),
		Steps: []resource.TestStep{
			// A test in the environment, created outside Terraform.
			{
				Config: testAccDevelopmentEnvironmentConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureDevelopmentEnvironmentID(&id),
					func(*terraform.State) error {
						testID = testAccCreateTestOutside(t, map[string]any{"environment": id})
						return nil
					},
				),
			},
			{
				Config:      testAccDevelopmentEnvironmentConfig(name),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`(?s)can't\s+be\s+deleted\s+while\s+tests\s+use\s+it`),
			},
			// Once the test is gone, the destroy after the test succeeds.
			{
				PreConfig: func() {
					client := testAccClient(t)
					req, err := client.NewRequest(context.Background(), http.MethodDelete, "/api/v2/tests/"+strconv.Itoa(testID)+"/", nil)
					testAccNoError(t, err)
					_, err = client.Do(req, nil)
					testAccNoError(t, err)
				},
				Config: testAccDevelopmentEnvironmentConfig(name),
			},
		},
	})
}

func testAccDevelopmentEnvironmentConfig(name string) string {
	return fmt.Sprintf(`
resource "defectdojo_development_environment" "test" {
  name = %q
}
`, name)
}

// testAccCaptureDevelopmentEnvironmentID stores the ID of the development
// environment in id.
func testAccCaptureDevelopmentEnvironmentID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[developmentEnvironmentAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", developmentEnvironmentAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckDevelopmentEnvironmentDestroyed checks that the development
// environment is gone.
func testAccCheckDevelopmentEnvironmentDestroyed(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := testAccClient(t).DevelopmentEnvironments.Get(context.Background(), *id)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("development environment %d still exists", *id)
	}
}
