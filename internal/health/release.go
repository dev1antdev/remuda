package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// remuda's own releases on GitHub, read for the newest version; the updates package offers them.

// ReleasesRepo is where remuda's releases live.
const ReleasesRepo = "dev1antdev/remuda"

var releasesURL = "https://api.github.com/repos/" + ReleasesRepo + "/releases/latest"

// Release is the newest release as GitHub lists it.
type Release struct {
	Tag       string            `json:"tag"`
	Version   string            `json:"version"`
	Assets    map[string]string `json:"-"` // asset name → download URL
	Checksums string            `json:"-"` // the checksums.txt asset; "" when the release has none
}

func fetchRelease(ctx context.Context, url string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "remuda")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Release{}, &httpError{url, res.StatusCode}
	}
	return parseRelease(res.Body)
}

// parseRelease reads GitHub's release object: the tag and the assets.
func parseRelease(r io.Reader) (Release, error) {
	var m struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return Release{}, err
	}
	rel := Release{Tag: m.Tag, Version: semver.FindString(m.Tag), Assets: map[string]string{}}
	if rel.Version == "" {
		return Release{}, fmt.Errorf("release tag %q carries no version", m.Tag)
	}
	for _, a := range m.Assets {
		if a.Name == "checksums.txt" {
			rel.Checksums = a.URL
			continue
		}
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}
