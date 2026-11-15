resource "defectdojo_product_type" "web" {
  name        = "Web applications"
  description = "Customer-facing web applications"
}

resource "defectdojo_product" "shop" {
  name                        = "Shop"
  description                 = "The web shop"
  product_type_id             = defectdojo_product_type.web.id
  business_criticality        = "high"
  platform                    = "web"
  life_cycle                  = "production"
  enable_skip_risk_acceptance = true
  user_records                = 25000
  revenue                     = "1000000.00"
  tags                        = ["pci"]
}

resource "defectdojo_jira_product_configuration" "shop" {
  jira_instance_id = "1"
  product_id       = defectdojo_product.shop.id
  project_key      = "SHOP"
}
