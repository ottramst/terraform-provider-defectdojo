---
page_title: "Connect Dependency-Track to DefectDojo"
subcategory: ""
description: |-
  Upload Dependency-Track's findings to a DefectDojo engagement, configured with this provider and the Dependency-Track provider.
---

# Connect Dependency-Track to DefectDojo

[Dependency-Track](https://dependencytrack.org) can upload the findings of its projects to DefectDojo: every upload imports a
*Dependency Track Finding Packaging Format* scan into an engagement. This guide sets both sides up in Terraform, with this provider
and the [Dependency-Track provider](https://registry.terraform.io/providers/ottramst/dependencytrack/latest):

- **DefectDojo:**
  - a service account for Dependency-Track (`defectdojo_user`), without superuser rights;
  - a product that the account may import into (its `authorized_users`);
  - a CI/CD engagement that receives the findings.
- **Dependency-Track:**
  - the DefectDojo integration (`dependencytrack_config_property`: enabled, URL and API key, the latter through a `dependencytrack_secret` on v5);
  - a project whose properties name the engagement and turn on reimports (`dependencytrack_project_property`).

## The account's API key

Dependency-Track signs in to DefectDojo with an API key. DefectDojo has no API to create a token for another user, so get the
account's key once it exists, in one of two ways:

- **In DefectDojo's UI:** sign in as the account and open *API v2 Key*.
- **With the password:** ask DefectDojo for the token, using the password of `password_wo`:

  ```shell
  curl -s -X POST "$DEFECTDOJO_URL/api/v2/api-token-auth/" \
    -H 'Content-Type: application/json' \
    -d '{"username": "dependency-track", "password": "…"}'
  ```

Pass the key to Terraform as `dependency_track_api_key`, for example through `TF_VAR_dependency_track_api_key`. A typical
workflow applies twice: first without the Dependency-Track resources, or with a placeholder key, then again with the key.

-> **Dependency-Track v4 and v5 differ.** On v5, the example's version, `defectdojo.apiKey` holds the *name* of a secret
(`dependencytrack_secret`) that holds the key. On v4, it holds the key itself: set its value to the key and leave the secret
out.

~> The Dependency-Track provider stores the API key in the Terraform state. Protect the state, and rotate the key in DefectDojo
(*API v2 Key*, or `POST /api/v2/users/{id}/reset_api_token/` as a superuser or global owner) if the state leaks.
API tokens don't expire unless DefectDojo's `DD_API_TOKEN_DEFAULT_EXPIRY_DAYS` is set, or a superuser gives the account's
token its own expiry; if one applies, renew the key before it expires.

## Example

```terraform
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
```

## How it works

- **Uploads:** Dependency-Track uploads the findings of every project with a `defectdojo.engagementId` on a schedule:
  - on v5, the server setting `dt.task.defect-dojo-upload.cron`, by default daily at 02:00;
  - on v4, the `defectdojo.sync.cadence` property, by default every 60 minutes.
- **Reimports:**
  - With `defectdojo.reimport`, an upload reimports into the engagement's test, which updates and closes findings.
  - Without it, each upload adds a test.
  - The global `defectdojo.reimport.enabled` property turns reimports on for every project.
- **The engagement's end:**
  - DefectDojo moves the end of a CI/CD engagement to the scan date of each import, if that is later.
  - The far-future `target_end` keeps the engagement stable in Terraform. Otherwise, use `lifecycle { ignore_changes = [target_end] }`.
- **The account's rights:**
  - `authorized_users` lets it view, edit, add and import within the product, its engagements, tests and findings, but not
    delete them. That is the narrowest grant DefectDojo's open-source edition offers, and anyone with the key has it.
  - It needs no configuration permissions, staff or superuser rights.

## Removing the integration

- **Dependency-Track:** destroying `dependencytrack_config_property` only removes it from the Terraform state, so the
  integration stays on. Set `defectdojo.enabled` to `"false"` and apply before removing the resources.
- **The engagement:** once Dependency-Track has uploaded, the engagement has tests, and `defectdojo_engagement` refuses to
  destroy it unless `force_destroy = true`, which deletes the tests and findings with it.
- **The account:** its findings name it as their reporter, so DefectDojo refuses to delete it while they exist. Deactivate it
  with `is_active = false` and remove it from the state with a `removed` block instead.
