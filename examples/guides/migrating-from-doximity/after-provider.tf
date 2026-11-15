terraform {
  required_providers {
    defectdojo = {
      source  = "ottramst/defectdojo"
      version = "~> 1.0"
    }
  }
}

provider "defectdojo" {
  url       = "https://defectdojo.example.com"
  api_token = var.defectdojo_api_token
}
