# Find the users, and generate import blocks and configuration with:
#   terraform query -generate-config-out=generated.tf
# Requires a superuser. Terraform returns at most 100 results unless limit is set.
list "defectdojo_user" "all" {
  provider = defectdojo
  limit    = 1000
}

# Only users whose username contains "svc-", ignoring case
list "defectdojo_user" "service_accounts" {
  provider = defectdojo

  config {
    username_contains = "svc-"
  }
}
