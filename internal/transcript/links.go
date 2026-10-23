package transcript

import "regexp"

// Link is a pull or merge request an agent named in an answer: its address and a short label, "PR #15" or "MR !9".
type Link struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

var requestRe = regexp.MustCompile(`https?://[^\s<>()\[\]"'` + "`" + `]+?/(pull|merge_requests)/(\d+)\b`)

// PullRequest is the last pull or merge request the assistant named in its text: GitHub's /pull/N, GitLab's /merge_requests/N.
func PullRequest(turns []Turn) (Link, bool) {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role != "assistant" {
			continue
		}
		for j := len(turns[i].Parts) - 1; j >= 0; j-- {
			part := turns[i].Parts[j]
			if part.Type != "text" {
				continue
			}
			found := requestRe.FindAllStringSubmatch(part.Text, -1)
			if len(found) == 0 {
				continue
			}
			last := found[len(found)-1]
			return Link{URL: last[0], Label: requestLabel(last[1], last[2])}, true
		}
	}
	return Link{}, false
}

func requestLabel(kind, number string) string {
	if kind == "pull" {
		return "PR #" + number
	}
	return "MR !" + number
}
