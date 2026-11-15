# Find the engagements of a product, and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
# Imports of scan results can create many engagements: filter by product.
list "defectdojo_engagement" "shop" {
  provider = defectdojo
  limit    = 500

  config {
    product_id = 1
  }
}

# Only engagements whose name contains "pentest", ignoring case
list "defectdojo_engagement" "pentests" {
  provider = defectdojo

  config {
    name_contains = "pentest"
  }
}
