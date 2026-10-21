package data

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSecret_PrintIsMasked(t *testing.T) {
	s := Secret("super-secret-token")

	for _, got := range []string{s.String(), fmt.Sprint(s), fmt.Sprintf("%v", s), fmt.Sprintf("%s", s), fmt.Sprintf("%#v", s)} {
		if strings.Contains(got, "super-secret-token") {
			t.Fatalf("секрет утёк в вывод: %q", got)
		}
	}
	if s.String() != "***" {
		t.Errorf("String() = %q", s.String())
	}
	if Secret("").String() != "" {
		t.Error("пустой секрет должен печататься как пустая строка")
	}
}

func TestSecret_SettingsPrintIsMasked(t *testing.T) {
	st := Settings{
		URL:      "https://host/api_jsonrpc.php",
		Token:    Secret("token-value"),
		User:     "admin",
		Password: Secret("pass-value"),
	}

	rendered := fmt.Sprintf("%+v %v %#v", st, st, st)
	if strings.Contains(rendered, "token-value") || strings.Contains(rendered, "pass-value") {
		t.Fatalf("секреты утекли через fmt: %s", rendered)
	}
}

func TestSecret_JSONKeepsValue(t *testing.T) {
	st := Settings{URL: "https://host", Token: Secret("t0k"), Password: Secret("p4ss")}

	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(raw), `"token":"t0k"`) {
		t.Fatalf("токен не сохранён в JSON: %s", raw)
	}

	var back Settings
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if back.Token.Value() != "t0k" || back.Password.Value() != "p4ss" {
		t.Fatalf("после round-trip: token=%q pass=%q", back.Token.Value(), back.Password.Value())
	}
}

func TestSecret_IsSet(t *testing.T) {
	tests := map[Secret]bool{
		"":      false,
		"   ":   false,
		"token": true,
	}
	for secret, want := range tests {
		if got := secret.IsSet(); got != want {
			t.Errorf("Secret(%q).IsSet() = %v, want %v", string(secret), got, want)
		}
	}
}
