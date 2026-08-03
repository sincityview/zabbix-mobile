package data

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFormatTime(t *testing.T) {
	tests := []struct {
		input       string
		expectMatch bool
	}{
		{"1700000000", true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		result := FormatTime(tt.input)
		if tt.expectMatch {
			if len(result) != 14 || result[2] != '.' || result[5] != ' ' || result[8] != ':' || result[11] != ':' {
				t.Errorf("FormatTime(%q) = %q, expected format 'DD.MM HH:MM:SS'", tt.input, result)
			}
		} else {
			if result != tt.input {
				t.Errorf("FormatTime(%q) = %q, want %q", tt.input, result, tt.input)
			}
		}
	}
}

func TestDataRequestAPI_MissingConfig(t *testing.T) {
	cfg := NewConfig()
	cfg.URL = ""
	cfg.Token = ""

	_, err := DataRequestAPI(cfg)
	if err == nil {
		t.Error("expected error for empty URL/Token")
	}
}

func TestDataRequestAPI_Success(t *testing.T) {
	problemsResp := ZabbixResponse{
		JSONRPC: "2.0",
		Result: mustMarshal([]Problem{
			{EventID: "1", Name: "Test Problem", Clock: "1700000000", Severity: "3", ObjectID: "100"},
		}),
	}

	triggersResp := ZabbixResponse{
		JSONRPC: "2.0",
		Result: mustMarshal([]Trigger{
			{TriggerID: "100", Hosts: []Host{{Name: "TestHost"}}},
		}),
	}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			json.NewEncoder(w).Encode(problemsResp)
		} else {
			json.NewEncoder(w).Encode(triggersResp)
		}
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.URL = server.URL
	cfg.Token = "test-token"
	cfg.RetryCount = 0

	problems, err := DataRequestAPI(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(problems) != 1 {
		t.Fatalf("expected 1 problem, got %d", len(problems))
	}

	if problems[0].HostName != "TestHost" {
		t.Errorf("expected HostName 'TestHost', got %q", problems[0].HostName)
	}
}

func TestDataRequestAPI_Retry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		resp := ZabbixResponse{
			JSONRPC: "2.0",
			Result:  mustMarshal([]Problem{}),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.URL = server.URL
	cfg.Token = "test-token"
	cfg.RetryCount = 3
	cfg.RetryDelay = 0

	_, err := DataRequestAPI(cfg)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestBasicAuth(t *testing.T) {
	result := basicAuth("user", "pass")
	expected := "Basic dXNlcjpwYXNz"
	if result != expected {
		t.Errorf("basicAuth() = %q, want %q", result, expected)
	}
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
