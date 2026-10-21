package data

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config — параметры обращения к Zabbix API.
type Config struct {
	URL        string
	Token      string
	User       string
	Password   string
	Limit      int
	SelfSigned bool

	// RetryCount — количество повторов после первой неудачной попытки.
	RetryCount int
	// RetryDelay — базовая задержка экспоненциального backoff (0 — без паузы).
	RetryDelay time.Duration
	// Timeout — таймаут одного HTTP-запроса.
	Timeout time.Duration
	// HostCacheTTL — сколько помнить соответствие "триггер → хост".
	HostCacheTTL time.Duration
}

// NewConfig возвращает конфигурацию с разумными значениями по умолчанию.
func NewConfig() Config {
	return Config{
		Limit:        DefaultLimit,
		RetryCount:   2,
		RetryDelay:   500 * time.Millisecond,
		Timeout:      20 * time.Second,
		HostCacheTTL: 10 * time.Minute,
	}
}

// Cleartext сообщает, что адрес сервера использует http:// без шифрования.
// UI должен предупредить об этом: токен уйдёт по сети открытым текстом.
func (c Config) Cleartext() bool {
	return strings.HasPrefix(strings.ToLower(NormalizeURL(c.URL)), "http://")
}

// HostLookupError означает, что проблемы получены, но имена хостов подтянуть не
// удалось. Данные при этом пригодны для показа.
type HostLookupError struct {
	Err error
}

func (e *HostLookupError) Error() string {
	return "не удалось получить имена хостов: " + e.Err.Error()
}

func (e *HostLookupError) Unwrap() error { return e.Err }

// HTTPError — неожиданный HTTP-статус ответа.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("HTTP %d %s", e.Status, http.StatusText(e.Status))
	}
	return fmt.Sprintf("HTTP %d %s: %s", e.Status, http.StatusText(e.Status), e.Body)
}

// ZabbixError — ошибка, вернувшаяся в теле JSON-RPC ответа.
type ZabbixError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func (e *ZabbixError) Error() string {
	switch {
	case e.Data != "":
		return fmt.Sprintf("Zabbix API [%d]: %s (%s)", e.Code, e.Message, e.Data)
	case e.Message != "":
		return fmt.Sprintf("Zabbix API [%d]: %s", e.Code, e.Message)
	default:
		return fmt.Sprintf("Zabbix API [%d]", e.Code)
	}
}

// NormalizeURL приводит адрес сервера к виду, пригодному для API:
// добавляет схему https:// и путь /api_jsonrpc.php, если их нет.
func NormalizeURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}

	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return s
	}

	path := strings.TrimSuffix(u.Path, "/")
	switch {
	case path == "":
		path = "/api_jsonrpc.php"
	case strings.HasSuffix(path, ".php"):
		// уже указывает на файл API
	default:
		path += "/api_jsonrpc.php"
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// unknownHost подставляется, если имя хоста получить не удалось.
const unknownHost = "Unknown Host"

// Client — клиент Zabbix API. Его стоит создавать один раз и переиспользовать:
// так сохраняется keep-alive соединение и кеш хостов между опросами.
type Client struct {
	cfg  Config
	http *http.Client
	url  string

	mu    sync.Mutex
	hosts map[string]hostEntry

	now func() time.Time
}

type hostEntry struct {
	name     string
	cachedAt time.Time
}

// NewClient создаёт клиент по конфигурации.
func NewClient(cfg Config) *Client {
	defaults := NewConfig()
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaults.Timeout
	}
	if cfg.Limit <= 0 {
		cfg.Limit = defaults.Limit
	}
	if cfg.HostCacheTTL <= 0 {
		cfg.HostCacheTTL = defaults.HostCacheTTL
	}
	if cfg.RetryCount < 0 {
		cfg.RetryCount = 0
	}

	return &Client{
		cfg:   cfg,
		http:  newHTTPClient(cfg.SelfSigned, cfg.Timeout),
		url:   NormalizeURL(cfg.URL),
		hosts: make(map[string]hostEntry),
		now:   time.Now,
	}
}

// newHTTPClient создаёт отдельный транспорт: TLS-настройки зависят от флага
// самоподписанного сертификата, поэтому клиентов максимум два, и они
// переиспользуются (иначе на каждый опрос шёл новый TLS handshake).
func newHTTPClient(selfSigned bool, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &http.Client{
		Transport:     sharedTransport(selfSigned),
		Timeout:       timeout,
		CheckRedirect: sameHostRedirect,
	}
}

// sameHostRedirect разрешает редиректы только на тот же хост.
//
// Заголовок Authorization Go копирует при редиректе на тот же хост, а вот
// переадресацию на чужой домен (утечка токена) нужно блокировать.
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if len(via) > 3 {
		return errors.New("слишком много перенаправлений")
	}
	if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		return fmt.Errorf("перенаправление на другой хост запрещено (%s)", req.URL.Host)
	}
	return nil
}

var (
	transportMu    sync.Mutex
	transportCache = map[bool]*http.Transport{}
)

func sharedTransport(selfSigned bool) *http.Transport {
	transportMu.Lock()
	defer transportMu.Unlock()
	if t, ok := transportCache[selfSigned]; ok {
		return t
	}
	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: selfSigned, // #nosec G402 — осознанный выбор пользователя
			MinVersion:         tls.VersionTLS12,
		},
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
	transportCache[selfSigned] = t
	return t
}

// InvalidateHostCache сбрасывает кеш "триггер → хост" (например, при смене
// сервера или токена).
func (c *Client) InvalidateHostCache() {
	c.mu.Lock()
	c.hosts = make(map[string]hostEntry)
	c.mu.Unlock()
}

// Problems забирает актуальные проблемы и подставляет имена хостов.
func (c *Client) Problems() ([]Problem, error) {
	if c.url == "" || (c.cfg.Token == "" && c.cfg.User == "") {
		return nil, errors.New("URL и токен обязательны")
	}

	params := map[string]any{
		"output":     []string{"eventid", "name", "clock", "severity", "objectid", "acknowledged"},
		"sortfield":  []string{"eventid"},
		"sortorder":  "DESC",
		"suppressed": false,
		"recent":     true,
	}
	if c.cfg.Limit > 0 {
		params["limit"] = c.cfg.Limit
	}

	var problems []Problem
	err := c.call(ZabbixRequest{
		JSONRPC: "2.0",
		Method:  "problem.get",
		Params:  params,
		ID:      1,
	}, &problems)
	if err != nil {
		return nil, err
	}
	if len(problems) == 0 {
		return problems, nil
	}

	if err := c.resolveHosts(problems); err != nil {
		return problems, &HostLookupError{Err: err}
	}
	return problems, nil
}

// resolveHosts заполняет HostName, запрашивая у сервера только те триггеры,
// которых нет в кеше. На повторных опросах это экономит целый HTTP-запрос.
func (c *Client) resolveHosts(problems []Problem) error {
	ids := make([]string, 0, len(problems))
	seen := make(map[string]struct{}, len(problems))
	for _, p := range problems {
		if p.ObjectID == "" {
			continue
		}
		if _, ok := seen[p.ObjectID]; ok {
			continue
		}
		seen[p.ObjectID] = struct{}{}
		ids = append(ids, p.ObjectID)
	}

	// Запрашиваем только то, чего нет в кеше. Если в кеше есть всё — сетевой
	// запрос не нужен, но имена хостов всё равно надо проставить ниже.
	if fresh := c.freshHosts(ids); len(fresh) > 0 {
		var triggers []Trigger
		err := c.call(ZabbixRequest{
			JSONRPC: "2.0",
			Method:  "trigger.get",
			Params: map[string]any{
				"triggerids":  fresh,
				"selectHosts": []string{"name"},
				"output":      []string{"triggerid"},
			},
			ID: 2,
		}, &triggers)
		if err != nil {
			return err
		}
		c.storeHosts(triggers)
	}

	c.mu.Lock()
	for i := range problems {
		if e, ok := c.hosts[problems[i].ObjectID]; ok && e.name != "" {
			problems[i].HostName = e.name
			continue
		}
		problems[i].HostName = unknownHost
	}
	c.mu.Unlock()
	return nil
}

// freshHosts возвращает ID триггеров, которых нет в кеше (или срок истёк).
func (c *Client) freshHosts(ids []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if e, ok := c.hosts[id]; ok && now.Sub(e.cachedAt) < c.cfg.HostCacheTTL {
			continue
		}
		out = append(out, id)
	}
	return out
}

func (c *Client) storeHosts(triggers []Trigger) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	for _, t := range triggers {
		if len(t.Hosts) == 0 {
			continue
		}
		c.hosts[t.TriggerID] = hostEntry{name: t.Hosts[0].Name, cachedAt: now}
	}
}

// DataRequestAPI — обёртка «в один вызов» без переиспользования клиента.
// Оставлена для обратной совместимости и тестов; в приложении используется Client.
func DataRequestAPI(cfg Config) ([]Problem, error) {
	return NewClient(cfg).Problems()
}

// call сериализует запрос, отправляет его с повторами и разбирает ответ.
func (c *Client) call(request ZabbixRequest, target any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	attempts := c.cfg.RetryCount + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if delay := c.backoff(attempt); delay > 0 {
				time.Sleep(delay)
			}
		}

		lastErr = c.doRequest(body, target)
		if lastErr == nil {
			return nil
		}
		if !retryable(lastErr) {
			return lastErr
		}
	}
	return fmt.Errorf("все попытки исчерпаны: %w", lastErr)
}

// backoff — экспоненциальная задержка с потолком 5 секунд.
func (c *Client) backoff(attempt int) time.Duration {
	d := c.cfg.RetryDelay
	if d <= 0 {
		return 0
	}
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= 5*time.Second {
			return 5 * time.Second
		}
	}
	return d
}

// retryable: повторяем только сетевые сбои и 5xx/429. Ошибку API (например,
// неверный токен) повторять бессмысленно — только зря грузим сервер.
func retryable(err error) bool {
	var apiErr *ZabbixError
	if errors.As(err, &apiErr) {
		return false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Status == http.StatusTooManyRequests || httpErr.Status >= 500
	}
	var jsonErr *json.SyntaxError
	if errors.As(err, &jsonErr) {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return true
}

func (c *Client) doRequest(body []byte, target any) error {
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "zabbix-monitor")

	switch {
	case c.cfg.Token != "":
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	case c.cfg.User != "":
		req.Header.Set("Authorization", basicAuth(c.cfg.User, c.cfg.Password))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &HTTPError{Status: resp.StatusCode, Body: truncate(strings.TrimSpace(string(raw)), 200)}
	}

	var zResp ZabbixResponse
	if err := json.Unmarshal(raw, &zResp); err != nil {
		return fmt.Errorf("некорректный ответ JSON-RPC: %w", err)
	}
	if zResp.Error != nil {
		return zResp.Error
	}
	if len(zResp.Result) == 0 {
		return nil
	}
	return json.Unmarshal(zResp.Result, target)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

// FormatTime превращает unix-время из API в "02.01 15:04:05".
func FormatTime(clock string) string {
	timestamp, err := strconv.ParseInt(clock, 10, 64)
	if err != nil {
		return clock
	}
	return time.Unix(timestamp, 0).Format("02.01 15:04:05")
}
