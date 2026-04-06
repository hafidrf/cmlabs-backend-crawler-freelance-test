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
	srcsetRe       = regexp.MustCompile(`srcset=(["'])([^"']*)(["'])`)
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
	localFix := `<style id="crawler-local-fix">html,body{overflow:auto !important;overflow-y:auto !important;height:auto !important;max-height:none !important;scroll-behavior:auto !important;} body{position:static !important;touch-action:auto !important;} *{overscroll-behavior:auto !important;}</style><script id="crawler-local-fix-script">(function(){function unlock(){try{document.documentElement.style.overflow='auto';document.documentElement.style.overflowY='auto';document.documentElement.style.height='auto';document.body.style.overflow='auto';document.body.style.overflowY='auto';document.body.style.position='static';document.body.style.height='auto';var all=document.querySelectorAll('*');for(var i=0;i<all.length;i++){var el=all[i];if(!el||!el.style)continue;var of=el.style.overflow;var ofy=el.style.overflowY;if(of==='hidden')el.style.overflow='visible';if(ofy==='hidden')el.style.overflowY='visible';}}catch(e){}}function forceWheelScroll(e){try{if(e.defaultPrevented)e.preventDefault();var dy=e.deltaY||0;var dx=e.deltaX||0;window.scrollBy(dx,dy);var se=document.scrollingElement||document.documentElement;if(se&&Math.abs(dy)>0){se.scrollTop+=dy;}}catch(err){}}unlock();window.addEventListener('load',unlock);window.addEventListener('wheel',forceWheelScroll,{passive:false,capture:true});window.addEventListener('mousewheel',forceWheelScroll,{passive:false,capture:true});window.addEventListener('DOMMouseScroll',forceWheelScroll,{passive:false,capture:true});})();</script>`

	// Resolve root-relative assets so local file opening still fetches site CSS/JS/images.
	html = replaceRootRelativeAttr(html, relativeHrefRe, "href", origin)
	html = replaceRootRelativeAttr(html, relativeSrcRe, "src", origin)
	html = replaceRootRelativeSrcset(html, origin)

	if strings.Contains(html, "<head>") {
		// Add base tag to resolve remaining relative references.
		return strings.Replace(html, "<head>", "<head><base href=\""+base+"\">"+localFix, 1)
	}
	if strings.Contains(html, "<HEAD>") {
		return strings.Replace(html, "<HEAD>", "<HEAD><base href=\""+base+"\">"+localFix, 1)
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

func replaceRootRelativeSrcset(input, origin string) string {
	return srcsetRe.ReplaceAllStringFunc(input, func(m string) string {
		parts := srcsetRe.FindStringSubmatch(m)
		if len(parts) != 4 {
			return m
		}
		quote := parts[1]
		set := parts[2]
		closeQuote := parts[3]
		if quote != closeQuote {
			return m
		}

		items := strings.Split(set, ",")
		for i, item := range items {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			pieces := strings.Fields(item)
			if len(pieces) == 0 {
				continue
			}
			if strings.HasPrefix(pieces[0], "/") && !strings.HasPrefix(pieces[0], "//") {
				pieces[0] = origin + pieces[0]
				items[i] = strings.Join(pieces, " ")
			} else {
				items[i] = item
			}
		}

		return "srcset=" + quote + strings.Join(items, ", ") + quote
	})
}
