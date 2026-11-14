# The system-wide settings, which DefectDojo creates: the resource adopts
# them. Events that aren't set get DefectDojo's default, alert.
resource "defectdojo_notifications" "system" {
  sla_breach     = ["mail", "alert"]
  scan_added     = ["webhooks"]
  jira_update    = ["mail"]
  user_mentioned = []
}

# The template that DefectDojo copies into the settings of new users.
resource "defectdojo_notifications" "template" {
  template      = true
  product_added = ["mail", "alert"]
}

# A user's personal settings, which DefectDojo creates with the user.
resource "defectdojo_notifications" "jdoe" {
  user_id          = defectdojo_user.jdoe.id
  user_mentioned   = ["mail", "alert"]
  scan_added_empty = ["alert"]
}

# The user's settings for one product.
resource "defectdojo_notifications" "jdoe_storefront" {
  user_id    = defectdojo_user.jdoe.id
  product_id = defectdojo_product.storefront.id
  sla_breach = ["mail"]
}
