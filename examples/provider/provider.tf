# Authentication with an API v2 token
provider "defectdojo" {
  url       = "https://defectdojo.example.com"
  api_token = var.defectdojo_api_token
}

# Authentication with a username and password (HTTP basic authentication),
# which also works when API tokens are disabled on the server
provider "defectdojo" {
  alias    = "basic"
  url      = "https://defectdojo.example.com"
  username = "terraform"
  password = var.defectdojo_password
}

# Every attribute can also be set with an environment variable:
# DEFECTDOJO_URL, DEFECTDOJO_API_TOKEN, DEFECTDOJO_USERNAME, DEFECTDOJO_PASSWORD
