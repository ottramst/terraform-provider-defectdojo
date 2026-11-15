resource "defectdojo_product_lifecycle" "maintenance" {
  value         = "maintenance"
  name          = "Maintenance only"
  icon          = "wrench"
  display_order = 25

  # DefectDojo can't change the value, and refuses to delete an entry that
  # products use: create the new entry before deleting the old one.
  lifecycle {
    create_before_destroy = true
  }
}

resource "defectdojo_product" "example" {
  name            = "Example"
  description     = "An example product"
  product_type_id = 4
  life_cycle      = defectdojo_product_lifecycle.maintenance.value
}
