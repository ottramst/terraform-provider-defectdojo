# Find the product origins, including those DefectDojo comes with, and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_product_origin" "all" {
  provider = defectdojo
}

# Only those whose name contains "open source", ignoring case
list "defectdojo_product_origin" "filtered" {
  provider = defectdojo

  config {
    name_contains = "open source"
  }
}
