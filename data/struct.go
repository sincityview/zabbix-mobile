package data

import "encoding/json"

type ZabbixRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
	Auth    string         `json:"auth,omitempty"`
	ID      int            `json:"id"`
}

type Problem struct {
	EventID      string `json:"eventid"`
	Name         string `json:"name"`
	Clock        string `json:"clock"`
	Severity     string `json:"severity"`
	Acknowledged string `json:"acknowledged"`
	ObjectID     string `json:"objectid"`
	HostName     string `json:"-"`
}

type Trigger struct {
	TriggerID string `json:"triggerid"`
	Hosts     []Host `json:"hosts"`
}

type Host struct {
	Name string `json:"name"`
}

type ZabbixResponse struct {
	JSONRPC string `json:"jsonrpc"`
	Result json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type jsonRawMessage []byte
func (m *jsonRawMessage) UnmarshalJSON(data []byte) error { *m = data; return nil }
