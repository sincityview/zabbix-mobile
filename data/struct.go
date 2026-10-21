package data

import (
	"encoding/json"
	"strconv"
)

// ZabbixRequest — тело JSON-RPC запроса к API.
type ZabbixRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
	ID      int            `json:"id"`
}

// Problem — одна проблема из problem.get.
type Problem struct {
	EventID      string `json:"eventid"`
	Name         string `json:"name"`
	Clock        string `json:"clock"`
	Severity     string `json:"severity"`
	Acknowledged string `json:"acknowledged"`
	ObjectID     string `json:"objectid"`
	// HostName заполняется из trigger.get, а также сохраняется в снапшоте
	// последних проблем (см. snapshot.go) — поэтому у поля есть json-тег.
	HostName string `json:"host_name,omitempty"`
}

// SeverityLevel возвращает важность числом (0..5) или -1 для неизвестного значения.
func (p Problem) SeverityLevel() int {
	n, err := strconv.Atoi(p.Severity)
	if err != nil || n < 0 || n > 5 {
		return -1
	}
	return n
}

// AcknowledgedProblem сообщает, подтверждена ли проблема оператором.
func (p Problem) AcknowledgedProblem() bool {
	return p.Acknowledged == "1"
}

// Trigger — запись из trigger.get.
type Trigger struct {
	TriggerID string `json:"triggerid"`
	Hosts     []Host `json:"hosts"`
}

// Host — хост триггера.
type Host struct {
	Name string `json:"name"`
}

// ZabbixResponse — ответ JSON-RPC.
type ZabbixResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *ZabbixError    `json:"error"`
}
