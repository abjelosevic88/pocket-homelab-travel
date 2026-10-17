package geo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// PhotoCandidate is a freely licensed picture found for a place.
type PhotoCandidate struct {
	Title   string // File:… on Commons
	URL     string // 1200px thumbnail
	Credit  string
	License string
	PageURL string
	Source  string // wikipedia | commons
}

var badFile = regexp.MustCompile(`(?i)(map|flag|icon|logo|coat|wappen|seal|emblem|plan\b|diagram|chart|graph|population|statistic|sign\.|\.svg$|\.gif$|\.pdf$|\.tif)`)

// FindPhotos looks for pictures of a place: first the Wikipedia article's
// images (name + hint resolve to an article), then anything on Commons
// photographed within 250 m of the coordinates.
func (c *Client) FindPhotos(ctx context.Context, name, hint string, lat, lng *float64, max int) ([]PhotoCandidate, error) {
	if max <= 0 {
		max = 4
	}
	type pick struct {
		title, source string
	}
	var picks []pick
	seen := map[string]bool{}
	add := func(titles []string, source string) {
		for _, t := range titles {
			t = strings.ReplaceAll(t, "_", " ")
			if !seen[t] {
				seen[t] = true
				picks = append(picks, pick{t, source})
			}
		}
	}
	// The article's own pictures, but only when the article is about this
	// place and not merely the city it is in.
	if title := c.wikiTitle(ctx, name, hint); title != "" && relevant(title, name) {
		add(c.wikiMedia(ctx, title), "wikipedia")
	}
	// Top up with photos taken right there.
	if len(picks) < max && lat != nil && lng != nil {
		add(c.commonsNearby(ctx, *lat, *lng, 150), "commons")
		if len(picks) < max {
			add(c.commonsNearby(ctx, *lat, *lng, 400), "commons")
		}
	}
	if len(picks) == 0 {
		return nil, nil
	}
	if len(picks) > 12 {
		picks = picks[:12]
	}
	titles := make([]string, 0, len(picks))
	for _, p := range picks {
		titles = append(titles, p.title)
	}
	infos, err := c.commonsInfo(ctx, titles)
	if err != nil {
		return nil, err
	}
	out := []PhotoCandidate{}
	for pass := 0; pass < 2 && len(out) < max; pass++ {
		for _, p := range picks {
			pc, ok := infos[p.title]
			if !ok || pc.URL == "" {
				continue
			}
			// JPEGs first; PNGs only when nothing else turned up.
			isJPG := strings.HasSuffix(strings.ToLower(p.title), ".jpg") || strings.HasSuffix(strings.ToLower(p.title), ".jpeg")
			if (pass == 0) != isJPG {
				continue
			}
			pc.Source = p.source
			out = append(out, pc)
			if len(out) >= max {
				break
			}
		}
	}
	return out, nil
}

// relevant reports whether an article title plausibly names the place: they
// share a word of four letters or more (so "Banja Luka" does not stand in for
// "Gospodska Street").
func relevant(title, name string) bool {
	words := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ' ' || r == '(' || r == ')' || r == ',' || r == '-' || r == '\'' }) {
			if len([]rune(w)) >= 4 {
				m[w] = true
			}
		}
		return m
	}
	a, b := words(title), words(name)
	for w := range a {
		if b[w] {
			return true
		}
	}
	return false
}

// wikiTitle resolves the best English Wikipedia article for a place.
func (c *Client) wikiTitle(ctx context.Context, name, hint string) string {
	for _, q := range []string{strings.TrimSpace(name + " " + hint), name} {
		if strings.TrimSpace(q) == "" {
			continue
		}
		var sr struct {
			Query struct {
				Search []struct {
					Title string `json:"title"`
				} `json:"search"`
			} `json:"query"`
		}
		if err := c.getJSON(ctx, "https://en.wikipedia.org/w/api.php?action=query&list=search&format=json&srlimit=1&srsearch="+url.QueryEscape(q), &sr); err == nil && len(sr.Query.Search) > 0 {
			return sr.Query.Search[0].Title
		}
	}
	return ""
}

// wikiMedia lists the image files used in an article, lead image first.
func (c *Client) wikiMedia(ctx context.Context, title string) []string {
	var ml struct {
		Items []struct {
			Title     string `json:"title"`
			Type      string `json:"type"`
			LeadImage bool   `json:"leadImage"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, "https://en.wikipedia.org/api/rest_v1/page/media-list/"+url.PathEscape(strings.ReplaceAll(title, " ", "_")), &ml); err != nil {
		return nil
	}
	var lead, rest []string
	for _, it := range ml.Items {
		if it.Type != "image" || badFile.MatchString(it.Title) {
			continue
		}
		if it.LeadImage {
			lead = append(lead, it.Title)
		} else {
			rest = append(rest, it.Title)
		}
	}
	return append(lead, rest...)
}

// commonsNearby lists photos geotagged near a point.
func (c *Client) commonsNearby(ctx context.Context, lat, lng float64, radius int) []string {
	var gs struct {
		Query struct {
			Geosearch []struct {
				Title string `json:"title"`
			} `json:"geosearch"`
		} `json:"query"`
	}
	u := fmt.Sprintf("https://commons.wikimedia.org/w/api.php?action=query&format=json&list=geosearch&gsnamespace=6&gsprimary=all&gslimit=12&gsradius=%d&gscoord=%s", radius, url.QueryEscape(fmt.Sprintf("%f|%f", lat, lng)))
	if err := c.getJSON(ctx, u, &gs); err != nil {
		return nil
	}
	var out []string
	for _, g := range gs.Query.Geosearch {
		if !badFile.MatchString(g.Title) {
			out = append(out, g.Title)
		}
	}
	return out
}

// commonsInfo fetches thumbnail URLs and attribution for files.
func (c *Client) commonsInfo(ctx context.Context, titles []string) (map[string]PhotoCandidate, error) {
	var r struct {
		Query struct {
			Pages map[string]struct {
				Title     string `json:"title"`
				ImageInfo []struct {
					ThumbURL       string `json:"thumburl"`
					DescriptionURL string `json:"descriptionurl"`
					Width          int    `json:"width"`
					Height         int    `json:"height"`
					ExtMetadata    map[string]struct {
						Value string `json:"value"`
					} `json:"extmetadata"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	u := "https://commons.wikimedia.org/w/api.php?action=query&format=json&prop=imageinfo&iiprop=url|size|extmetadata&iiurlwidth=1200&iiextmetadatafilter=Artist|LicenseShortName&titles=" + url.QueryEscape(strings.Join(titles, "|"))
	if err := c.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	out := map[string]PhotoCandidate{}
	for _, p := range r.Query.Pages {
		if len(p.ImageInfo) == 0 {
			continue
		}
		ii := p.ImageInfo[0]
		if ii.Width < 500 || ii.Height < 300 {
			continue // icons and tiny scans
		}
		pc := PhotoCandidate{Title: p.Title, URL: ii.ThumbURL, PageURL: ii.DescriptionURL}
		if a, ok := ii.ExtMetadata["Artist"]; ok {
			pc.Credit = strings.TrimSpace(stripTags(a.Value))
		}
		if l, ok := ii.ExtMetadata["LicenseShortName"]; ok {
			pc.License = l.Value
		}
		out[p.Title] = pc
	}
	return out, nil
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}

// Download fetches an image (max 8 MB) and returns bytes + content type.
func (c *Client) Download(ctx context.Context, u string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", c.UA)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	return b, res.Header.Get("Content-Type"), err
}
