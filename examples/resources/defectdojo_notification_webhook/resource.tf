variable "webhook_token" {
  type      = string
  sensitive = true
}

resource "defectdojo_notification_webhook" "chat" {
  name        = "Chat relay"
  url         = "https://hooks.example.com/defectdojo"
  header_name = "Authorization"

  # Never stored in the state. Increase the version to send a new value.
  header_value_wo         = "Bearer ${var.webhook_token}"
  header_value_wo_version = 1
}
