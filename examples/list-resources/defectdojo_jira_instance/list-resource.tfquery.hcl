# Find the Jira instances and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
# Add the required password_wo to the generated configuration.
list "defectdojo_jira_instance" "all" {
  provider = defectdojo
}

# Only those whose configuration name contains "acme", ignoring case
list "defectdojo_jira_instance" "filtered" {
  provider = defectdojo

  config {
    configuration_name_contains = "acme"
  }
}
