package transcript

import (
	"encoding/json"
	"strings"
	"time"
)

// UsageRecord is what one assistant record says it cost: the API's usage block, the model, when.
// One API response is written as several records sharing message.id with the same usage: a reader counts an id once.
type UsageRecord struct {
	MessageID                                                       string
	Model                                                           string
	Geo                                                             string // inference_geo: "us" costs a tenth more
	Speed                                                           string // "fast" is priced from another table
	At                                                              time.Time
	Sidechain                                                       bool
	Cwd                                                             string
	Input, Output, CacheRead, CacheWrite5m, CacheWrite1h, WebSearch int64
}

type usageRecord struct {
	Type        string `json:"type"`
	Timestamp   string `json:"timestamp"`
	IsSidechain bool   `json:"isSidechain"`
	IsAPIError  bool   `json:"isApiErrorMessage"`
	Cwd         string `json:"cwd"`
	Message     struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input         int64 `json:"input_tokens"`
			Output        int64 `json:"output_tokens"`
			CacheRead     int64 `json:"cache_read_input_tokens"`
			CacheCreate   int64 `json:"cache_creation_input_tokens"`
			CacheCreation struct {
				OneHour int64 `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
			ServerToolUse struct {
				WebSearch int64 `json:"web_search_requests"`
			} `json:"server_tool_use"`
			Geo   string `json:"inference_geo"`
			Speed string `json:"speed"`
		} `json:"usage"`
	} `json:"message"`
}

// UsageOf reads one raw record; false for anything but an assistant record with usage behind a real API call.
// An API error is written as an assistant record too (isApiErrorMessage, a "<synthetic>" model): no call behind it.
func UsageOf(line []byte) (UsageRecord, bool) {
	var r usageRecord
	if json.Unmarshal(line, &r) != nil || r.Type != "assistant" || r.Message.Usage == nil || r.IsAPIError {
		return UsageRecord{}, false
	}
	if strings.HasPrefix(r.Message.Model, "<") {
		return UsageRecord{}, false
	}
	u := r.Message.Usage
	oneHour := min(u.CacheCreation.OneHour, u.CacheCreate)
	at, _ := time.Parse(time.RFC3339Nano, r.Timestamp)
	return UsageRecord{
		MessageID: r.Message.ID, Model: r.Message.Model, Geo: u.Geo, Speed: u.Speed, At: at, Sidechain: r.IsSidechain, Cwd: r.Cwd,
		Input: u.Input, Output: u.Output, CacheRead: u.CacheRead,
		CacheWrite5m: u.CacheCreate - oneHour, CacheWrite1h: oneHour, WebSearch: u.ServerToolUse.WebSearch,
	}, true
}
