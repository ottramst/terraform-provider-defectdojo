# Find the development environments, including those DefectDojo comes with, and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_development_environment" "all" {
  provider = defectdojo
}

# Only those whose name contains "staging", ignoring case
list "defectdojo_development_environment" "filtered" {
  provider = defectdojo

  config {
    name_contains = "staging"
  }
}
