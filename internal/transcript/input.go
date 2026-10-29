package transcript

import (
	"encoding/json"
	"regexp"
)

var cutFieldRe = regexp.MustCompile(`"(file_path|notebook_path|path|command|cmd|pattern|url|description|query|input)":"((?:[^"\\]|\\.)*)"`)

// InputField is the first of the keys found in a tool's input: an object, or the string the server cut a big input to.
func InputField(input json.RawMessage, keys ...string) string {
	if len(input) == 0 {
		return ""
	}
	var obj map[string]any
	if json.Unmarshal(input, &obj) == nil {
		for _, key := range keys {
			if s, ok := obj[key].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	var cut string
	if json.Unmarshal(input, &cut) != nil {
		return ""
	}
	found := map[string]string{}
	for _, m := range cutFieldRe.FindAllStringSubmatch(cut, -1) {
		var value string
		if json.Unmarshal([]byte(`"`+m[2]+`"`), &value) == nil {
			found[m[1]] = value
		}
	}
	for _, key := range keys {
		if found[key] != "" {
			return found[key]
		}
	}
	return ""
}
