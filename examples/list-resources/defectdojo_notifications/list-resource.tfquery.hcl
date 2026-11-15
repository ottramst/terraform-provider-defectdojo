# Find the template for new users, and generate import blocks and
# configuration with:
#   terraform query -generate-config-out=generated.tf
list "defectdojo_notifications" "template" {
  provider = defectdojo

  config {
    template = true
  }
}

# The settings of one user: personal, and per product. DefectDojo creates
# personal settings for users, so an unfiltered list is long.
list "defectdojo_notifications" "user" {
  provider = defectdojo
  limit    = 500

  config {
    user_id = 7
  }
}
