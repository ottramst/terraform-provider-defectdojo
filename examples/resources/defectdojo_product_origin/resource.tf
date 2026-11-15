resource "defectdojo_product_origin" "acquired" {
  value         = "acquired"
  name          = "Acquired company"
  icon          = "building"
  display_order = 70

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
  origin          = defectdojo_product_origin.acquired.value
}
