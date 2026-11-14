terraform {
  required_version = ">= 1.11"

  required_providers {
    defectdojo = {
      source  = "ottramst/defectdojo"
      version = "~> 0.2.0"
    }
    dependencytrack = {
      source  = "ottramst/dependencytrack"
      version = "~> 0.7"
    }
  }
}

variable "defectdojo_url" {
  type        = string
  description = "The URL of DefectDojo, as Dependency-Track reaches it."
}

variable "dependency_track_password" {
  type        = string
  sensitive   = true
  description = "The initial password of the DefectDojo account that Dependency-Track uses."
}

variable "dependency_track_api_key" {
  type        = string
  sensitive   = true
  description = "The API key of that account, from DefectDojo (see the guide)."
}

# DefectDojo: the product and the engagement that Dependency-Track imports
# its findings into, and the account it uses. The example is for
# Dependency-Track v5; see the comment on the API key for v4.

resource "defectdojo_user" "dependency_track" {
  username    = "dependency-track"
  email       = "dependency-track@example.com"
  first_name  = "Dependency-Track"
  password_wo = var.dependency_track_password
}

resource "defectdojo_product_type" "web" {
  name = "Web Applications"
}

resource "defectdojo_product" "storefront" {
  name            = "Storefront"
  description     = "The online shop"
  product_type_id = defectdojo_product_type.web.id

  # The account may import scans into the product's engagements.
  authorized_users = [defectdojo_user.dependency_track.id]
}

resource "defectdojo_engagement" "dependency_track" {
  product_id      = defectdojo_product.storefront.id
  name            = "Dependency-Track"
  engagement_type = "CI/CD"
  target_start    = "2026-01-01"
  # Every import into a CI/CD engagement moves its end to the scan date if
  # that is later: a far-future end keeps it stable, and keeps DefectDojo
  # from closing the engagement automatically.
  target_end = "2099-12-31"
}

# Dependency-Track: the integration, and the project that uploads its
# findings to the engagement.

resource "dependencytrack_config_property" "defectdojo_enabled" {
  group_name = "integrations"
  name       = "defectdojo.enabled"
  value      = "true"
}

resource "dependencytrack_config_property" "defectdojo_url" {
  group_name = "integrations"
  name       = "defectdojo.url"
  value      = var.defectdojo_url
}

# Dependency-Track v5 keeps the API key in a secret, and defectdojo.apiKey
# names the secret. (On Dependency-Track v4, defectdojo.apiKey holds the key
# itself: set its value to var.dependency_track_api_key and leave the secret
# out.) The key ends up in the Terraform state: protect the state.
resource "dependencytrack_secret" "defectdojo_api_key" {
  name        = "defectdojo-api-key"
  value       = var.dependency_track_api_key
  description = "API key of the dependency-track account in DefectDojo"
}

resource "dependencytrack_config_property" "defectdojo_api_key" {
  group_name = "integrations"
  name       = "defectdojo.apiKey"
  value      = dependencytrack_secret.defectdojo_api_key.name
}

resource "dependencytrack_project" "storefront" {
  name    = "Storefront"
  version = "main"
}

resource "dependencytrack_project_property" "engagement" {
  project = dependencytrack_project.storefront.id
  group   = "integrations"
  name    = "defectdojo.engagementId"
  value   = tostring(defectdojo_engagement.dependency_track.id)
  type    = "STRING"
}

# Reimport into the same test on each upload, so that DefectDojo updates and
# closes findings instead of adding a test per upload.
resource "dependencytrack_project_property" "reimport" {
  project = dependencytrack_project.storefront.id
  group   = "integrations"
  name    = "defectdojo.reimport"
  value   = "true"
  type    = "BOOLEAN"
}
