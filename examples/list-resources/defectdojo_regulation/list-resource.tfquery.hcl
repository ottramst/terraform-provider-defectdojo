# Find the regulations, including those DefectDojo comes with, and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_regulation" "all" {
  provider = defectdojo
}

# Only those whose name contains "gdpr", ignoring case
list "defectdojo_regulation" "filtered" {
  provider = defectdojo

  config {
    name_contains = "gdpr"
  }
}
