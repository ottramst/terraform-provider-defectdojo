resource "defectdojo_note_type" "code_review" {
  name         = "Code Review"
  description  = "Notes from a manual code review."
  is_mandatory = false
}

# An active mandatory note type: the UI asks for a sign-off note before a
# finding is closed, or set inactive or false positive.
resource "defectdojo_note_type" "sign_off" {
  name        = "Sign-off"
  description = "Who accepted closing the finding, and why."
  is_single   = true
}
