package data

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"zabbix.local", "https://zabbix.local/api_jsonrpc.php"},
		{"https://zabbix.local", "https://zabbix.local/api_jsonrpc.php"},
		{"https://zabbix.local/", "https://zabbix.local/api_jsonrpc.php"},
		{"https://zabbix.local/zabbix", "https://zabbix.local/zabbix/api_jsonrpc.php"},
		{"https://zabbix.local/zabbix/", "https://zabbix.local/zabbix/api_jsonrpc.php"},
		{"http://zabbix.local/api_jsonrpc.php", "http://zabbix.local/api_jsonrpc.php"},
		{"https://zabbix.local/api_jsonrpc.php?x=1#frag", "https://zabbix.local/api_jsonrpc.php"},
		{"  zabbix.local/api_jsonrpc.php  ", "https://zabbix.local/api_jsonrpc.php"},
	}

	for _, tt := range tests {
		if got := NormalizeURL(tt.in); got != tt.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// fakeZabbix — тестовый сервер, отвечающий в зависимости от method.
type fakeZabbix struct {
	mu       sync.Mutex
	calls    map[string]int
	token    string
	problems []Problem
	hosts    []Trigger
	failWith *ZabbixError
	hostFail bool
	status   int
	delay    time.Duration
}

func newFakeZabbix(t *testing.T) (*fakeZabbix, *httptest.Server) {
	t.Helper()
	f := &fakeZabbix{
		calls: map[string]int{},
		problems: []Problem{
			{EventID: "1", Name: "CPU high", Clock: "1700000000", Severity: "4", ObjectID: "100"},
			{EventID: "2", Name: "Disk full", Clock: "1700000001", Severity: "5", ObjectID: "200"},
		},
		hosts: []Trigger{
			{TriggerID: "100", Hosts: []Host{{Name: "host-a"}}},
			{TriggerID: "200", Hosts: []Host{{Name: "host-b"}}},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		if f.delay > 0 {
			time.Sleep(f.delay)
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
			return
		}
		if f.token != "" && r.Header.Get("Authorization") != "Bearer "+f.token {
			json.NewEncoder(w).Encode(ZabbixResponse{Error: &ZabbixError{Code: -32602, Message: "Invalid params", Data: "bad token"}})
			return
		}

		var req ZabbixRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("не удалось разобрать запрос: %v", err)
			return
		}
		f.calls[req.Method]++

		if f.failWith != nil && req.Method == "problem.get" {
			json.NewEncoder(w).Encode(ZabbixResponse{Error: f.failWith})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "problem.get":
			json.NewEncoder(w).Encode(ZabbixResponse{Result: mustJSON(f.problems)})
		case "trigger.get":
			if f.hostFail {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			json.NewEncoder(w).Encode(ZabbixResponse{Result: mustJSON(f.hosts)})
		default:
			json.NewEncoder(w).Encode(ZabbixResponse{Error: &ZabbixError{Message: "unknown method"}})
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeZabbix) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func TestClient_ProblemsFillsHosts(t *testing.T) {
	f, srv := newFakeZabbix(t)
	f.token = "abc"

	c := NewClient(Config{URL: srv.URL, Token: "abc"})
	problems, err := c.Problems()
	if err != nil {
		t.Fatalf("Problems() error = %v", err)
	}
	if len(problems) != 2 {
		t.Fatalf("получено %d проблем", len(problems))
	}
	if problems[0].HostName != "host-a" || problems[1].HostName != "host-b" {
		t.Fatalf("неверные хосты: %+v", problems)
	}
	if problems[1].SeverityLevel() != 5 {
		t.Errorf("SeverityLevel() = %d", problems[1].SeverityLevel())
	}
}

func TestClient_HostCacheAvoidsSecondRequest(t *testing.T) {
	f, srv := newFakeZabbix(t)

	c := NewClient(Config{URL: srv.URL, Token: "t"})
	for i := 0; i < 3; i++ {
		if _, err := c.Problems(); err != nil {
			t.Fatalf("Problems() error = %v", err)
		}
	}

	if got := f.count("problem.get"); got != 3 {
		t.Errorf("problem.get вызовов = %d, want 3", got)
	}
	if got := f.count("trigger.get"); got != 1 {
		t.Errorf("trigger.get вызовов = %d, want 1 (остальное из кеша)", got)
	}
}

func TestClient_HostCacheExpires(t *testing.T) {
	f, srv := newFakeZabbix(t)

	c := NewClient(Config{URL: srv.URL, Token: "t", HostCacheTTL: time.Minute})
	now := time.Unix(1700000000, 0)
	c.now = func() time.Time { return now }

	if _, err := c.Problems(); err != nil {
		t.Fatalf("Problems() error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := c.Problems(); err != nil {
		t.Fatalf("Problems() error = %v", err)
	}

	if got := f.count("trigger.get"); got != 2 {
		t.Errorf("trigger.get вызовов = %d, want 2 (кеш истёк)", got)
	}
}

func TestClient_InvalidateHostCache(t *testing.T) {
	f, srv := newFakeZabbix(t)

	c := NewClient(Config{URL: srv.URL, Token: "t"})
	if _, err := c.Problems(); err != nil {
		t.Fatalf("Problems() error = %v", err)
	}
	c.InvalidateHostCache()
	if _, err := c.Problems(); err != nil {
		t.Fatalf("Problems() error = %v", err)
	}

	if got := f.count("trigger.get"); got != 2 {
		t.Errorf("trigger.get вызовов = %d, want 2", got)
	}
}

func TestClient_NoRetryOnAPIError(t *testing.T) {
	f, srv := newFakeZabbix(t)
	f.failWith = &ZabbixError{Code: -32602, Message: "Invalid params", Data: "bad token"}

	c := NewClient(Config{URL: srv.URL, Token: "t", RetryCount: 3, RetryDelay: 0})
	_, err := c.Problems()
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}

	var zerr *ZabbixError
	if !errors.As(err, &zerr) {
		t.Fatalf("error = %v, want ZabbixError", err)
	}
	if got := f.count("problem.get"); got != 1 {
		t.Errorf("problem.get вызовов = %d, want 1 (ошибки API не повторяем)", got)
	}
}

func TestClient_NoRetryOnClientError(t *testing.T) {
	f, srv := newFakeZabbix(t)
	f.status = http.StatusUnauthorized

	c := NewClient(Config{URL: srv.URL, Token: "t", RetryCount: 3, RetryDelay: 0})
	_, err := c.Problems()
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}

	var herr *HTTPError
	if !errors.As(err, &herr) || herr.Status != http.StatusUnauthorized {
		t.Fatalf("error = %v, want HTTPError 401", err)
	}
	if got := f.count("problem.get"); got != 0 {
		t.Errorf("problem.get вызовов = %d, want 0 (ответ 401)", got)
	}
}

func TestClient_RetriesServerError(t *testing.T) {
	f, srv := newFakeZabbix(t)
	f.hostFail = true

	c := NewClient(Config{URL: srv.URL, Token: "t", RetryCount: 2, RetryDelay: 0})
	problems, err := c.Problems()

	var herr *HostLookupError
	if !errors.As(err, &herr) {
		t.Fatalf("error = %v, want HostLookupError", err)
	}
	if len(problems) != 2 {
		t.Fatalf("проблемы должны вернуться даже без хостов, получено %d", len(problems))
	}
	if got := f.count("trigger.get"); got != 3 {
		t.Errorf("trigger.get вызовов = %d, want 3 (1 + 2 повтора)", got)
	}
}

func TestClient_BasicAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(ZabbixResponse{Result: mustJSON([]Problem{})})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL, User: "user", Password: "pass"})
	if _, err := c.Problems(); err != nil {
		t.Fatalf("Problems() error = %v", err)
	}
	if gotAuth != basicAuth("user", "pass") {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

func TestClient_RequiresURLAndToken(t *testing.T) {
	if _, err := NewClient(Config{}).Problems(); err == nil {
		t.Fatal("ожидалась ошибка без URL и токена")
	}
}

func TestClient_NormalizesURLWithPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(ZabbixResponse{Result: mustJSON([]Problem{})})
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL, Token: "t"})
	if _, err := c.Problems(); err != nil {
		t.Fatalf("Problems() error = %v", err)
	}
	if !strings.HasSuffix(gotPath, "/api_jsonrpc.php") {
		t.Errorf("path = %q, want .../api_jsonrpc.php", gotPath)
	}
}

func TestProblem_Acknowledged(t *testing.T) {
	if !(Problem{Acknowledged: "1"}).AcknowledgedProblem() {
		t.Error("ожидалось подтверждение")
	}
	if (Problem{Acknowledged: "0"}).AcknowledgedProblem() {
		t.Error("не должно быть подтверждения")
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// Регрессия: на втором опросе (все триггеры уже в кеше) имена хостов должны
// оставаться заполненными — раньше ранний выход из resolveHosts оставлял
// HostName пустым, и в UI пропадала строка с хостом.
func TestClient_HostNamesSurviveFromCache(t *testing.T) {
	f, srv := newFakeZabbix(t)

	c := NewClient(Config{URL: srv.URL, Token: "t"})
	for i := 0; i < 3; i++ {
		problems, err := c.Problems()
		if err != nil {
			t.Fatalf("опрос %d: Problems() error = %v", i+1, err)
		}
		if len(problems) != 2 {
			t.Fatalf("опрос %d: получено %d проблем", i+1, len(problems))
		}
		for _, p := range problems {
			if p.HostName == "" {
				t.Fatalf("опрос %d: у проблемы %s пустое имя хоста", i+1, p.EventID)
			}
			if p.HostName == unknownHost {
				t.Fatalf("опрос %d: проблема %s потеряла хост", i+1, p.EventID)
			}
		}
	}

	// и запрос кеш не должен был повторяться
	if got := f.count("trigger.get"); got != 1 {
		t.Errorf("trigger.get вызовов = %d, want 1", got)
	}
}

// Смешанный случай: часть триггеров в кеше, часть новых.
func TestClient_PartialHostCacheHit(t *testing.T) {
	f, srv := newFakeZabbix(t)

	c := NewClient(Config{URL: srv.URL, Token: "t"})
	if _, err := c.Problems(); err != nil {
		t.Fatalf("Problems() error = %v", err)
	}

	// на сервере появляется новая проблема с новым триггером
	f.mu.Lock()
	f.problems = append(f.problems, Problem{EventID: "3", Name: "Ping loss", Clock: "1700000002", Severity: "3", ObjectID: "300"})
	f.hosts = append(f.hosts, Trigger{TriggerID: "300", Hosts: []Host{{Name: "host-c"}}})
	f.mu.Unlock()

	problems, err := c.Problems()
	if err != nil {
		t.Fatalf("Problems() error = %v", err)
	}

	want := map[string]string{"1": "host-a", "2": "host-b", "3": "host-c"}
	for _, p := range problems {
		if p.HostName != want[p.EventID] {
			t.Errorf("проблема %s: HostName = %q, want %q", p.EventID, p.HostName, want[p.EventID])
		}
	}
	if got := f.count("trigger.get"); got != 2 {
		t.Errorf("trigger.get вызовов = %d, want 2 (второй раз только за новым триггером)", got)
	}
}
