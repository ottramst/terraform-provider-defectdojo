variable "dependency_track_password" {
  type      = string
  sensitive = true
}

# A service account for Dependency-Track, which imports its findings into a
# product's engagement (see defectdojo_product's authorized_users).
resource "defectdojo_user" "dependency_track" {
  username   = "dependency-track"
  email      = "dependency-track@example.com"
  first_name = "Dependency-Track"

  # Only sent when the user is created; never stored in the state.
  password_wo = var.dependency_track_password
}

variable "jdoe_initial_password" {
  type      = string
  sensitive = true
}

# A user who may manage product types and SLA configurations.
resource "defectdojo_user" "security_lead" {
  username    = "jdoe"
  email       = "jane.doe@example.com"
  first_name  = "Jane"
  last_name   = "Doe"
  password_wo = var.jdoe_initial_password

  configuration_permissions = [
    "add_product_type",
    "change_sla_configuration",
  ]
}
