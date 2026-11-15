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

const cicdInfrastructureAddress = "defectdojo_cicd_infrastructure.test"

func TestAccCICDInfrastructure(t *testing.T) {
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
		CheckDestroy: testAccCheckCICDInfrastructureDestroyed(t, &id),
		Steps: []resource.TestStep{
			// Create with the required attributes
			{
				Config: testAccCICDInfrastructureConfig(name, "build_server", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("infrastructure_type"), knownvalue.StringExact("build_server")),
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("url"), knownvalue.Null()),
					statecheck.ExpectIdentityValueMatchesState(cicdInfrastructureAddress, tfjsonpath.New("id")),
				},
				Check: testAccCaptureCICDInfrastructureID(&id),
			},
			// Update the other attributes in place
			{
				Config: testAccCICDInfrastructureConfig(renamed, "build_server", `
  description = "The main build server."
  url         = "https://jenkins.example.com"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(cicdInfrastructureAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("description"), knownvalue.StringExact("The main build server.")),
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("url"), knownvalue.StringExact("https://jenkins.example.com")),
				},
			},
			// Import by ID
			{
				ResourceName:      cicdInfrastructureAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				ResourceName:    cicdInfrastructureAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// Removing the optional attributes clears them
			{
				Config: testAccCICDInfrastructureConfig(renamed, "build_server", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("description"), knownvalue.Null()),
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("url"), knownvalue.Null()),
				},
			},
			// Changed outside Terraform: the change is reverted
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).CICDInfrastructure.Patch(context.Background(), id, &defectdojo.CICDInfrastructurePatch{
						Name: new(name + "-changed"), URL: new("https://changed.example.com"),
					})
					testAccNoError(t, err)
				},
				Config: testAccCICDInfrastructureConfig(renamed, "build_server", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(cicdInfrastructureAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("url"), knownvalue.Null()),
				},
			},
			// The type can't change: a change replaces the object
			{
				Config: testAccCICDInfrastructureConfig(renamed, "orchestration", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(cicdInfrastructureAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(cicdInfrastructureAddress, tfjsonpath.New("infrastructure_type"), knownvalue.StringExact("orchestration")),
				},
				Check: testAccCaptureCICDInfrastructureID(&id),
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).CICDInfrastructure.Delete(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: testAccCICDInfrastructureConfig(renamed, "orchestration", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(cicdInfrastructureAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureCICDInfrastructureID(&id),
			},
		},
	})
}

// A create or a rename onto a name that another object of the type has fails
// with an explanation; the same name with another type is fine.
func TestAccCICDInfrastructure_nameTaken(t *testing.T) {
	name := randName(t)
	var id int
	config := func(second string) string {
		return testAccCICDInfrastructureConfig(name, "scm_server", "") + fmt.Sprintf(`
resource "defectdojo_cicd_infrastructure" "second" {
  name                = %q
  infrastructure_type = "scm_server"
}

resource "defectdojo_cicd_infrastructure" "build" {
  name                = %q
  infrastructure_type = "build_server"
}
`, second, name)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCICDInfrastructureDestroyed(t, &id),
		Steps: []resource.TestStep{
			{
				Config: config(name + "-second"),
				Check:  testAccCaptureCICDInfrastructureID(&id),
			},
			{
				Config:      config(name),
				ExpectError: regexp.MustCompile(`(?s)Another\s+CI/CD\s+infrastructure\s+object\s+of\s+type\s+scm_server\s+is\s+named`),
			},
			{
				Config: config(name+"-second") + fmt.Sprintf(`
resource "defectdojo_cicd_infrastructure" "third" {
  name                = %q
  infrastructure_type = "scm_server"
}
`, name),
				ExpectError: regexp.MustCompile(`(?s)Another\s+CI/CD\s+infrastructure\s+object\s+of\s+type\s+scm_server\s+is\s+named`),
			},
			{
				Config: config(name + "-second"),
			},
		},
	})
}

// An engagement links to one object of each type; removing the links clears
// them, and a link cleared outside Terraform is put back.
func TestAccEngagement_cicdInfrastructure(t *testing.T) {
	testAccSkipUnlessAcc(t)
	name := randName(t)
	productTypeID := testAccCreateProductType(t)

	infrastructure := fmt.Sprintf(`
resource "defectdojo_cicd_infrastructure" "scm" {
  name                = %[1]q
  infrastructure_type = "scm_server"
}

resource "defectdojo_cicd_infrastructure" "build" {
  name                = %[1]q
  infrastructure_type = "build_server"
}

resource "defectdojo_cicd_infrastructure" "orchestration" {
  name                = %[1]q
  infrastructure_type = "orchestration"
}

resource "defectdojo_product" "test" {
  name            = %[1]q
  description     = "acceptance test"
  product_type_id = %[2]d
  force_destroy   = true
}
`, name, productTypeID)
	engagement := func(links string) string {
		return infrastructure + fmt.Sprintf(`
resource "defectdojo_engagement" "test" {
  product_id   = defectdojo_product.test.id
  name         = %q
  target_start = "2026-01-01"
  target_end   = "2026-01-31"
%s}
`, name, links)
	}
	linked := engagement(`
  cicd_scm_server_id           = defectdojo_cicd_infrastructure.scm.id
  cicd_build_server_id         = defectdojo_cicd_infrastructure.build.id
  cicd_orchestration_engine_id = defectdojo_cicd_infrastructure.orchestration.id
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: linked + `
data "defectdojo_engagement" "test" {
  id = defectdojo_engagement.test.id
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("defectdojo_engagement.test", tfjsonpath.New("cicd_scm_server_id"),
						"defectdojo_cicd_infrastructure.scm", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs("defectdojo_engagement.test", tfjsonpath.New("cicd_build_server_id"),
						"defectdojo_cicd_infrastructure.build", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs("defectdojo_engagement.test", tfjsonpath.New("cicd_orchestration_engine_id"),
						"defectdojo_cicd_infrastructure.orchestration", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs("data.defectdojo_engagement.test", tfjsonpath.New("cicd_build_server_id"),
						"defectdojo_cicd_infrastructure.build", tfjsonpath.New("id"), compare.ValuesSame()),
				},
			},
			// An object of the wrong type is DefectDojo's 400.
			{
				Config: engagement(`
  cicd_scm_server_id = defectdojo_cicd_infrastructure.build.id
`),
				ExpectError: regexp.MustCompile(`cicd_scm_server:\s+Invalid\s+pk`),
			},
			// The SCM server deleted outside Terraform: DefectDojo clears the
			// link, and Terraform creates the object again and links it.
			{
				PreConfig: func() {
					client := testAccClient(t)
					ci, _, err := client.CICDInfrastructure.GetByName(context.Background(), name, defectdojo.CICDInfrastructureTypeSCMServer)
					testAccNoError(t, err)
					_, err = client.CICDInfrastructure.Delete(context.Background(), ci.ID)
					testAccNoError(t, err)
				},
				Config: linked,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("defectdojo_cicd_infrastructure.scm", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("defectdojo_engagement.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("defectdojo_engagement.test", tfjsonpath.New("cicd_scm_server_id"),
						"defectdojo_cicd_infrastructure.scm", tfjsonpath.New("id"), compare.ValuesSame()),
				},
			},
			// Removing the links clears them.
			{
				Config: engagement(""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("defectdojo_engagement.test", tfjsonpath.New("cicd_scm_server_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue("defectdojo_engagement.test", tfjsonpath.New("cicd_build_server_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue("defectdojo_engagement.test", tfjsonpath.New("cicd_orchestration_engine_id"), knownvalue.Null()),
				},
			},
		},
	})
}

func testAccCICDInfrastructureConfig(name, infrastructureType, extra string) string {
	return fmt.Sprintf(`
resource "defectdojo_cicd_infrastructure" "test" {
  name                = %q
  infrastructure_type = %q
%s}
`, name, infrastructureType, extra)
}

// testAccCaptureCICDInfrastructureID stores the ID of the CI/CD
// infrastructure object in id.
func testAccCaptureCICDInfrastructureID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[cicdInfrastructureAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", cicdInfrastructureAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckCICDInfrastructureDestroyed checks that the CI/CD
// infrastructure object is gone.
func testAccCheckCICDInfrastructureDestroyed(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := testAccClient(t).CICDInfrastructure.Get(context.Background(), *id)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("CI/CD infrastructure object %d still exists", *id)
	}
}
