resource "defectdojo_product_type" "web" {
  name = "Web Applications"
}

resource "defectdojo_product" "storefront" {
  name            = "Storefront"
  description     = "The online shop"
  product_type_id = defectdojo_product_type.web.id

  business_criticality = "high"
  platform             = "web"
  life_cycle           = "production"
  origin               = "internal"
  internet_accessible  = true

  product_manager_id = 12
  regulation_ids     = [1] # PCI DSS
  tags               = ["pci", "shop"]
}

# Without authorized_users, regulation_ids and tags, the provider leaves
# them as they are, so they can be managed elsewhere.
resource "defectdojo_product" "inventory" {
  name             = "Inventory"
  description      = "Stock management"
  product_type_id  = defectdojo_product_type.web.id
  authorized_users = [12, 15]
}
