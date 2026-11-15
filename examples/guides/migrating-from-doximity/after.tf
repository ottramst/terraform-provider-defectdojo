resource "defectdojo_product_type" "web" {
  name        = "Web applications"
  description = "Customer-facing web applications"
}

resource "defectdojo_product" "shop" {
  name                 = "Shop"
  description          = "The web shop"
  product_type_id      = defectdojo_product_type.web.id
  business_criticality = "high"
  platform             = "web"
  life_cycle           = "production"
  # Renamed from enable_skip_risk_acceptance.
  enable_simple_risk_acceptance = true
  user_records                  = 25000
  revenue                       = "1000000.00"
  tags                          = ["pci"]
}

# Was defectdojo_jira_product_configuration.
resource "defectdojo_jira_project" "shop" {
  jira_instance_id = 1
  product_id       = defectdojo_product.shop.id
  project_key      = "SHOP"
}

moved {
  from = defectdojo_jira_product_configuration.shop
  to   = defectdojo_jira_project.shop
}
