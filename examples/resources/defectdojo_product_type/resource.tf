resource "defectdojo_product_type" "web" {
  name             = "Web Applications"
  description      = "Customer-facing web applications"
  critical_product = true
}

# Give two users access to the product type and its products. Without
# authorized_users, the provider leaves the authorized users as they are.
resource "defectdojo_product_type" "internal" {
  name             = "Internal Tools"
  authorized_users = [12, 15]
}
