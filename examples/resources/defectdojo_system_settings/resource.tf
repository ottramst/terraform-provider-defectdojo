# Only the settings in the configuration are managed; the others keep
# DefectDojo's values.
resource "defectdojo_system_settings" "example" {
  enable_deduplication = true
  delete_duplicates    = true
  max_dupes            = 5

  enable_finding_sla       = true
  enable_notify_sla_active = true

  engagement_auto_close      = true
  engagement_auto_close_days = 30

  # Notifications to Slack, also needed by the slack channel of
  # defectdojo_notifications.
  enable_slack_notifications = true
  slack_channel              = "#security"
  slack_token_wo             = var.slack_token
  slack_token_wo_version     = 1
}

variable "slack_token" {
  type      = string
  sensitive = true
  ephemeral = true
}
