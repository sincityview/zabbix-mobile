package data

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

type Config struct {
	URL        string
	Token      string
	User       string
	Password   string
	Limit      int
	SelfSigned bool
	RetryCount int
	RetryDelay time.Duration
}

func NewConfig() Config {
	return Config{
		Limit:      200,
		RetryCount: 3,
		RetryDelay: 1 * time.Second,
	}
}

func (c Config) httpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: c.SelfSigned,
			},
		},
		Timeout: 30 * time.Second,
	}
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

func DataRequestAPI(cfg Config) ([]Problem, error) {
	if cfg.URL == "" || cfg.Token == "" {
		return nil, fmt.Errorf("URL и Token обязательны")
	}

	if cfg.RetryCount <= 0 {
		cfg.RetryCount = 3
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 1 * time.Second
	}

	reqProblem := ZabbixRequest{
		JSONRPC: "2.0",
		Method:  "problem.get",
		Params: map[string]any{
			"output":     []string{"eventid", "name", "clock", "severity", "objectid", "acknowledged"},
			"sortfield":  []string{"eventid"},
			"sortorder":  "DESC",
			"suppressed": false,
			"recent":     true,
		},
		Auth: cfg.Token,
		ID:   1,
	}

	if cfg.Limit > 0 {
		reqProblem.Params["limit"] = cfg.Limit
	}

	var problems []Problem
	if err := apiCall(cfg, reqProblem, &problems); err != nil {
		return nil, err
	}

	if len(problems) == 0 {
		return problems, nil
	}

	triggerIDs := make([]string, 0, len(problems))
	for _, p := range problems {
		triggerIDs = append(triggerIDs, p.ObjectID)
	}

	reqTriggers := ZabbixRequest{
		JSONRPC: "2.0",
		Method:  "trigger.get",
		Params: map[string]any{
			"triggerids":  triggerIDs,
			"selectHosts": []string{"name"},
			"output":      []string{"triggerid"},
		},
		Auth: cfg.Token,
		ID:   2,
	}

	var triggers []Trigger
	if err := apiCall(cfg, reqTriggers, &triggers); err != nil {
		return problems, fmt.Errorf("failed to fetch triggers: %w", err)
	}

	hostMap := make(map[string]string)
	for _, t := range triggers {
		if len(t.Hosts) > 0 {
			hostMap[t.TriggerID] = t.Hosts[0].Name
		}
	}

	for i := range problems {
		if name, ok := hostMap[problems[i].ObjectID]; ok {
			problems[i].HostName = name
		} else {
			problems[i].HostName = "Unknown Host"
		}
	}

	return problems, nil
}

func apiCall(cfg Config, request interface{}, target interface{}) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	var lastErr error
	client := cfg.httpClient()

	for attempt := 0; attempt <= cfg.RetryCount; attempt++ {
		if attempt > 0 {
			delay := cfg.RetryDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			time.Sleep(delay)
		}

		lastErr = doRequest(client, cfg, body, target)
		if lastErr == nil {
			return nil
		}
	}

	return fmt.Errorf("все попытки исчерпаны: %w", lastErr)
}

func doRequest(client *http.Client, cfg Config, body []byte, target interface{}) error {
	req, err := http.NewRequest("POST", cfg.URL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if cfg.User != "" {
		req.Header.Set("Authorization", basicAuth(cfg.User, cfg.Password))
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var zResp ZabbixResponse
	if err := json.NewDecoder(resp.Body).Decode(&zResp); err != nil {
		return err
	}

	if zResp.Error != nil {
		return fmt.Errorf("Zabbix API Error: %v", zResp.Error)
	}

	return json.Unmarshal(zResp.Result, target)
}

func FormatTime(clock string) string {
	timestamp, err := strconv.ParseInt(clock, 10, 64)
	if err != nil {
		return clock
	}
	return time.Unix(timestamp, 0).Format("02.01 15:04:05")
}
