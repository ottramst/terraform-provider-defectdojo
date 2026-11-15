# Find the note types and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_note_type" "all" {
  provider = defectdojo
}

# Only those whose name contains "sign-off", ignoring case
list "defectdojo_note_type" "filtered" {
  provider = defectdojo

  config {
    name_contains = "sign-off"
  }
}
