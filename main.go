package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest"
)

const apiBase = "https://api.github.com"

const defaultModel = "claude-haiku-4-5-20251001"

func modelName() string {
	if m := os.Getenv("GIT_DIGEST_MODEL"); m != "" {
		return m
	}
	return defaultModel
}

func ignoredRepos() []string {
	var out []string
	for _, s := range strings.Split(os.Getenv("GIT_DIGEST_IGNORE"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func gatewayURL() string { return strings.TrimRight(os.Getenv("GIT_DIGEST_GATEWAY_URL"), "/") }

const (
	sparseCommitThreshold = 5
	maxPatchLines         = 5
)

type repo struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	PushedAt    string `json:"pushed_at"`
}

type commit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
	} `json:"commit"`
}

type fileStat struct {
	Filename  string `json:"filename"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"`
}

type commitDetail struct {
	Files []fileStat `json:"files"`
}

type repoCommitSet struct {
	name        string
	description string
	commits     []commit
}

func main() {
	loadEnvFile()
	lookback := flag.Int("lookback", 1, "days to look back")
	private := flag.Bool("private", os.Getenv("GIT_DIGEST_PRIVATE") == "true", "include private repos")
	serve := flag.Bool("serve", false, "serve digests in browser after generating")
	push := flag.Bool("push", false, "copy digest into portfolio repo (GIT_DIGEST_PORTFOLIO)")
	gateway := flag.Bool("gateway", false, "use your OpenAI-compatible LLM gateway (GIT_DIGEST_GATEWAY_URL)")
	local := flag.Bool("local", false, "alias for -gateway")
	static := flag.Bool("static", false, "generate the digest with the deterministic static-digest pipeline instead of an LLM (no API, no network beyond GitHub, no hallucinations)")
	settings := flag.Bool("settings", false, "run interactive setup and exit")
	noPatches := flag.Bool("no-patches", false, "skip file diffs in sparse commits")
	noPrev := flag.Bool("no-prev", false, "skip adding previous digest to context")
	noRepeat := flag.Bool("no-repeat", false, "exclude commit SHAs already included in prior digests")
	personality := flag.String("personality", "", "override tone/voice of the digest (e.g. \"bombastic almost comical blogger but no goofy\")")
	note := flag.String("note", "", "manual note to include in digest (e.g. private/offline work not in GitHub)")
	flag.Parse()

	if *settings {
		if err := runSettings(); err != nil {
			fmt.Fprintf(os.Stderr, "settings: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if needsSetup() {
		fmt.Println("No configuration found — let's set up git-digest.")
		if err := runSettings(); err != nil {
			fmt.Fprintf(os.Stderr, "settings: %v\n", err)
			os.Exit(1)
		}
		loadEnvFile()
		fmt.Println()
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "homedir: %v\n", err)
		os.Exit(1)
	}
	digestsDir := os.Getenv("GIT_DIGEST_OUTPUT")
	if digestsDir == "" {
		digestsDir = filepath.Join(home, "digests")
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "GITHUB_TOKEN not set (run with --settings)")
		os.Exit(1)
	}

	since := time.Now().UTC().AddDate(0, 0, -*lookback)

	var seen map[string]bool
	if *noRepeat {
		seen = loadSeenSHAs(digestsDir, since)
	}

	var username string
	username, err = getUsername(token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Fetching activity for %s (last %d day(s))...\n", username, *lookback)

	repos, err := activeRepos(token, since, *private)
	if err != nil {
		fmt.Fprintf(os.Stderr, "repos: %v\n", err)
		os.Exit(1)
	}
	if len(repos) == 0 && *note == "" {
		fmt.Println("No activity found.")
		return
	}

	sets := make([]repoCommitSet, len(repos))
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Add(1)
		go func(i int, r repo) {
			defer wg.Done()
			commits, err := repoCommits(token, r.FullName, since, username)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn %s: %v\n", r.FullName, err)
				return
			}
			if seen != nil {
				var fresh []commit
				for _, c := range commits {
					if !seen[c.SHA] {
						fresh = append(fresh, c)
					}
				}
				commits = fresh
			}
			sets[i] = repoCommitSet{r.FullName, r.Description, commits}
		}(i, r)
	}
	wg.Wait()

	var active []repoCommitSet
	for _, s := range sets {
		if len(s.commits) > 0 {
			active = append(active, s)
		}
	}
	if len(active) == 0 && *note == "" {
		fmt.Println("No commits found in window.")
		return
	}

	totalCommits := 0
	for _, s := range active {
		totalCommits += len(s.commits)
	}

	fmt.Println("─────────────────────────────────")
	fmt.Printf(" Git Digest — %s\n", time.Now().Format("2006-01-02"))
	fmt.Println("─────────────────────────────────")
	fmt.Println()

	var output strings.Builder
	if *static {
		activity, err := buildStaticActivity(token, active)
		if err != nil {
			fmt.Fprintf(os.Stderr, "static: %v\n", err)
			os.Exit(1)
		}
		digest, err := staticdigest.Generate(bytes.NewReader(activity), *note)
		if err != nil {
			fmt.Fprintf(os.Stderr, "static: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(digest)
		output.WriteString(digest)
	} else if err := generateWithLLM(token, username, *lookback, active, totalCommits, *noPatches, *noPrev, *personality, *note, *gateway, *local, &output); err != nil {
		fmt.Fprintf(os.Stderr, "digest: %v\n", err)
		os.Exit(1)
	}

	mdPath, err := writeDigest(time.Now(), *lookback, output.String(), digestsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: write: %v\n", err)
	}

	if err := writeCommits(time.Now(), *lookback, active, digestsDir); err != nil {
		fmt.Fprintf(os.Stderr, "warn: write commits: %v\n", err)
	}

	if *push && mdPath != "" {
		if err := pushToPortfolio(mdPath); err != nil {
			fmt.Fprintf(os.Stderr, "push: %v\n", err)
		}
	}

	if *serve {
		if err := startServer(digestsDir, mdPath); err != nil {
			fmt.Fprintf(os.Stderr, "serve: %v\n", err)
			os.Exit(1)
		}
	} else if !*push && mdPath != "" && os.Getenv("GIT_DIGEST_PORTFOLIO") != "" {
		fmt.Print("Push to portfolio? [y/N]: ")
		var ans string
		fmt.Scanln(&ans)
		if strings.ToLower(strings.TrimSpace(ans)) == "y" {
			if err := pushToPortfolio(mdPath); err != nil {
				fmt.Fprintf(os.Stderr, "push: %v\n", err)
			}
		}
	}
}

// generateWithLLM builds the grounded prompt and streams it to the configured
// LLM backend (Claude CLI or an OpenAI-compatible gateway), writing the
// response into output.
func generateWithLLM(token, username string, lookback int, active []repoCommitSet, totalCommits int, noPatches, noPrev bool, personality, note string, gateway, local bool, output *strings.Builder) error {
	includePatches := !noPatches && totalCommits <= sparseCommitThreshold

	var prompt strings.Builder
	if !noPrev {
		if prev := latestDigest(); prev != "" {
			prompt.WriteString("Previous digest (style/tone reference only — NOT current activity, do not restate its content):\n")
			prompt.WriteString("---\n")
			prompt.WriteString(strings.TrimSpace(prev))
			prompt.WriteString("\n---\n\n")
		}
	}
	if note != "" {
		prompt.WriteString("Manual notes (private/offline work not captured in GitHub):\n")
		prompt.WriteString(note + "\n\n")
	}
	prompt.WriteString(fmt.Sprintf("GitHub commits for %s, %s (last %d day(s)):\n\n",
		username, time.Now().Format("2006-01-02"), lookback))
	for _, s := range active {
		if s.description != "" {
			prompt.WriteString(fmt.Sprintf("**%s** — %s\n", s.name, s.description))
		} else {
			prompt.WriteString(fmt.Sprintf("**%s**\n", s.name))
		}
		for _, c := range s.commits {
			msg := strings.SplitN(c.Commit.Message, "\n", 2)[0]
			prompt.WriteString(fmt.Sprintf("- %s: %s\n", c.SHA[:7], msg))
			stats, err := commitStats(token, s.name, c.SHA)
			if err != nil || len(stats) == 0 {
				continue
			}
			cap := len(stats)
			if cap > 5 {
				cap = 5
			}
			var parts []string
			for _, f := range stats[:cap] {
				parts = append(parts, fmt.Sprintf("%s +%d -%d", f.Filename, f.Additions, f.Deletions))
			}
			if len(stats) > 5 {
				parts = append(parts, fmt.Sprintf("…+%d more", len(stats)-5))
			}
			prompt.WriteString("  " + strings.Join(parts, ", ") + "\n")
			if includePatches {
				for _, f := range stats[:cap] {
					if f.Patch == "" {
						continue
					}
					lines := strings.Split(f.Patch, "\n")
					if len(lines) > maxPatchLines {
						lines = append(lines[:maxPatchLines], "…")
					}
					prompt.WriteString(fmt.Sprintf("  diff %s:\n", f.Filename))
					for _, line := range lines {
						prompt.WriteString("    " + line + "\n")
					}
				}
			}
		}
		prompt.WriteString("\n")
	}
	prompt.WriteString("Write a developer journal entry in markdown with:\n")
	prompt.WriteString("1. `## Summary` — what shipped, what you were deep in, anything notable across repos\n")
	prompt.WriteString("2. `## Per-Repo Activity` — one `### repo` subsection per repo, 1-2 sentences each\n")
	if personality != "" {
		prompt.WriteString("Tone: " + personality + ". No 'next steps', no 'you should'. Just what happened and why it matters.\n")
	} else {
		prompt.WriteString("Tone: retrospective and observational, like a code blog post. No 'next steps', no 'you should'. Just what happened and why it matters.\n")
	}
	prompt.WriteString("Ground every claim in the commit messages and diffs above. Do not invent features, filenames, or behaviors not evidenced in the data.")

	promptStr := prompt.String()
	if os.Getenv("DEBUG_PROMPT") != "" {
		fmt.Fprintf(os.Stderr, "\n=== PROMPT ===\n%s\n=== END ===\n\n", promptStr)
	}

	useGateway := gateway || local || os.Getenv("GIT_DIGEST_BACKEND") == "gateway"
	if useGateway {
		return streamGateway(promptStr, output)
	}
	return streamClaude(promptStr, output)
}

// buildStaticActivity fetches full (untruncated) file stats and patches for
// every commit and marshals them into static-digest's activity JSON schema —
// the same shape writeCommits persists, plus per-file diffs it doesn't.
func buildStaticActivity(token string, active []repoCommitSet) ([]byte, error) {
	type sdFile struct {
		Filename  string `json:"filename"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Patch     string `json:"patch"`
	}
	type sdCommit struct {
		SHA     string   `json:"sha"`
		Message string   `json:"message"`
		URL     string   `json:"url"`
		Files   []sdFile `json:"files"`
	}
	type sdRepo struct {
		Commits []sdCommit `json:"commits"`
	}

	out := make(map[string]sdRepo, len(active))
	for _, s := range active {
		var commits []sdCommit
		for _, c := range s.commits {
			msg := strings.SplitN(c.Commit.Message, "\n", 2)[0]
			stats, err := commitStats(token, s.name, c.SHA)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn %s@%s: %v\n", s.name, c.SHA[:7], err)
			}
			var files []sdFile
			for _, f := range stats {
				files = append(files, sdFile{f.Filename, f.Additions, f.Deletions, f.Patch})
			}
			commits = append(commits, sdCommit{
				SHA:     c.SHA,
				Message: msg,
				URL:     fmt.Sprintf("https://github.com/%s/commit/%s", s.name, c.SHA),
				Files:   files,
			})
		}
		out[s.name] = sdRepo{Commits: commits}
	}
	return json.Marshal(out)
}

func getUsername(token string) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	if err := ghGet(token, "/user", &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

func activeRepos(token string, since time.Time, includePrivate bool) ([]repo, error) {
	var all []repo
	for page := 1; ; page++ {
		var repos []repo
		url := fmt.Sprintf("/user/repos?per_page=100&page=%d&sort=pushed&affiliation=owner", page)
		if err := ghGet(token, url, &repos); err != nil {
			return nil, err
		}
		if len(repos) == 0 {
			break
		}
		for _, r := range repos {
			if !includePrivate && r.Private {
				continue
			}
			if isIgnored(r.FullName) {
				continue
			}
			pushed, err := time.Parse(time.RFC3339, r.PushedAt)
			if err != nil {
				continue
			}
			if pushed.After(since) {
				all = append(all, r)
			}
		}
		if len(repos) < 100 {
			break
		}
	}
	return all, nil
}

func commitStats(token, fullName, sha string) ([]fileStat, error) {
	var detail commitDetail
	if err := ghGet(token, fmt.Sprintf("/repos/%s/commits/%s", fullName, sha), &detail); err != nil {
		return nil, err
	}
	return detail.Files, nil
}

func repoCommits(token, fullName string, since time.Time, username string) ([]commit, error) {
	var commits []commit
	url := fmt.Sprintf("/repos/%s/commits?since=%s&author=%s&per_page=50",
		fullName, since.Format(time.RFC3339), username)
	if err := ghGet(token, url, &commits); err != nil {
		return nil, err
	}
	return commits, nil
}

func streamClaude(prompt string, capture *strings.Builder) error {
	cmd := exec.Command("claude", "-p", "--model", modelName())
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)
		capture.WriteString(line + "\n")
	}

	return cmd.Wait()
}

func streamGateway(prompt string, capture *strings.Builder) error {
	base := gatewayURL()
	if base == "" {
		return fmt.Errorf("GIT_DIGEST_GATEWAY_URL not set (run with --settings)")
	}
	m := os.Getenv("GIT_DIGEST_MODEL")
	if m == "" {
		m = "default"
	}

	body, _ := json.Marshal(map[string]any{
		"model":  m,
		"stream": true,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	resp, err := http.Post(
		base+"/v1/chat/completions",
		"application/json",
		strings.NewReader(string(body)),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("local API: %s", resp.Status)
	}

	type chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var c chunk
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			continue
		}
		if len(c.Choices) > 0 {
			if text := c.Choices[0].Delta.Content; text != "" {
				fmt.Print(text)
				capture.WriteString(text)
			}
		}
	}
	fmt.Println()
	return scanner.Err()
}

func writeDigest(t time.Time, lookback int, digest, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Git Digest — %s\n\n", t.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("_Lookback: %d day(s)_\n\n", lookback))
	sb.WriteString(strings.TrimSpace(digest))
	sb.WriteString("\n")

	path := filepath.Join(dir, fmt.Sprintf("%s-%dd.md", t.Format("2006-01-02"), lookback))
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return "", err
	}
	fmt.Printf("\nSaved: %s\n", path)
	return path, nil
}

func writeCommits(t time.Time, lookback int, active []repoCommitSet, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	type CommitData struct {
		SHA     string `json:"sha"`
		Message string `json:"message"`
		URL     string `json:"url"`
	}

	type RepoCommits struct {
		Commits []CommitData `json:"commits"`
	}

	data := make(map[string]RepoCommits)
	for _, set := range active {
		var commits []CommitData
		for _, c := range set.commits {
			msg := strings.SplitN(c.Commit.Message, "\n", 2)[0]
			commits = append(commits, CommitData{
				SHA:     c.SHA,
				Message: msg,
				URL:     fmt.Sprintf("https://github.com/%s/commit/%s", set.name, c.SHA),
			})
		}
		data[set.name] = RepoCommits{Commits: commits}
	}

	path := filepath.Join(dir, fmt.Sprintf("%s-%dd-commits.json", t.Format("2006-01-02"), lookback))
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return err
	}
	return nil
}

func isIgnored(fullName string) bool {
	name := fullName
	if i := strings.Index(fullName, "/"); i >= 0 {
		name = fullName[i+1:]
	}
	for _, ig := range ignoredRepos() {
		if ig == fullName || ig == name {
			return true
		}
	}
	return false
}

func latestDigest() string {
	portfolio := os.Getenv("GIT_DIGEST_PORTFOLIO")
	if portfolio == "" {
		return ""
	}
	dir := filepath.Join(portfolio, "digests")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var newest string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".md") {
			continue
		}
		if n > newest {
			newest = n
		}
	}
	if newest == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, newest))
	if err != nil {
		return ""
	}
	return string(data)
}

func loadSeenSHAs(dir string, since time.Time) map[string]bool {
	seen := make(map[string]bool)
	if p := os.Getenv("GIT_DIGEST_PORTFOLIO"); p != "" {
		dir = filepath.Join(p, "digests")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return seen
	}
	cutoff := since.AddDate(0, 0, -1)
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, "-commits.json") {
			continue
		}
		if d, err := time.Parse("2006-01-02", n[:10]); err == nil && d.Before(cutoff) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		var parsed map[string]struct {
			Commits []struct {
				SHA string `json:"sha"`
			} `json:"commits"`
		}
		if json.Unmarshal(data, &parsed) != nil {
			continue
		}
		for _, rc := range parsed {
			for _, c := range rc.Commits {
				seen[c.SHA] = true
			}
		}
	}
	return seen
}

func loadEnvFile() {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if os.Getenv(strings.TrimSpace(k)) == "" {
			os.Setenv(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
}

func ghGet(token, path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, apiBase+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GitHub API %s: %s", path, resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
