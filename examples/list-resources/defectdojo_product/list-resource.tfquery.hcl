# Find the products, and generate import blocks and configuration with:
#   terraform query -generate-config-out=generated.tf
# Terraform returns at most 100 results unless limit is set. The generated
# configuration sets authorized_users, regulation_ids, tags and
# sla_configuration_id: remove those another system manages.
list "defectdojo_product" "all" {
  provider = defectdojo
  limit    = 1000
}

# Only the products of one product type whose name contains "shop", ignoring case
list "defectdojo_product" "shop" {
  provider = defectdojo

  config {
    product_type_id = 1
    name_contains   = "shop"
  }
}
