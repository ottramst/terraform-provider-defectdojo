resource "defectdojo_product" "storefront" {
  name            = "Storefront"
  description     = "The online shop"
  product_type_id = 4
}

# A long-lived engagement that a CI/CD pipeline, or Dependency-Track's
# DefectDojo integration, imports scans into. Imports move target_end to the
# scan date, so it is set far in the future, which also keeps DefectDojo from
# closing the engagement automatically.
resource "defectdojo_engagement" "ci" {
  product_id                  = defectdojo_product.storefront.id
  name                        = "CI/CD"
  engagement_type             = "CI/CD"
  target_start                = "2026-01-01"
  target_end                  = "2099-12-31"
  branch_tag                  = "main"
  source_code_management_uri  = "https://git.example.com/shop/storefront"
  deduplication_on_engagement = true
  tags                        = ["ci"]

  lifecycle {
    # Pipelines and DefectDojo close and reopen engagements.
    ignore_changes = [status]
  }
}

# An interactive engagement, such as a penetration test.
resource "defectdojo_engagement" "pentest" {
  product_id   = defectdojo_product.storefront.id
  name         = "Penetration test 2026"
  target_start = "2026-03-02"
  target_end   = "2026-03-13"
  status       = "Scheduled"
  lead_id      = 12
  description  = "Yearly external penetration test"
}
