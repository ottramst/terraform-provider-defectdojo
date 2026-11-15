# Find the product types, and generate import blocks and configuration with:
#   terraform query -generate-config-out=generated.tf
# Terraform returns at most 100 results unless limit is set.
list "defectdojo_product_type" "all" {
  provider = defectdojo
  limit    = 1000
}

# Only product types whose name contains "web", ignoring case
list "defectdojo_product_type" "web" {
  provider = defectdojo

  config {
    name_contains = "web"
  }
}
