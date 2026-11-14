package provider

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

// The tests change shared settings (the system-wide ones and the template),
// so they must not run in parallel with tests that create users; no test in
// this package runs in parallel.

const notificationsAddress = "defectdojo_notifications.test"

// A user's personal settings, which DefectDojo creates with the user, are
// adopted, and reset to the defaults on destroy.
func TestAccNotifications_personal(t *testing.T) {
	username := randName(t)
	var id int
	user := testAccUserConfig(username, "")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Resource identity needs Terraform 1.12+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		Steps: []resource.TestStep{
			{
				Config: user + `
resource "defectdojo_notifications" "test" {
  user_id          = defectdojo_user.test.id
  product_added    = ["mail", "alert"]
  scan_added_empty = ["mail"]
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("product_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("template"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("product_added"), knownvalue.SetExact([]knownvalue.Check{
						knownvalue.StringExact("mail"), knownvalue.StringExact("alert"),
					})),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("other"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("alert")})),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("scan_added_empty"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("mail")})),
					statecheck.ExpectIdentityValueMatchesState(notificationsAddress, tfjsonpath.New("id")),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureNotificationsID(&id),
					// The settings DefectDojo created for the user were adopted.
					func(s *terraform.State) error {
						n, _, err := testAccClient(t).Notifications.GetByScope(context.Background(), testAccStateInt(s, userAddress, "id"), 0, false)
						if err != nil {
							return err
						}
						if n.ID != id {
							return fmt.Errorf("the user's settings are %d, the resource's %d", n.ID, id)
						}
						return nil
					},
				),
			},
			// Update in place; events that aren't configured get the default.
			{
				Config: user + `
resource "defectdojo_notifications" "test" {
  user_id    = defectdojo_user.test.id
  sla_breach = []
  other      = ["mail"]
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(notificationsAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("product_added"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("alert")})),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("sla_breach"), knownvalue.SetSizeExact(0)),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("scan_added_empty"), knownvalue.SetSizeExact(0)),
				},
			},
			// Import by ID
			{
				ResourceName:      notificationsAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				ResourceName:    notificationsAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// Changed outside Terraform: the change is reverted
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).Notifications.Patch(context.Background(), id, &defectdojo.NotificationSettingsPatch{
						Other: new([]string{"slack"}), ScanAddedEmpty: new("alert"),
					})
					testAccNoError(t, err)
				},
				Config: user + `
resource "defectdojo_notifications" "test" {
  user_id    = defectdojo_user.test.id
  sla_breach = []
  other      = ["mail"]
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(notificationsAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("other"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("mail")})),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("scan_added_empty"), knownvalue.SetSizeExact(0)),
				},
			},
			// Destroyed while the user stays: the settings are reset, not
			// deleted, so the user is still notified.
			{
				Config: user,
				Check:  testAccCheckNotificationsDefaults(t, &id),
			},
		},
	})
}

// Settings for a product are created and deleted.
func TestAccNotifications_product(t *testing.T) {
	testAccSkipUnlessAcc(t)
	productTypeID := testAccCreateProductType(t)
	product := fmt.Sprintf(`
resource "defectdojo_product" "test" {
  name            = %q
  description     = "notifications"
  product_type_id = %d
}
`, randName(t), productTypeID)
	config := product + `
resource "defectdojo_notifications" "test" {
  user_id          = 1
  product_id       = defectdojo_product.test.id
  engagement_added = ["mail"]
}
`
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("user_id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("engagement_added"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("mail")})),
				},
				Check: testAccCaptureNotificationsID(&id),
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).Notifications.Delete(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(notificationsAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureNotificationsID(&id),
			},
			// Destroyed: deleted.
			{
				Config: product,
				Check: func(*terraform.State) error {
					_, _, err := testAccClient(t).Notifications.Get(context.Background(), id)
					if !errors.Is(err, defectdojo.ErrNotFound) {
						return fmt.Errorf("Get(%d) error = %v, want the settings deleted", id, err)
					}
					return nil
				},
			},
		},
	})
}

// The system-wide settings are adopted and reset on destroy.
func TestAccNotifications_system(t *testing.T) {
	testAccSkipUnlessAcc(t)
	client := testAccClient(t)
	system, _, err := client.Notifications.GetByScope(context.Background(), 0, 0, false)
	testAccNoError(t, err)
	t.Cleanup(func() {
		if _, _, err := client.Notifications.Update(context.Background(), system.ID, system); err != nil {
			t.Logf("cleanup: restoring the system-wide notification settings: %v", err)
		}
	})
	id := system.ID

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationsDefaults(t, &id),
		Steps: []resource.TestStep{
			{
				Config: `
resource "defectdojo_notifications" "test" {
  sla_breach          = ["mail", "alert"]
  sla_breach_combined = []
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("id"), knownvalue.Int64Exact(int64(system.ID))),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("user_id"), knownvalue.Null()),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("sla_breach_combined"), knownvalue.SetSizeExact(0)),
				},
			},
		},
	})
}

// The template is created, copied into new users' settings, and deleted.
func TestAccNotifications_template(t *testing.T) {
	testAccSkipUnlessAcc(t)
	client := testAccClient(t)
	if existing, _, err := client.Notifications.GetByScope(context.Background(), 0, 0, true); err == nil {
		t.Fatalf("a notification settings template exists already (ID %d); this test needs a server without one", existing.ID)
	}
	t.Cleanup(func() {
		if n, _, err := client.Notifications.GetByScope(context.Background(), 0, 0, true); err == nil {
			if _, err := client.Notifications.Delete(context.Background(), n.ID); err != nil {
				t.Logf("cleanup: deleting the notification settings template: %v", err)
			}
		}
	})
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		CheckDestroy: func(*terraform.State) error {
			if _, _, err := client.Notifications.GetByScope(context.Background(), 0, 0, true); !errors.Is(err, defectdojo.ErrNotFound) {
				return fmt.Errorf("GetByScope(template) error = %v, want the template deleted", err)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: `
resource "defectdojo_notifications" "test" {
  template      = true
  product_added = ["mail"]
}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("template"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("user_id"), knownvalue.Null()),
				},
				Check: testAccCaptureNotificationsID(&id),
			},
			// Updated in place, with a PATCH: a PUT of the template fails.
			{
				Config: `
resource "defectdojo_notifications" "test" {
  template      = true
  product_added = ["slack"]
}
` + testAccUserConfig(randName(t), ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationsAddress, tfjsonpath.New("product_added"), knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("slack")})),
				},
				// A user created with the template in place gets its events.
				Check: func(s *terraform.State) error {
					n, _, err := client.Notifications.GetByScope(context.Background(), testAccStateInt(s, userAddress, "id"), 0, false)
					if err != nil {
						return err
					}
					if !reflect.DeepEqual(n.ProductAdded, []string{"slack"}) {
						return fmt.Errorf("the new user's product_added = %v, want the template's [slack]", n.ProductAdded)
					}
					return nil
				},
			},
		},
	})
}

func TestAccNotifications_validation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "defectdojo_notifications" "test" {
  template = true
  user_id  = 1
}
`,
				ExpectError: regexp.MustCompile(`The\s+template\s+has\s+no\s+user\s+or\s+product`),
			},
			{
				Config: `
resource "defectdojo_notifications" "test" {
  product_id = 1
}
`,
				ExpectError: regexp.MustCompile(`user_id`),
			},
			{
				Config: `
resource "defectdojo_notifications" "test" {
  other            = ["pager"]
  scan_added_empty = ["mail", "alert"]
}
`,
				ExpectError: regexp.MustCompile(`(?s)value must be one of.*set must contain at most 1`),
			},
			// An explicit template = false is fine. A valid config last,
			// for the destroy after the test.
			{
				Config: `
resource "defectdojo_notifications" "test" {
  template = false
  user_id  = 1
}
`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// testAccCaptureNotificationsID stores the ID of the notification settings in
// id.
func testAccCaptureNotificationsID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		*id = testAccStateInt(s, notificationsAddress, "id")
		return nil
	}
}

// testAccStateInt returns a numeric attribute of a resource in the state, or
// 0.
func testAccStateInt(s *terraform.State, address, attribute string) int {
	var n int
	if rs, ok := s.RootModule().Resources[address]; ok {
		_, _ = fmt.Sscan(rs.Primary.Attributes[attribute], &n)
	}
	return n
}

// testAccCheckNotificationsDefaults checks that the notification settings
// exist with DefectDojo's defaults.
func testAccCheckNotificationsDefaults(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		n, _, err := testAccClient(t).Notifications.Get(context.Background(), *id)
		if err != nil {
			return fmt.Errorf("the settings were reset, not deleted: %w", err)
		}
		alert := []string{"alert"}
		if !reflect.DeepEqual(n.ProductAdded, alert) || !reflect.DeepEqual(n.Other, alert) || !reflect.DeepEqual(n.SLABreach, alert) ||
			len(n.ScanAddedEmpty) != 0 {
			return fmt.Errorf("settings %d = %+v, want DefectDojo's defaults", *id, n)
		}
		return nil
	}
}
