# Find the product lifecycles, including those DefectDojo comes with, and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_product_lifecycle" "all" {
  provider = defectdojo
}

# Only those whose name contains "production", ignoring case
list "defectdojo_product_lifecycle" "filtered" {
  provider = defectdojo

  config {
    name_contains = "production"
  }
}
