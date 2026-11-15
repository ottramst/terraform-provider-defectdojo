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

const notificationWebhookAddress = "defectdojo_notification_webhook.test"

func TestAccNotificationWebhook(t *testing.T) {
	name := randName(t)
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Resource identity needs Terraform 1.12+, write-only attributes 1.11+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		CheckDestroy: testAccCheckNotificationWebhookDestroyed(t, &id),
		Steps: []resource.TestStep{
			// Create without a header. DefectDojo doesn't test the URL.
			{
				Config: testAccNotificationWebhookConfig(name, "https://hooks.invalid/one", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("url"), knownvalue.StringExact("https://hooks.invalid/one")),
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("header_name"), knownvalue.Null()),
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("status"), knownvalue.StringExact("active")),
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("note"), knownvalue.Null()),
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("first_error"), knownvalue.Null()),
					statecheck.ExpectIdentityValueMatchesState(notificationWebhookAddress, tfjsonpath.New("id")),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureNotificationWebhookID(&id),
					testAccCheckNotificationWebhookHeader(t, &id, "", ""),
				),
			},
			// Update in place, with a header
			{
				Config: testAccNotificationWebhookConfig(name+"-renamed", "http://hooks.invalid/two", `
  header_name             = "Authorization"
  header_value_wo         = "Bearer one"
  header_value_wo_version = 1
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(notificationWebhookAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("name"), knownvalue.StringExact(name+"-renamed")),
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("header_name"), knownvalue.StringExact("Authorization")),
					// The header value is never in the state.
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("header_value_wo"), knownvalue.Null()),
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("header_value_wo_version"), knownvalue.Int64Exact(1)),
				},
				Check: testAccCheckNotificationWebhookHeader(t, &id, "Authorization", "Bearer one"),
			},
			// A new value with the same version isn't sent.
			{
				Config: testAccNotificationWebhookConfig(name+"-renamed", "http://hooks.invalid/two", `
  header_name             = "Authorization"
  header_value_wo         = "Bearer ignored"
  header_value_wo_version = 1
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: testAccCheckNotificationWebhookHeader(t, &id, "Authorization", "Bearer one"),
			},
			// A new version sends the value.
			{
				Config: testAccNotificationWebhookConfig(name+"-renamed", "http://hooks.invalid/two", `
  header_name             = "Authorization"
  header_value_wo         = "Bearer two"
  header_value_wo_version = 2
`),
				Check: testAccCheckNotificationWebhookHeader(t, &id, "Authorization", "Bearer two"),
			},
			// Import by ID
			{
				ResourceName:            notificationWebhookAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"header_value_wo_version"},
			},
			// Import by identity. An imported webhook has no version in the
			// state, and setting one sends the value, so the import's config
			// leaves the value out.
			{
				Config: testAccNotificationWebhookConfig(name+"-renamed", "http://hooks.invalid/two", `
  header_name = "Authorization"
`),
				ResourceName:    notificationWebhookAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// The configuration -generate-config-out writes for an import plans no
			// changes. terraform query writes it from the list resource's state,
			// which TestListResourceStates checks is the same.
			{
				Config: testAccNotificationWebhookConfig(name+"-renamed", "http://hooks.invalid/two", `
  header_name = "Authorization"
`),
				ResourceName:    notificationWebhookAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				GenerateConfig:  true,
			},
			// Changed outside Terraform: the change is reverted, but not the
			// header value, which Terraform can't compare.
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).NotificationWebhooks.Patch(context.Background(), id, &defectdojo.NotificationWebhookPatch{
						URL: new("https://hooks.invalid/changed"), HeaderValue: defectdojo.Value("Bearer changed"),
					}, nil)
					testAccNoError(t, err)
				},
				Config: testAccNotificationWebhookConfig(name+"-renamed", "http://hooks.invalid/two", `
  header_name             = "Authorization"
  header_value_wo         = "Bearer two"
  header_value_wo_version = 2
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(notificationWebhookAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("url"), knownvalue.StringExact("http://hooks.invalid/two")),
				},
				Check: testAccCheckNotificationWebhookHeader(t, &id, "Authorization", "Bearer changed"),
			},
			// Removing the header and its value clears both.
			{
				Config: testAccNotificationWebhookConfig(name+"-renamed", "http://hooks.invalid/two", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("header_name"), knownvalue.Null()),
					statecheck.ExpectKnownValue(notificationWebhookAddress, tfjsonpath.New("header_value_wo_version"), knownvalue.Null()),
				},
				Check: testAccCheckNotificationWebhookHeader(t, &id, "", ""),
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).NotificationWebhooks.Delete(context.Background(), id, nil)
					testAccNoError(t, err)
				},
				Config: testAccNotificationWebhookConfig(name+"-renamed", "http://hooks.invalid/two", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(notificationWebhookAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureNotificationWebhookID(&id),
			},
		},
	})
}

// Setting the version for the first time, after an import, sends the value:
// unlike a replace, the change is a PATCH.
func TestAccNotificationWebhook_importThenSetValue(t *testing.T) {
	testAccSkipUnlessAcc(t)
	name := randName(t)
	wh, _, err := testAccClient(t).NotificationWebhooks.Create(context.Background(), &defectdojo.NotificationWebhook{
		Name: name, URL: "https://hooks.invalid/import", HeaderName: "Authorization", HeaderValue: "Bearer old",
	}, nil)
	testAccNoError(t, err)
	id := wh.ID
	t.Cleanup(func() {
		if _, err := testAccClient(t).NotificationWebhooks.Delete(context.Background(), id, nil); err != nil && !errors.Is(err, defectdojo.ErrNotFound) {
			t.Logf("cleanup: %v", err)
		}
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		CheckDestroy: testAccCheckNotificationWebhookDestroyed(t, &id),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
import {
  to = defectdojo_notification_webhook.test
  id = "%d"
}
`, id) + testAccNotificationWebhookConfig(name, "https://hooks.invalid/import", `
  header_name             = "Authorization"
  header_value_wo         = "Bearer new"
  header_value_wo_version = 1
`),
				Check: testAccCheckNotificationWebhookHeader(t, &id, "Authorization", "Bearer new"),
			},
		},
	})
}

func TestAccNotificationWebhook_validation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config:      testAccNotificationWebhookConfig("invalid", "ftp://hooks.invalid/x", ""),
				ExpectError: regexp.MustCompile(`must\s+be\s+an\s+http://\s+or\s+https://\s+URL`),
			},
			// A header value needs a version, so that it can be changed.
			{
				Config:      testAccNotificationWebhookConfig("invalid", "https://hooks.invalid/x", `  header_value_wo = "x"`+"\n"),
				ExpectError: regexp.MustCompile(`header_value_wo_version`),
			},
			{
				Config:      testAccNotificationWebhookConfig("invalid", "https://hooks.invalid/x", "  header_value_wo_version = 1\n"),
				ExpectError: regexp.MustCompile(`header_value_wo`),
			},
			// A header value without a header name would never be sent.
			{
				Config: testAccNotificationWebhookConfig("invalid", "https://hooks.invalid/x", `
  header_value_wo         = "x"
  header_value_wo_version = 1
`),
				ExpectError: regexp.MustCompile(`header_name`),
			},
		},
	})
}

func testAccNotificationWebhookConfig(name, url, extra string) string {
	return fmt.Sprintf(`
resource "defectdojo_notification_webhook" "test" {
  name = %[1]q
  url  = %[2]q
%[3]s}
`, name, url, extra)
}

// testAccCaptureNotificationWebhookID stores the ID of the webhook in id.
func testAccCaptureNotificationWebhookID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[notificationWebhookAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", notificationWebhookAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckNotificationWebhookHeader checks the webhook's header in
// DefectDojo, which is the only place its value is.
func testAccCheckNotificationWebhookHeader(t *testing.T, id *int, name, value string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		wh, _, err := testAccClient(t).NotificationWebhooks.Get(context.Background(), *id, nil)
		if err != nil {
			return err
		}
		if wh.HeaderName != name || wh.HeaderValue != value {
			return fmt.Errorf("header = %q: %q, want %q: %q", wh.HeaderName, wh.HeaderValue, name, value)
		}
		return nil
	}
}

// testAccCheckNotificationWebhookDestroyed checks that the webhook is gone.
func testAccCheckNotificationWebhookDestroyed(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := testAccClient(t).NotificationWebhooks.Get(context.Background(), *id, nil)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("notification webhook %d still exists", *id)
	}
}
