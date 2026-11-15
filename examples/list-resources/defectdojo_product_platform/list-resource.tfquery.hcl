# Find the product platforms, including those DefectDojo comes with, and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_product_platform" "all" {
  provider = defectdojo
}

# Only those whose name contains "mobile", ignoring case
list "defectdojo_product_platform" "filtered" {
  provider = defectdojo

  config {
    name_contains = "mobile"
  }
}
