resource "defectdojo_jira_instance" "example" {
  configuration_name = "Acme Jira"
  url                = "https://acme.atlassian.net"
  username           = "defectdojo@acme.example"

  # A Jira API token; change the version to send a new one.
  password_wo         = var.jira_api_token
  password_wo_version = 1

  # From https://acme.atlassian.net/rest/api/2/field and the transitions of an issue
  epic_name_id     = 10011
  open_status_key  = 11
  close_status_key = 31

  info_mapping_severity     = "Lowest"
  low_mapping_severity      = "Low"
  medium_mapping_severity   = "Medium"
  high_mapping_severity     = "High"
  critical_mapping_severity = "Highest"

  default_issue_type          = "Security"
  accepted_mapping_resolution = "Won't Fix"
}

variable "jira_api_token" {
  type      = string
  sensitive = true
  ephemeral = true
}
