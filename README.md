# git-digest

[![Read about the commits](https://img.shields.io/badge/commits-code%20blog-1a1a1a?style=flat-square)](https://wwel.sh/digest.html?repo=git-digest)

**Single-binary Go tool that turns your GitHub commit activity into a daily dev-journal digest — AI-written, or fully deterministic and offline with `-static`.**

It fetches your recent commits and renders a structured markdown digest: either an LLM writes a retrospective developer-journal entry from a grounded prompt, or the built-in deterministic pipeline classifies and scores the commits itself with no model in the loop. Either way, it saves the digest locally and (optionally) serves it in a clean dark-mode web UI.

---

## What it does

1. Fetches your GitHub repos filtered by recent push activity
2. Pulls your commits from the last N days (with optional file diffs)
3. Renders a structured markdown digest (summary + per-repo breakdown) — either an LLM writes it from a grounded prompt, or `-static` builds it deterministically with no model at all
4. Saves the digest locally
5. **Optionally:**
   - Serves all digests via a built-in dark-mode web UI
   - Copies the digest into a "portfolio" repo's `digests/` folder

---

## LLM backend (cloud-optional)

git-digest needs *something* to write the prose. You pick:

- **Claude Code CLI** (default) — convenient, but uses Anthropic's cloud models. Requires the [`claude`](https://github.com/anthropics/claude-code) CLI installed and authenticated.
- **Your own LLM gateway** — point it at any OpenAI `/v1/chat/completions`-compatible endpoint (local llama.cpp, Ollama proxy, self-hosted server, etc.) with `--gateway`. Fully offline if your gateway is.
- **No LLM at all** — `-static` runs the built-in deterministic template pipeline (classify commits, parse hunks, score importance, render markdown). No API, no model, no hallucinations — every line traces to a commit fact.

There's no built-in API-key path — it's the CLI, your own endpoint, or `-static`.

---

## Setup

First run with no config launches an interactive walkthrough. You can re-run it anytime:

```bash
./git-digest --settings
```

It asks, one question at a time, for your GitHub token, LLM backend, model, repos to ignore, private-repo preference, portfolio path, and output directory — then writes `~/.config/git-digest/env`. Every answer also has a plain environment-variable equivalent (below) if you'd rather skip the wizard.

---

## Usage

```
./git-digest [flags]
```

### Flags

| Flag         | Default | Description |
|--------------|---------|-------------|
| `-lookback`  | `1`     | Days to look back for activity |
| `-private`   | env     | Include private repositories |
| `-serve`     | false   | Start web UI at `http://localhost:4242` after generating |
| `-push`      | false   | Copy digest into the portfolio repo's `digests/` folder |
| `-gateway`   | false   | Use your LLM gateway instead of the Claude CLI |
| `-static`    | false   | Skip the LLM entirely, render with the deterministic static-digest pipeline |
| `-settings`  | false   | Run the interactive setup and exit |
| `-no-patches`| false   | Skip file diffs in sparse commits |
| `-no-prev`   | false   | Don't include the previous digest as a style reference |
| `-no-repeat` | false   | Exclude commit SHAs already covered by prior digests |
| `-personality` | ""    | Override the tone/voice of the digest |
| `-note`      | ""      | Manual note to fold in (e.g. private/offline work) |

### Environment variables

All are optional except `GITHUB_TOKEN`, and all can be set by the wizard.

| Variable                 | Description |
|--------------------------|-------------|
| `GITHUB_TOKEN`           | GitHub personal access token (repo scope) — **required** |
| `GIT_DIGEST_BACKEND`     | `claude`, `gateway`, or `both` (default `claude`) |
| `GIT_DIGEST_GATEWAY_URL` | Base URL of your OpenAI-compatible gateway (e.g. `http://localhost:7860`) |
| `GIT_DIGEST_MODEL`       | Model name (default `claude-haiku-4-5-20251001`) |
| `GIT_DIGEST_IGNORE`      | Comma-separated repos to skip (default none) |
| `GIT_DIGEST_PRIVATE`     | `true` to include private repos by default |
| `GIT_DIGEST_PORTFOLIO`   | Local path to a portfolio repo for `-push` |
| `GIT_DIGEST_OUTPUT`      | Where digests are saved (default `~/digests`) |

Set these in your shell, or let the wizard write them to `~/.config/git-digest/env` (loaded automatically, shell env takes precedence).

---

## Install & build

Requires Go 1.21+:

```bash
git clone https://github.com/usr-wwelsh/git-digest
cd git-digest
go build
```

Binary is created at `./git-digest`. No third-party Go dependencies.

---

## Web UI

Running with `-serve` starts an HTTP server on port 4242 with:
- Index page listing all generated digests with previews
- Individual digest view with rendered markdown
- GitHub-style dark theme, zero client-side JavaScript

---

## License

MIT — Copyright 2026 William Welsh
