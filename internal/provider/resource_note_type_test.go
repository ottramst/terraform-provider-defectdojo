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

const noteTypeAddress = "defectdojo_note_type.test"

// The note types stay inactive, except in one step: an active mandatory note
// type keeps the UI of the shared stack from closing findings.
func TestAccNoteType(t *testing.T) {
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
		CheckDestroy: testAccCheckNoteTypeDestroyed(t, &id),
		Steps: []resource.TestStep{
			// Create with the defaults, but inactive
			{
				Config: testAccNoteTypeConfig(name, "  is_active = false\n"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("name"), knownvalue.StringExact(name)),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("description"), knownvalue.StringExact("acceptance test")),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("is_single"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("is_active"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("is_mandatory"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("force_destroy"), knownvalue.Bool(false)),
					statecheck.ExpectIdentityValueMatchesState(noteTypeAddress, tfjsonpath.New("id")),
				},
				Check: testAccCaptureNoteTypeID(&id),
			},
			// Update every attribute in place, and activate it for one step
			{
				Config: fmt.Sprintf(`
resource "defectdojo_note_type" "test" {
  name         = %q
  description  = "Notes from a code review."
  is_single    = true
  is_active    = true
  is_mandatory = false
}
`, renamed),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(noteTypeAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("name"), knownvalue.StringExact(renamed)),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("description"), knownvalue.StringExact("Notes from a code review.")),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("is_single"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("is_active"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("is_mandatory"), knownvalue.Bool(false)),
				},
			},
			{
				Config: testAccNoteTypeConfig(renamed, "  is_active = false\n"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("is_single"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("is_active"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("is_mandatory"), knownvalue.Bool(true)),
				},
			},
			// Import by ID
			{
				ResourceName:            noteTypeAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"force_destroy"},
			},
			// Import by identity
			{
				ResourceName:    noteTypeAddress,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
			// Changed outside Terraform: the change is reverted
			{
				PreConfig: func() {
					_, _, err := testAccClient(t).NoteTypes.Patch(context.Background(), id, &defectdojo.NoteTypePatch{
						Description: new("changed"), IsSingle: new(true),
					})
					testAccNoError(t, err)
				},
				Config: testAccNoteTypeConfig(renamed, "  is_active = false\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(noteTypeAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("description"), knownvalue.StringExact("acceptance test")),
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("is_single"), knownvalue.Bool(false)),
				},
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					_, err := testAccClient(t).NoteTypes.Delete(context.Background(), id)
					testAccNoError(t, err)
				},
				Config: testAccNoteTypeConfig(renamed, "  is_active = false\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(noteTypeAddress, plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureNoteTypeID(&id),
			},
		},
	})
}

// A note type with notes is only destroyed with force_destroy, which deletes
// the notes too.
func TestAccNoteType_forceDestroy(t *testing.T) {
	testAccSkipUnlessAcc(t)
	name := randName(t)
	testID := testAccCreateTestOutside(t, nil)
	var id int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNoteTypeDestroyed(t, &id),
		Steps: []resource.TestStep{
			// Two notes of the type, added outside Terraform.
			{
				Config: testAccNoteTypeConfig(name, "  is_active = false\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureNoteTypeID(&id),
					func(*terraform.State) error {
						testAccCreateNote(t, testID, id)
						testAccCreateNote(t, testID, id)
						return nil
					},
				),
			},
			{
				Config:      testAccNoteTypeConfig(name, "  is_active = false\n"),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`(?s)has\s+2\s+notes.*force_destroy\s+=\s+true`),
			},
			{
				Config: testAccNoteTypeConfig(name, "  is_active     = false\n  force_destroy = true\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(noteTypeAddress, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(noteTypeAddress, tfjsonpath.New("force_destroy"), knownvalue.Bool(true)),
				},
			},
			// The destroy after the test deletes the note type and its notes.
		},
	})
}

func testAccNoteTypeConfig(name, extra string) string {
	return fmt.Sprintf(`
resource "defectdojo_note_type" "test" {
  name        = %[1]q
  description = "acceptance test"
%[2]s}
`, name, extra)
}

// testAccCaptureNoteTypeID stores the ID of the note type in id.
func testAccCaptureNoteTypeID(id *int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[noteTypeAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", noteTypeAddress)
		}
		_, err := fmt.Sscan(rs.Primary.ID, id)
		return err
	}
}

// testAccCheckNoteTypeDestroyed checks that the note type is gone.
func testAccCheckNoteTypeDestroyed(t *testing.T, id *int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *id == 0 {
			return nil
		}
		_, _, err := testAccClient(t).NoteTypes.Get(context.Background(), *id)
		if errors.Is(err, defectdojo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("note type %d still exists", *id)
	}
}
