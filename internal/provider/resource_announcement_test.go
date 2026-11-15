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

const announcementAddress = "defectdojo_announcement.test"

// DefectDojo keeps one announcement, shared by the whole stack, so one test
// covers the resource and leaves none behind.
func TestAccAnnouncement(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			if a, _, err := testAccClient(t).Announcements.Current(context.Background()); !errors.Is(err, defectdojo.ErrNotFound) {
				t.Fatalf("the stack has an announcement before the test: %+v, %v", a, err)
			}
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// Resource identity needs Terraform 1.12+.
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_12_0),
		},
		CheckDestroy: testAccCheckNoAnnouncement(t),
		Steps: []resource.TestStep{
			// Create with the defaults
			{
				Config: testAccAnnouncementConfig("Maintenance on <b>Saturday</b>.", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(announcementAddress, tfjsonpath.New("id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(announcementAddress, tfjsonpath.New("message"), knownvalue.StringExact("Maintenance on <b>Saturday</b>.")),
					statecheck.ExpectKnownValue(announcementAddress, tfjsonpath.New("style"), knownvalue.StringExact("info")),
					statecheck.ExpectKnownValue(announcementAddress, tfjsonpath.New("dismissable"), knownvalue.Bool(false)),
					statecheck.ExpectIdentityValueMatchesState(announcementAddress, tfjsonpath.New("id")),
				},
			},
			// dismissable changes in place
			{
				Config: testAccAnnouncementConfig("Maintenance on <b>Saturday</b>.", "  dismissable = true\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(announcementAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(announcementAddress, tfjsonpath.New("dismissable"), knownvalue.Bool(true)),
				},
			},
			// The message and the style replace it, so everyone sees it again
			{
				Config: testAccAnnouncementConfig("Maintenance moved to Sunday.", "  style       = \"warning\"\n  dismissable = true\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(announcementAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(announcementAddress, tfjsonpath.New("id"), knownvalue.Int64Exact(1)),
					statecheck.ExpectKnownValue(announcementAddress, tfjsonpath.New("message"), knownvalue.StringExact("Maintenance moved to Sunday.")),
					statecheck.ExpectKnownValue(announcementAddress, tfjsonpath.New("style"), knownvalue.StringExact("warning")),
				},
			},
			// The style alone replaces it too
			{
				Config: testAccAnnouncementConfig("Maintenance moved to Sunday.", "  style       = \"danger\"\n  dismissable = true\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(announcementAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(announcementAddress, tfjsonpath.New("style"), knownvalue.StringExact("danger")),
				},
			},
			{
				Config: testAccAnnouncementConfig("Maintenance moved to Sunday.", "  style       = \"warning\"\n  dismissable = true\n"),
			},
			// Import by ID
			{
				ResourceName:      announcementAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Import by identity
			{
				ResourceName:    announcementAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// Changed outside Terraform: Terraform replaces it
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).Announcements.Patch(context.Background(), 1, &defectdojo.AnnouncementPatch{Message: new("changed")})
					testAccNoError(t, err)
				},
				Config: testAccAnnouncementConfig("Maintenance moved to Sunday.", "  style       = \"warning\"\n  dismissable = true\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(announcementAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(announcementAddress, tfjsonpath.New("message"), knownvalue.StringExact("Maintenance moved to Sunday.")),
				},
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).Announcements.Delete(context.Background(), 1)
					testAccNoError(t, err)
				},
				Config: testAccAnnouncementConfig("Maintenance moved to Sunday.", "  style       = \"warning\"\n  dismissable = true\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(announcementAddress, plancheck.ResourceActionCreate),
					},
				},
			},
			// An announcement created outside Terraform blocks a second
			// resource, which suggests the import.
			{
				Config: testAccAnnouncementConfig("Maintenance moved to Sunday.", "  style       = \"warning\"\n  dismissable = true\n") + `
resource "defectdojo_announcement" "second" {
  message    = "second"
  depends_on = [defectdojo_announcement.test]
}
`,
				ExpectError: regexp.MustCompile(`(?s)already\s+has\s+an\s+announcement.*Import\s+it`),
			},
			// A valid config last, for the destroy after the test.
			{
				Config: testAccAnnouncementConfig("Maintenance moved to Sunday.", "  style       = \"warning\"\n  dismissable = true\n"),
			},
		},
	})
}

func testAccAnnouncementConfig(message, extra string) string {
	return fmt.Sprintf(`
resource "defectdojo_announcement" "test" {
  message = %q
%s}
`, message, extra)
}

// testAccCheckNoAnnouncement checks that the stack has no announcement.
func testAccCheckNoAnnouncement(t *testing.T) resource.TestCheckFunc {
	return func(*terraform.State) error {
		a, _, err := testAccClient(t).Announcements.Current(context.Background())
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("announcement %d still exists", a.ID)
	}
}
