resource "defectdojo_product_type" "web" {
  name = "Web Applications"
}

data "defectdojo_user" "manager" {
  username = "jdoe"
}

resource "defectdojo_sla_configuration" "strict" {
  name     = "Internet-facing"
  critical = 3
  high     = 14
}

resource "defectdojo_product" "storefront" {
  name                 = "Storefront"
  description          = "The online shop"
  product_type_id      = defectdojo_product_type.web.id
  sla_configuration_id = defectdojo_sla_configuration.strict.id

  business_criticality = "high"
  platform             = "web"
  life_cycle           = "production"
  origin               = "internal"
  internet_accessible  = true

  product_manager_id = data.defectdojo_user.manager.id
  regulation_ids     = [1] # PCI DSS
  tags               = ["pci", "shop"]
}

# Without regulation_ids and tags, the provider leaves them as they are, so
# they can be managed elsewhere; authorized_users are managed here.
resource "defectdojo_product" "inventory" {
  name             = "Inventory"
  description      = "Stock management"
  product_type_id  = defectdojo_product_type.web.id
  authorized_users = [12, 15]
}
