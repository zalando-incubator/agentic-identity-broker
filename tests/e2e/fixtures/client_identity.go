package fixtures

const (
	ReportsClientV1 = "reports-client-v1"
	ReportsClientV2 = "reports-client-v2"
	ReportsSecretV1 = "synthetic-reports-secret-v1"
	ReportsSecretV2 = "synthetic-reports-secret-v2"
)

func ClientIdentityServiceRequest(providerURL, source string) map[string]any {
	request := CredentialServiceRequest("reports-prod", providerURL, source)
	request["display_name"] = "Reports Production"
	if source != "filesystem" {
		request["client_id"] = ReportsClientV1
		request["client_secret"] = ReportsSecretV1
	}
	return request
}

func ClientIdentityCalendarRequest(providerURL string) map[string]any {
	request := CredentialServiceRequest("calendar-prod", providerURL, "stored")
	request["display_name"] = "Calendar Production"
	request["client_id"] = "calendar-client-v1"
	request["client_secret"] = "synthetic-calendar-secret"
	request["protected_resources"] = []string{providerURL + "/calendar-resource"}
	return request
}
