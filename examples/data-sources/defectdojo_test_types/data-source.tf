data "defectdojo_test_types" "all" {}

# Check that DefectDojo knows a scan type before a pipeline imports it.
check "trivy" {
  assert {
    condition     = contains(data.defectdojo_test_types.all.names, "Trivy Scan")
    error_message = "DefectDojo has no Trivy Scan test type."
  }
}

data "defectdojo_test_types" "zap" {
  name = "ZAP Scan"
}

output "zap_test_type_id" {
  value = one(data.defectdojo_test_types.zap.test_types[*].id)
}
