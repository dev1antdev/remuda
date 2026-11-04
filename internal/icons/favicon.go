package icons

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A site's icon, fetched once at the person's word in the icon dialog: the page's own <link rel="…icon"> tags
// ranked, /favicon.ico last, the first that answers a picture of at most MaxImage kept as a data URL. Nothing is
// remembered and nothing is fetched again later.

// MaxPage is how much of the site's page is read for its <link> tags.
const MaxPage = 1 << 20

// MaxRedirects is how many redirects one fetch follows.
const MaxRedirects = 5

// MaxIconLinks is how many of the page's own icon links are tried, the best ranked: a page may name thousands.
const MaxIconLinks = 6

// FaviconTimeout is how long the whole search may take, the page and every icon tried.
var FaviconTimeout = 30 * time.Second

// ErrNotWeb is an address that is not a web page.
var ErrNotWeb = errors.New("the address must be an http or https web page")

// Client is the one the favicon fetch uses: ten seconds in all, at most MaxRedirects redirects.
func Client() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirects {
				return fmt.Errorf("more than %d redirects", MaxRedirects)
			}
			return nil
		},
	}
}

// SiteURL reads the address the person typed: a bare host is taken as https, anything but http and https refused.
func SiteURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrNotWeb
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	site, err := url.Parse(raw)
	if err != nil || (site.Scheme != "http" && site.Scheme != "https") || site.Host == "" {
		return nil, ErrNotWeb
	}
	return site, nil
}

// Favicon finds the site's icon and answers it as a data URL with the address it came from.
func Favicon(ctx context.Context, client *http.Client, raw string) (string, string, error) {
	site, err := SiteURL(raw)
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(ctx, FaviconTimeout)
	defer cancel()
	page := site
	var candidates []iconLink
	body, final, pageErr := fetch(ctx, client, site.String(), MaxPage)
	if pageErr == nil {
		page = final
		candidates = rankLinks(linksIn(string(body), final))
		if len(candidates) > MaxIconLinks {
			candidates = candidates[:MaxIconLinks]
		}
	}
	fallback := &url.URL{Scheme: page.Scheme, Host: page.Host, Path: "/favicon.ico"}
	candidates = append(candidates, iconLink{href: fallback.String(), fallback: true})
	reason := "" // why the best candidate failed: the page's, else its first linked icon's
	if pageErr != nil {
		reason = "the page did not answer (" + pageErr.Error() + ")"
	}
	failed := func(why string, candidate iconLink) {
		if reason == "" && !candidate.fallback {
			reason = why
		}
	}
	tried := map[string]bool{}
	for _, candidate := range candidates {
		if tried[candidate.href] {
			continue
		}
		tried[candidate.href] = true
		if strings.HasPrefix(candidate.href, "data:") {
			if _, _, err := ParseImage(candidate.href); err == nil {
				return candidate.href, page.String(), nil
			}
			continue
		}
		data, _, err := fetch(ctx, client, candidate.href, MaxImage)
		if err != nil {
			failed("its icon: "+err.Error(), candidate)
			continue
		}
		contentType := kindOf(data, candidate.href)
		if contentType == "" {
			failed("its icon is not a picture", candidate)
			continue
		}
		dataURL := DataURL(contentType, data)
		if _, _, err := ParseImage(dataURL); err != nil {
			failed(err.Error(), candidate)
			continue
		}
		return dataURL, candidate.href, nil
	}
	if reason == "" {
		reason = "it names no icon and has no /favicon.ico"
	}
	return "", "", fmt.Errorf("no icon found on %s: %s", site.Host, reason)
}

// fetch GETs an address as remuda, answering at most limit bytes and the address it ended at; more than limit is an error.
func fetch(ctx context.Context, client *http.Client, address string, limit int64) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, nil, errors.New("the address does not read")
	}
	req.Header.Set("User-Agent", "remuda")
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, errors.New("it could not be reached")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("it answered %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, nil, errors.New("its answer broke off")
	}
	if int64(len(data)) > limit {
		return nil, nil, fmt.Errorf("its answer is over %d KB", limit>>10)
	}
	return data, res.Request.URL, nil
}

// kindOf names the picture the bytes are, by what they sniff as; an SVG by its <svg element. "" for anything else.
func kindOf(data []byte, address string) string {
	sniffed := http.DetectContentType(data)
	for _, contentType := range imageKinds {
		if contentType != "image/svg+xml" && strings.HasPrefix(sniffed, contentType) {
			return contentType
		}
	}
	if sniffsAs(data, "image/svg+xml") || (strings.HasSuffix(strings.ToLower(address), ".svg") && strings.Contains(strings.ToLower(string(data)), "<svg")) {
		return "image/svg+xml"
	}
	return ""
}

type iconLink struct {
	href     string
	fallback bool // the site's /favicon.ico, tried last
	touch    bool // an apple-touch-icon: drawn for a home screen, so large and clear
	size     int  // the largest side the sizes attribute declares, 0 when it declares none
	svg      bool
}

var (
	linkTag   = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	attribute = regexp.MustCompile(`(?is)([a-z][a-z0-9_:-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
	sizeWord  = regexp.MustCompile(`(?i)^(\d+)x(\d+)$`)
)

// linksIn reads every <link> whose rel names an icon, its href resolved against the page.
func linksIn(page string, base *url.URL) []iconLink {
	var out []iconLink
	for _, tag := range linkTag.FindAllString(page, -1) {
		attrs := map[string]string{}
		for _, m := range attribute.FindAllStringSubmatch(tag, -1) {
			attrs[strings.ToLower(m[1])] = html.UnescapeString(m[2] + m[3] + m[4])
		}
		rel := strings.Fields(strings.ToLower(attrs["rel"]))
		touch := contains(rel, "apple-touch-icon") || contains(rel, "apple-touch-icon-precomposed")
		if contains(rel, "mask-icon") || (!touch && !contains(rel, "icon")) {
			continue
		}
		href := strings.TrimSpace(attrs["href"])
		if href == "" {
			continue
		}
		if !strings.HasPrefix(href, "data:") {
			ref, err := url.Parse(href)
			if err != nil {
				continue
			}
			resolved := base.ResolveReference(ref)
			if resolved.Scheme != "http" && resolved.Scheme != "https" {
				continue
			}
			href = resolved.String()
		}
		link := iconLink{href: href, touch: touch, size: largestSize(attrs["sizes"])}
		link.svg = strings.Contains(strings.ToLower(attrs["type"]), "svg") || strings.HasSuffix(strings.ToLower(strings.SplitN(href, "?", 2)[0]), ".svg") || strings.EqualFold(strings.TrimSpace(attrs["sizes"]), "any")
		out = append(out, link)
	}
	return out
}

func contains(words []string, want string) bool {
	for _, word := range words {
		if word == want {
			return true
		}
	}
	return false
}

func largestSize(sizes string) int {
	largest := 0
	for _, word := range strings.Fields(sizes) {
		if m := sizeWord.FindStringSubmatch(word); m != nil {
			if side, _ := strconv.Atoi(m[1]); side > largest {
				largest = side
			}
		}
	}
	return largest
}

// rankLinks orders the icons: apple-touch first, then a declared size of 32 or more (the largest first), then an SVG, then the rest.
func rankLinks(links []iconLink) []iconLink {
	tier := func(link iconLink) int {
		switch {
		case link.touch:
			return 0
		case link.size >= 32:
			return 1
		case link.svg:
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(links, func(i, j int) bool {
		if tier(links[i]) != tier(links[j]) {
			return tier(links[i]) < tier(links[j])
		}
		return links[i].size > links[j].size
	})
	return links
}
