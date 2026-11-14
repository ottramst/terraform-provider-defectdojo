data "defectdojo_product" "storefront" {
  name = "Storefront"
}

# Engagement names need not be unique, so a name lookup needs the product.
data "defectdojo_engagement" "ci" {
  product_id = data.defectdojo_product.storefront.id
  name       = "CI/CD"
}

data "defectdojo_engagement" "by_id" {
  id = 15
}
