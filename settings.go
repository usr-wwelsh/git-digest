package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "git-digest", "env")
}

// needsSetup reports whether the first-run wizard should fire: no token in the
// environment and no config file on disk yet.
func needsSetup() bool {
	if os.Getenv("GITHUB_TOKEN") != "" {
		return false
	}
	_, err := os.Stat(configPath())
	return os.IsNotExist(err)
}

func readConfig() map[string]string {
	cfg := map[string]string{}
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			cfg[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return cfg
}

func writeConfig(cfg map[string]string) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("# git-digest configuration\n")
	for _, k := range keys {
		if cfg[k] == "" {
			continue
		}
		sb.WriteString(k + "=" + cfg[k] + "\n")
	}
	return os.WriteFile(path, []byte(sb.String()), 0600)
}

func runSettings() error {
	in := bufio.NewReader(os.Stdin)
	cfg := readConfig()

	fmt.Println("git-digest setup — press Enter to keep the [current] value.")
	fmt.Println()

	cfg["GITHUB_TOKEN"] = askSecret(in, "GitHub personal access token (repo scope)", cfg["GITHUB_TOKEN"])

	fmt.Println()
	fmt.Println("How should digests be written?")
	fmt.Println("  1) Claude Code CLI       (uses Anthropic's cloud models)")
	fmt.Println("  2) Your own LLM gateway  (any OpenAI /v1-compatible endpoint)")
	fmt.Println("  3) Both                  (CLI by default, gateway with --gateway)")
	switch strings.TrimSpace(ask(in, "Choose 1/2/3", backendChoice(cfg["GIT_DIGEST_BACKEND"]))) {
	case "2":
		cfg["GIT_DIGEST_BACKEND"] = "gateway"
	case "3":
		cfg["GIT_DIGEST_BACKEND"] = "both"
	default:
		cfg["GIT_DIGEST_BACKEND"] = "claude"
	}

	if cfg["GIT_DIGEST_BACKEND"] != "claude" {
		cfg["GIT_DIGEST_GATEWAY_URL"] = ask(in, "LLM gateway base URL (e.g. http://localhost:7860)", cfg["GIT_DIGEST_GATEWAY_URL"])
	}
	cfg["GIT_DIGEST_MODEL"] = ask(in, "Model name (blank = "+defaultModel+")", cfg["GIT_DIGEST_MODEL"])

	fmt.Println()
	cfg["GIT_DIGEST_IGNORE"] = ask(in, "Repos to skip, comma-separated (blank = none)", cfg["GIT_DIGEST_IGNORE"])
	cfg["GIT_DIGEST_PRIVATE"] = boolStr(askYN(in, "Include private repos by default?", cfg["GIT_DIGEST_PRIVATE"] == "true"))

	fmt.Println()
	cfg["GIT_DIGEST_PORTFOLIO"] = ask(in, "Portfolio repo path to copy digests into (blank = skip)", cfg["GIT_DIGEST_PORTFOLIO"])
	cfg["GIT_DIGEST_OUTPUT"] = ask(in, "Directory to save digests (blank = ~/digests)", cfg["GIT_DIGEST_OUTPUT"])

	if err := writeConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("\nSaved %s\n", configPath())
	return nil
}

func ask(in *bufio.Reader, q, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", q, def)
	} else {
		fmt.Printf("%s: ", q)
	}
	line, _ := in.ReadString('\n')
	if line = strings.TrimSpace(line); line != "" {
		return line
	}
	return def
}

// askSecret is ask() for credentials: it never echoes the stored value, only a
// masked hint. (Input typing isn't hidden — that would need a non-stdlib dep.)
func askSecret(in *bufio.Reader, q, def string) string {
	if def != "" {
		hint := def
		if len(hint) > 4 {
			hint = "…" + hint[len(hint)-4:]
		}
		fmt.Printf("%s [keep existing %s]: ", q, hint)
	} else {
		fmt.Printf("%s: ", q)
	}
	line, _ := in.ReadString('\n')
	if line = strings.TrimSpace(line); line != "" {
		return line
	}
	return def
}

func askYN(in *bufio.Reader, q string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	fmt.Printf("%s [%s]: ", q, hint)
	line, _ := in.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

func backendChoice(b string) string {
	switch b {
	case "gateway":
		return "2"
	case "both":
		return "3"
	default:
		return "1"
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

