package transcript

import (
	"bytes"
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
	Window                                                          int // the model's context window when the file says it (Codex does), 0 otherwise
	Input, Output, CacheRead, CacheWrite5m, CacheWrite1h, WebSearch int64
}

// UsageScanner reads a session file's API responses a line at a time: Claude Code's assistant records, Codex's
// token records (with the model and the window the records before them named), Gemini's answers with tokens.
type UsageScanner interface {
	Scan(line []byte) (UsageRecord, bool)
}

type claudeUsageScanner struct{}

func (claudeUsageScanner) Scan(line []byte) (UsageRecord, bool) {
	if !bytes.Contains(line, []byte(`"usage"`)) {
		return UsageRecord{}, false
	}
	return UsageOf(line)
}

// NewUsageScanner is the scanner for a session file, by the format its path says.
func NewUsageScanner(path string) UsageScanner {
	switch FormatOf(path) {
	case Codex:
		return &codexUsageScanner{}
	case Gemini:
		return geminiUsageScanner{}
	}
	return claudeUsageScanner{}
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
