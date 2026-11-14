resource "defectdojo_sla_configuration" "strict" {
  name        = "Internet-facing"
  description = "Shorter SLAs for internet-facing products"
  critical    = 3
  high        = 14
  medium      = 60
  low         = 90
  enforce_low = false
}

resource "defectdojo_product" "storefront" {
  name                 = "Storefront"
  description          = "The online shop"
  product_type_id      = 1
  sla_configuration_id = defectdojo_sla_configuration.strict.id
}

# DefectDojo's Default SLA configuration, which products get unless they
# name another, can be imported and managed, but not deleted. Changing its
# days makes DefectDojo recalculate the SLA dates of every product that uses
# it, and locks them until that has finished.
import {
  to = defectdojo_sla_configuration.default
  id = "1"
}

resource "defectdojo_sla_configuration" "default" {
  name        = "Default"
  description = "The Default SLA Configuration. Products not using an explicit SLA Configuration will use this one."
}
