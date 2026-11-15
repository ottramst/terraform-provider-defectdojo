# Find the notification webhooks and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_notification_webhook" "all" {
  provider = defectdojo
}

# Only those whose name contains "chat", ignoring case
list "defectdojo_notification_webhook" "filtered" {
  provider = defectdojo

  config {
    name_contains = "chat"
  }
}
