package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseProxyListTrimsAndSkipsEmpty(t *testing.T) {
	got := parseProxyList(" socks5://a:1, ,http://b:2,, socks5://c:3 ")
	want := []string{"socks5://a:1", "http://b:2", "socks5://c:3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseProxyList() = %#v, want %#v", got, want)
	}
}

func TestSaveResultsAppendsOnlySuccessAndKeepsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	existing := []map[string]interface{}{
		{"email": "old@example.com", "refreshToken": "old-token"},
	}
	results := []map[string]interface{}{
		{
			"status":        "success",
			"email":         "new@example.com",
			"client_id":     "cid",
			"client_secret": "secret",
			"aws_token":     map[string]interface{}{"refreshToken": "new-token"},
			"verify": map[string]interface{}{
				"credit_used":  1,
				"credit_limit": 50,
				"subscription": "Free",
			},
		},
		{"status": "failed", "email": "failed@example.com"},
	}

	if err := saveResults(existing, results, path); err != nil {
		t.Fatalf("saveResults() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got []map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("saved %d records, want 2: %#v", len(got), got)
	}
	if got[0]["email"] != "old@example.com" || got[1]["email"] != "new@example.com" {
		t.Fatalf("unexpected saved records: %#v", got)
	}
	if got[1]["refreshToken"] != "new-token" {
		t.Fatalf("refreshToken = %#v, want new-token", got[1]["refreshToken"])
	}
}
