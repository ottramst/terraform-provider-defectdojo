terraform {
  required_providers {
    defectdojo = {
      source  = "doximity/defectdojo"
      version = "0.0.13"
    }
  }
}

provider "defectdojo" {
  base_url = "https://defectdojo.example.com"
  api_key  = var.defectdojo_api_key
}
