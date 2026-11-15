# Find the SLA configurations and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_sla_configuration" "all" {
  provider = defectdojo
}

# Only those whose name contains "strict", ignoring case
list "defectdojo_sla_configuration" "filtered" {
  provider = defectdojo

  config {
    name_contains = "strict"
  }
}
