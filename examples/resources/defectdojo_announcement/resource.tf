resource "defectdojo_announcement" "maintenance" {
  message     = "DefectDojo is down for maintenance on Saturday from 08:00 to 10:00 UTC. <a href=\"https://status.example.com\">Status page</a>"
  style       = "warning"
  dismissable = true
}
