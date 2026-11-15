package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
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

const userAddress = "defectdojo_user.test"

// testAccPassword meets DefectDojo's default password rules.
const testAccPassword = "Acc-Test-Passw0rd!"

func TestAccUser(t *testing.T) {
	username := randName(t)
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Resource identity needs Terraform 1.12+, write-only attributes 1.11+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		CheckDestroy: testAccCheckUserDestroyed(t, &id),
		Steps: []resource.TestStep{
			// Create with the defaults and a password
			{
				Config: testAccUserConfig(username, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("username"), knownvalue.StringExact(username)),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("email"), knownvalue.StringExact(username+"@example.com")),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("first_name"), knownvalue.Null()),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("is_active"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("is_staff"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("is_superuser"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("configuration_permissions"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("date_joined"), knownvalue.NotNull()),
					// The password is never in the state.
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("password_wo"), knownvalue.Null()),
					statecheck.ExpectIdentityValueMatchesState(userAddress, tfjsonpath.New("id")),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureUserID(&id),
					// The password works.
					func(*terraform.State) error {
						_, _, err := testAccClient(t).Auth.Token(context.Background(), username, testAccPassword, nil)
						return err
					},
				),
			},
			// Update every attribute in place
			{
				Config: testAccUserConfig(username+"-renamed", `
  email                     = "renamed@example.com"
  first_name                = "Dependency"
  last_name                 = "Track"
  is_staff                  = true
  is_superuser              = true
  is_active                 = false
  configuration_permissions = ["add_user", "change_sla_configuration"]
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(userAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("username"), knownvalue.StringExact(username+"-renamed")),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("email"), knownvalue.StringExact("renamed@example.com")),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("first_name"), knownvalue.StringExact("Dependency")),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("last_name"), knownvalue.StringExact("Track")),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("is_staff"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("is_superuser"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("is_active"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("configuration_permissions"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("add_user"), knownvalue.StringExact("change_sla_configuration"),
					})),
				},
			},
			// Import by ID
			{
				ResourceName:      userAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				ResourceName:    userAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// The configuration -generate-config-out writes for an import plans no
			// changes. terraform query writes it from the list resource's state,
			// which TestListResourceStates checks is the same.
			{
				ResourceName:    userAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				GenerateConfig:  true,
			},
			// Unsetting the permissions leaves them as they are; unsetting a
			// name clears it.
			{
				Config: testAccUserConfig(username+"-renamed", `
  email     = "renamed@example.com"
  last_name = "Track"
  is_staff  = true
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("first_name"), knownvalue.Null()),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("configuration_permissions"), knownvalue.SetSizeExact(2)),
				},
				Check: func(*terraform.State) error {
					u, _, err := testAccClient(t).Users.Get(context.Background(), id, nil)
					if err != nil {
						return err
					}
					if u.FirstName != "" {
						return fmt.Errorf("first name = %q, want it cleared", u.FirstName)
					}
					return nil
				},
			},
			// An empty set removes the permissions.
			{
				Config: testAccUserConfig(username+"-renamed", `
  email                     = "renamed@example.com"
  last_name                 = "Track"
  is_staff                  = true
  configuration_permissions = []
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("configuration_permissions"), knownvalue.SetSizeExact(0)),
				},
			},
			// Changed outside Terraform: the change is reverted
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).Users.Patch(context.Background(), id, &defectdojo.UserPatch{
						Email: new("changed@example.com"), IsActive: new(false),
					}, nil)
					testAccNoError(t, err)
				},
				Config: testAccUserConfig(username+"-renamed", `
  email                     = "renamed@example.com"
  last_name                 = "Track"
  is_staff                  = true
  configuration_permissions = []
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(userAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("email"), knownvalue.StringExact("renamed@example.com")),
					statecheck.ExpectKnownValue(userAddress, tfjsonpath.New("is_active"), knownvalue.Bool(true)),
				},
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).Users.Delete(context.Background(), id, nil)
					testAccNoError(t, err)
				},
				Config: testAccUserConfig(username+"-renamed", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(userAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureUserID(&id),
			},
		},
	})
}

// A user that a product names as its manager can't be deleted; the error says
// what to do.
func TestAccUser_referenced(t *testing.T) {
	testAccSkipUnlessAcc(t)
	username := randName(t)
	var id, productID int
	productTypeID := testAccCreateProductType(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		CheckDestroy: testAccCheckUserDestroyed(t, &id),
		Steps: []resource.TestStep{
			// A product names the user as its manager, outside Terraform.
			{
				Config: testAccUserConfig(username, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureUserID(&id),
					func(*terraform.State) error {
						p, _, err := testAccClient(t).Products.Create(context.Background(), &defectdojo.Product{
							Name: randName(t), Description: "acceptance test", ProdType: productTypeID, ProductManager: id,
						}, nil)
						if err != nil {
							return err
						}
						productID = p.ID
						return nil
					},
				),
			},
			{
				Config:      testAccUserConfig(username, ""),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`(?s)can't\s+be\s+deleted\s+while\s+other\s+objects\s+refer\s+to\s+it.*is_active\s+=\s+false`),
			},
			// Without the reference, the destroy after the test succeeds.
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).Products.Patch(context.Background(), productID, &defectdojo.ProductPatch{
						ProductManager: defectdojo.Null[int](),
					}, nil)
					testAccNoError(t, err)
				},
				Config: testAccUserConfig(username, ""),
			},
		},
	})
}

func TestAccUser_validation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config:      testAccUserConfig("has space", ""),
				ExpectError: regexp.MustCompile(`must\s+contain\s+only\s+letters,\s+digits`),
			},
			{
				Config:      testAccUserConfig(randName(t), `  configuration_permissions = ["Add_User"]`+"\n"),
				ExpectError: regexp.MustCompile(`must\s+be\s+a\s+permission\s+codename`),
			},
			// An unknown codename is refused before the user is created.
			{
				Config:      testAccUserConfig(randName(t), `  configuration_permissions = ["add_nothing"]`+"\n"),
				ExpectError: regexp.MustCompile(`(?s)Unknown DefectDojo configuration permission.*"add_nothing"`),
			},
		},
	})
}

// testAccUserConfig returns the config of a user with a password. The email
// is username@example.com unless extra sets one.
func testAccUserConfig(username, extra string) string {
	email := ""
	if !strings.Contains(extra, "email") {
		email = fmt.Sprintf("  email       = \"%s@example.com\"\n", username)
	}
	return fmt.Sprintf(`
resource "defectdojo_user" "test" {
  username    = %[1]q
  password_wo = %[2]q
%[3]s%[4]s}
`, username, testAccPassword, email, extra)
}

// testAccCaptureUserID stores the ID of the user in id.
func testAccCaptureUserID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[userAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", userAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckUserDestroyed checks that the user is gone.
func testAccCheckUserDestroyed(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := testAccClient(t).Users.Get(context.Background(), *id, nil)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("user %d still exists", *id)
	}
}
