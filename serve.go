package main

import (
	"bufio"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

const port = 4242

type digestEntry struct {
	Slug     string
	Date     string
	Lookback string
	Preview  string
	Filename string
}

var reBold = regexp.MustCompile(`\*\*(.+?)\*\*`)
var reItalic = regexp.MustCompile(`_([^_]+)_`)

var indexTmpl = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>git-digest</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0d1117;color:#e6edf3;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:15px;line-height:1.6}
a{color:#58a6ff;text-decoration:none}
header{background:#161b22;border-bottom:1px solid #30363d;padding:14px 24px;display:flex;align-items:center;gap:12px}
.logo{font-weight:700;font-size:16px;letter-spacing:-.3px}
.meta{color:#8b949e;font-size:13px;margin-left:auto}
.container{max-width:800px;margin:32px auto;padding:0 24px}
.card{border:1px solid #30363d;border-radius:6px;padding:16px 20px;margin-bottom:10px;display:block;color:inherit;transition:border-color .1s,background .1s}
.card:hover{border-color:#58a6ff;background:#161b22}
.card-head{display:flex;align-items:center;gap:10px;margin-bottom:6px}
.date{font-weight:600;font-size:15px}
.badge{background:#1f6feb22;color:#58a6ff;border:1px solid #1f6feb55;font-size:11px;font-family:monospace;padding:1px 8px;border-radius:20px}
.preview{color:#8b949e;font-size:13px}
.empty{color:#8b949e;text-align:center;padding:64px 0;font-size:14px}
</style>
</head>
<body>
<header>
  <span class="logo">git-digest</span>
  <span class="meta">{{len .}} digest(s)</span>
</header>
<div class="container">
{{if .}}{{range .}}<a class="card" href="/digest/{{.Slug}}">
  <div class="card-head">
    <span class="date">{{.Date}}</span>
    <span class="badge">{{.Lookback}}</span>
  </div>
  {{if .Preview}}<div class="preview">{{.Preview}}</div>{{end}}
</a>
{{end}}{{else}}<div class="empty">No digests found.</div>{{end}}
</div>
</body>
</html>`))

var digestTmpl = template.Must(template.New("digest").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Entry.Date}} — git-digest</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0d1117;color:#e6edf3;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:15px;line-height:1.6}
a{color:#58a6ff;text-decoration:none}
header{background:#161b22;border-bottom:1px solid #30363d;padding:14px 24px;display:flex;align-items:center;gap:12px}
.back{color:#8b949e;font-size:13px}
.back:hover{color:#58a6ff}
.logo{font-weight:700;font-size:16px;letter-spacing:-.3px}
.badge{background:#1f6feb22;color:#58a6ff;border:1px solid #1f6feb55;font-size:11px;font-family:monospace;padding:1px 8px;border-radius:20px}
.container{max-width:800px;margin:40px auto;padding:0 24px}
.content h1{font-size:22px;margin:0 0 16px;border-bottom:1px solid #30363d;padding-bottom:10px}
.content h2{font-size:17px;margin:28px 0 10px;color:#e6edf3}
.content h3{font-size:14px;margin:20px 0 6px;color:#58a6ff;font-family:monospace}
.content p{margin:6px 0;color:#c9d1d9}
.content ul{margin:6px 0 6px 20px}
.content li{margin:3px 0;color:#c9d1d9}
.content strong{color:#e6edf3;font-weight:600}
.content em{color:#8b949e;font-style:italic}
</style>
</head>
<body>
<header>
  <a class="back" href="/">← back</a>
  <span class="logo">{{.Entry.Date}}</span>
  <span class="badge">{{.Entry.Lookback}}</span>
</header>
<div class="container">
  <div class="content">{{.Content}}</div>
</div>
</body>
</html>`))

func startServer(digestsDir, mdPath string) error {
	entries, err := loadEntries(digestsDir)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		indexTmpl.Execute(w, entries)
	})

	mux.HandleFunc("/digest/", func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimPrefix(r.URL.Path, "/digest/")
		for _, e := range entries {
			if e.Slug == slug {
				raw, err := os.ReadFile(filepath.Join(digestsDir, e.Filename))
				if err != nil {
					http.Error(w, "not found", 404)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				digestTmpl.Execute(w, map[string]any{
					"Entry":   e,
					"Content": mdToHTML(string(raw)),
				})
				return
			}
		}
		http.NotFound(w, r)
	})

	srv := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		fmt.Println("\nShutting down.")
		srv.Close()
	}()

	portfolioHint := ""
	if mdPath != "" && os.Getenv("GIT_DIGEST_PORTFOLIO") != "" {
		portfolioHint = ", p+Enter to push to portfolio"
	}
	fmt.Printf("\n→ http://localhost:%d  (Ctrl+C to exit%s)\n\n", port, portfolioHint)

	if mdPath != "" && os.Getenv("GIT_DIGEST_PORTFOLIO") != "" {
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				if strings.TrimSpace(scanner.Text()) == "p" {
					if err := pushToPortfolio(mdPath); err != nil {
						fmt.Fprintf(os.Stderr, "push: %v\n", err)
					}
				}
			}
		}()
	}
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func loadEntries(dir string) ([]digestEntry, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}

	var entries []digestEntry
	for _, f := range files {
		name := filepath.Base(f)
		base := strings.TrimSuffix(name, ".md")
		parts := strings.Split(base, "-")
		if len(parts) < 4 {
			continue
		}
		date := strings.Join(parts[:3], "-")
		lookback := parts[3]
		raw, _ := os.ReadFile(f)
		entries = append(entries, digestEntry{
			Slug:     base,
			Date:     date,
			Lookback: lookback,
			Preview:  extractPreview(string(raw)),
			Filename: name,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Slug > entries[j].Slug
	})
	return entries, nil
}

func extractPreview(md string) string {
	inSummary := false
	for _, line := range strings.Split(md, "\n") {
		if line == "## Summary" {
			inSummary = true
			continue
		}
		if inSummary && strings.HasPrefix(line, "##") {
			break
		}
		line = strings.TrimSpace(line)
		if inSummary && line != "" && !strings.HasPrefix(line, "_") && !strings.HasPrefix(line, "#") {
			line = reBold.ReplaceAllString(line, "$1")
			line = reItalic.ReplaceAllString(line, "$1")
			if len(line) > 160 {
				line = line[:160] + "…"
			}
			return line
		}
	}
	return ""
}

func mdToHTML(md string) template.HTML {
	var out strings.Builder
	inList := false

	closeList := func() {
		if inList {
			out.WriteString("</ul>\n")
			inList = false
		}
	}

	for _, line := range strings.Split(md, "\n") {
		switch {
		case strings.HasPrefix(line, "### "):
			closeList()
			out.WriteString("<h3>" + inlineFormat(html.EscapeString(line[4:])) + "</h3>\n")
		case strings.HasPrefix(line, "## "):
			closeList()
			out.WriteString("<h2>" + inlineFormat(html.EscapeString(line[3:])) + "</h2>\n")
		case strings.HasPrefix(line, "# "):
			closeList()
			out.WriteString("<h1>" + inlineFormat(html.EscapeString(line[2:])) + "</h1>\n")
		case strings.HasPrefix(line, "- "):
			if !inList {
				out.WriteString("<ul>\n")
				inList = true
			}
			out.WriteString("<li>" + inlineFormat(html.EscapeString(line[2:])) + "</li>\n")
		case strings.TrimSpace(line) == "":
			closeList()
		default:
			closeList()
			out.WriteString("<p>" + inlineFormat(html.EscapeString(line)) + "</p>\n")
		}
	}
	closeList()
	return template.HTML(out.String())
}

func inlineFormat(s string) string {
	s = reBold.ReplaceAllString(s, "<strong>$1</strong>")
	s = reItalic.ReplaceAllString(s, "<em>$1</em>")
	return s
}
