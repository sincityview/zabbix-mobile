package data

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSameHostRedirect(t *testing.T) {
	makeReq := func(host string) *http.Request {
		return &http.Request{URL: mustURL("https://" + host + "/api_jsonrpc.php")}
	}
	origin := makeReq("zabbix.example.com")

	// тот же хост — можно
	if err := sameHostRedirect(makeReq("zabbix.example.com"), []*http.Request{origin}); err != nil {
		t.Errorf("редирект на тот же хост отклонён: %v", err)
	}
	// другой хост — нельзя (иначе утечёт Authorization)
	if err := sameHostRedirect(makeReq("evil.example.com"), []*http.Request{origin}); err == nil {
		t.Error("редирект на чужой хост должен быть запрещён")
	}
	// слишком длинная цепочка
	via := make([]*http.Request, 4)
	for i := range via {
		via[i] = origin
	}
	if err := sameHostRedirect(makeReq("zabbix.example.com"), via); err == nil {
		t.Error("цепочка из 5 редиректов должна отклоняться")
	}
}

// Проверяем, что клиент действительно не ходит по редиректам на чужой хост,
// а значит не отдаёт туда токен.
func TestClient_RejectsCrossHostRedirect(t *testing.T) {
	leaked := false
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer evil.Close()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/api_jsonrpc.php", http.StatusTemporaryRedirect)
	}))
	defer proxy.Close()

	c := NewClient(Config{URL: proxy.URL, Token: "secret", RetryCount: 0})
	_, err := c.Problems()
	if err == nil {
		t.Fatal("ожидалась ошибка из-за запрещённого редиректа")
	}
	if leaked {
		t.Fatal("токен утёк на чужой хост через редирект")
	}
}

func TestConfig_Cleartext(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://zabbix.local", true},
		{"HTTP://zabbix.local/api_jsonrpc.php", true},
		{"https://zabbix.local", false},
		{"zabbix.local", false}, // по умолчанию https
		{"", false},
	}
	for _, tt := range tests {
		cfg := Config{URL: tt.url}
		if got := cfg.Cleartext(); got != tt.want {
			t.Errorf("Config{URL: %q}.Cleartext() = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestSettings_MovesUserInfoOutOfURL(t *testing.T) {
	st := Settings{URL: "https://admin:s3cret@zabbix.local/zabbix"}
	got := st.Normalized()

	if got.User != "admin" {
		t.Errorf("User = %q", got.User)
	}
	if got.Password.Value() != "s3cret" {
		t.Errorf("Password = %q", got.Password.Value())
	}
	if contains(got.URL, "admin") || contains(got.URL, "s3cret") {
		t.Errorf("учётные данные остались в URL: %q", got.URL)
	}
	if got.URL != "https://zabbix.local/zabbix/api_jsonrpc.php" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestStore_ClearRemovesEverything(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(sampleSettings()); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(sampleSettings()); err != nil { // создаст .bak
		t.Fatal(err)
	}

	if err := s.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if _, err := s.Load(); !errors.Is(err, ErrNoSettings) {
		t.Fatalf("после Clear() остались настройки: %v", err)
	}
}

func TestClient_ProblemWithoutHostFallsBack(t *testing.T) {
	f, srv := newFakeZabbix(t)
	f.hosts = nil // сервер не знает хостов

	c := NewClient(Config{URL: srv.URL, Token: "t"})
	problems, err := c.Problems()
	if err != nil {
		t.Fatalf("Problems() error = %v", err)
	}
	for _, p := range problems {
		if p.HostName != unknownHost {
			t.Errorf("HostName = %q, want %q", p.HostName, unknownHost)
		}
	}
}

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
