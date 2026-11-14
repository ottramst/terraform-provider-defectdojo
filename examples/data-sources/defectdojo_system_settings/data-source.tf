data "defectdojo_system_settings" "current" {}

output "jira_enabled" {
  value = data.defectdojo_system_settings.current.enable_jira
}
