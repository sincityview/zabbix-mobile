package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func DataRequestAPI(zabbixURL, apiToken string) ([]Problem, error) {
    if zabbixURL == "" || apiToken == "" {
        return nil, fmt.Errorf("URL or Token not set")
    }

	if zabbixURL == "" || apiToken == "" {
		return nil, fmt.Errorf("URL or Token not set")
	}

	reqProblem := ZabbixRequest{
		JSONRPC: "2.0",
		Method:  "problem.get",
		Params: map[string]any{
			"output":      []string{"eventid", "name", "clock", "severity", "objectid"},
			"sortfield":   []string{"eventid"},
			"sortorder":   "DESC",
			"suppressed":  false,
			"recent":      true,
		},
		Auth: apiToken,
		ID:   1,
	}

	var problems []Problem
	err := apiCall(zabbixURL, reqProblem, &problems)
	if err != nil {
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
			"filter": map[string]any{
				"status": 0,
			},
		},
		Auth: apiToken,
		ID:   2,
	}

	var triggers []Trigger
	err = apiCall(zabbixURL, reqTriggers, &triggers)
	if err != nil {
		return problems, nil
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

func apiCall(url string, request interface{}, target interface{}) error {
    data, err := json.Marshal(request)
    if err != nil {
        return err
    }
    
    resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    var zResp struct {
        Result json.RawMessage `json:"result"`
        Error  interface{}     `json:"error"`
    }

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
