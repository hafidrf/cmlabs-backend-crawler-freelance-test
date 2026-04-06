package output

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"cmlabs-backend-crawler-freelance-test/internal/crawler"
)

type Writer struct {
	outDir string
}

func NewWriter(outDir string) *Writer {
	return &Writer{outDir: outDir}
}

func (w *Writer) Write(results []crawler.Result) error {
	if err := os.MkdirAll(w.outDir, 0o755); err != nil {
		return err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].URL < results[j].URL
	})

	manifest := make([]manifestEntry, 0, len(results))
	for _, r := range results {
		fileName := fileNameFor(r)
		if r.HTML != "" {
			html := normalizeHTMLForLocalOpen(r.URL, r.HTML)
			if err := os.WriteFile(filepath.Join(w.outDir, fileName), []byte(html), 0o644); err != nil {
				return err
			}
		}

		manifest = append(manifest, manifestEntry{
			URL:          r.URL,
			Host:         r.Host,
			Title:        r.Title,
			Status:       r.Status,
			Error:        r.Error,
			DurationMS:   r.Duration.Milliseconds(),
			CrawledAtUTC: r.CrawledAtUTC.Format(time.RFC3339),
			HTMLFile:     fileName,
		})
	}

	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(w.outDir, "manifest.json"), b, 0o644)
}

type manifestEntry struct {
	URL          string `json:"url"`
	Host         string `json:"host"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	DurationMS   int64  `json:"duration_ms"`
	CrawledAtUTC string `json:"crawled_at_utc"`
	HTMLFile     string `json:"html_file"`
}

func fileNameFor(r crawler.Result) string {
	stamp := r.CrawledAtUTC.Format("20060102T150405Z")
	base := sanitize(fmt.Sprintf("%s-%s", r.Host, stamp))
	if base == "" {
		base = "unknown"
	}
	return base + ".html"
}

func sanitize(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	var b strings.Builder
	for _, ch := range v {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
			continue
		}
		if ch == '-' || ch == '_' || ch == '.' {
			b.WriteRune(ch)
			continue
		}
		b.WriteRune('-')
	}
	return strings.Trim(b.String(), "-")
}

var (
	relativeHrefRe = regexp.MustCompile(`href=(["'])(/[^"']*)(["'])`)
	relativeSrcRe  = regexp.MustCompile(`src=(["'])(/[^"']*)(["'])`)
)

// normalizeHTMLForLocalOpen makes output HTML more faithful when opened via file://.
func normalizeHTMLForLocalOpen(rawURL, html string) string {
	if strings.TrimSpace(html) == "" {
		return html
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return html
	}

	origin := u.Scheme + "://" + u.Host
	base := origin + "/"

	// Resolve root-relative assets so local file opening still fetches site CSS/JS/images.
	html = replaceRootRelativeAttr(html, relativeHrefRe, "href", origin)
	html = replaceRootRelativeAttr(html, relativeSrcRe, "src", origin)

	if strings.Contains(html, "<head>") {
		// Add base tag to resolve remaining relative references.
		return strings.Replace(html, "<head>", "<head><base href=\""+base+"\">", 1)
	}
	if strings.Contains(html, "<HEAD>") {
		return strings.Replace(html, "<HEAD>", "<HEAD><base href=\""+base+"\">", 1)
	}
	return html
}

func replaceRootRelativeAttr(input string, re *regexp.Regexp, attr, origin string) string {
	return re.ReplaceAllStringFunc(input, func(m string) string {
		parts := re.FindStringSubmatch(m)
		if len(parts) != 4 {
			return m
		}
		quote := parts[1]
		path := parts[2]
		closeQuote := parts[3]
		if quote != closeQuote {
			return m
		}
		if strings.HasPrefix(path, "//") {
			return m
		}
		return attr + "=" + quote + origin + path + quote
	})
}
