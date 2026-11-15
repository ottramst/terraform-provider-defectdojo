resource "defectdojo_product_platform" "kiosk" {
  value         = "kiosk"
  name          = "Kiosk"
  icon          = "tv"
  display_order = 60

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
  platform        = defectdojo_product_platform.kiosk.value
}
