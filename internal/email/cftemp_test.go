package email

import "testing"

func TestNewCFTempMailServiceTrimsBaseURLAndDomain(t *testing.T) {
	svc := NewCFTempMailService("https://mail.example.com/", "admin", "", " example.com ", "")
	provider, ok := svc.(*CFProvider)
	if !ok {
		t.Fatalf("NewCFTempMailService() type = %T, want *CFProvider", svc)
	}
	if provider.baseURL != "https://mail.example.com" {
		t.Fatalf("baseURL = %q, want trimmed URL", provider.baseURL)
	}
	if provider.domain != "example.com" {
		t.Fatalf("domain = %q, want trimmed domain", provider.domain)
	}
}

func TestParseRawBodyExtractsSubjectAndBody(t *testing.T) {
	raw := "Subject: Your verification code\r\n\r\n<html><body>Code: <b>654321</b></body></html>"
	content := parseRawBody(raw)
	if got := ExtractCode(content); got != "654321" {
		t.Fatalf("ExtractCode(parseRawBody()) = %q, want 654321", got)
	}
}
