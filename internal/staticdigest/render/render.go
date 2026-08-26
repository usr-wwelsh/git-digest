// Package render stitches extracted facts into digest markdown with plain
// templates. Every phrase traces back to an input fact — the renderer cannot
// invent filenames, functions, versions, or behaviors.
package render

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
)

const maxMentioned = 3

var verbPast = map[string]string{
	"feat":     "Added",
	"fix":      "Fixed",
	"refactor": "Refactored",
	"perf":     "Optimized",
	"docs":     "Documented",
	"test":     "Tested",
	"chore":    "Tidied",
	"build":    "Updated",
	"ci":       "Updated",
	"style":    "Polished",
	"revert":   "Reverted",
	"other":    "Worked on",
}

// imperativePast maps common commit-message imperative verbs to past tense.
// A leading verb is stripped outright (the template verb carries it); verbs
// heading later comma-separated segments are conjugated so multi-action
// subjects stay grammatical under one template verb.
var imperativePast = map[string]string{
	"add": "added", "allow": "allowed", "avoid": "avoided",
	"backport": "backported", "bump": "bumped", "cache": "cached",
	"cap": "capped", "catch": "caught", "clean": "cleaned",
	"close": "closed", "delete": "deleted", "document": "documented",
	"drop": "dropped", "dampen": "dampened", "dedupe": "deduped",
	"enable": "enabled", "escape": "escaped", "expose": "exposed",
	"extract": "extracted", "extend": "extended", "fix": "fixed",
	"gate": "gated", "guard": "guarded", "handle": "handled",
	"harden": "hardened", "hoist": "hoisted", "ignore": "ignored",
	"improve": "improved", "implement": "implemented", "inline": "inlined",
	"introduce": "introduced", "limit": "limited", "log": "logged",
	"make": "made", "merge": "merged", "migrate": "migrated",
	"memoize": "memoized", "move": "moved", "normalize": "normalized",
	"optimize": "optimized", "pin": "pinned", "polish": "polished",
	"prevent": "prevented", "recognize": "recognized", "remove": "removed",
	"rename": "renamed", "replace": "replaced", "repair": "repaired",
	"resolve": "resolved", "restrict": "restricted", "restructure": "restructured",
	"retire": "retired", "retry": "retried", "revert": "reverted",
	"rework": "reworked", "sanitize": "sanitized", "set": "set",
	"ship": "shipped", "show": "showed", "simplify": "simplified",
	"skip": "skipped", "split": "split", "stream": "streamed",
	"strip": "stripped", "support": "supported", "switch": "switched",
	"test": "tested", "tidy": "tidied", "truncate": "truncated",
	"tune": "tuned", "unify": "unified", "update": "updated",
	"upgrade": "upgraded", "use": "used", "validate": "validated",
	"wire": "wired",
}

var pastForm = func() map[string]bool {
	m := make(map[string]bool, len(imperativePast))
	for _, p := range imperativePast {
		m[p] = true
	}
	return m
}()

// riskTails attaches a grounded consequence to commits whose diffs touch
// sensitive ground: the flag was detected in the patch, the tail just names
// what such a change means.
var riskTails = map[string]string{
	"possible secret in added lines": "review for leaked credentials",
	"destructive schema operation":   "contains destructive schema ops",
	"security-sensitive paths":       "hardening security-sensitive paths",
	"schema migration":               "shipping the schema migration",
	"public API surface":             "changing the public API surface",
	"pipeline config":                "CI now exercises this path",
}

var tailPriority = []string{
	"possible secret in added lines",
	"destructive schema operation",
	"security-sensitive paths",
	"schema migration",
	"public API surface",
	"pipeline config",
}

var dayPhrases = map[string][]string{
	"fix":      {"Stabilization was the theme of the day.", "A hardening-and-fixes kind of day."},
	"feat":     {"Feature shipping defined the day.", "A build-something-new kind of day."},
	"docs":     {"Wrap-up and documentation closed the day.", "A documentation-heavy day."},
	"perf":     {"Performance work carried the day."},
	"chore":    {"Housekeeping filled the day.", "Cleanup and upkeep were the story."},
	"refactor": {"Restructuring filled the day.", "A cleanup-and-restructure kind of day."},
}

// Digest renders repo facts into the markdown shape git-digest publishes:
// "## Summary" plus one "### repo" section per repository. note, when set,
// is a manual note (e.g. offline work not captured in GitHub) reproduced
// verbatim as the Summary's opening sentence — grounded facts never rewrite
// or paraphrase it.
func Digest(repos []facts.RepoFacts, note string) string {
	var total int
	for _, r := range repos {
		total += len(r.Commits)
	}
	note = strings.TrimSpace(note)
	if total == 0 && note == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Summary\n\n")
	if note != "" {
		sb.WriteString(note)
		if last := note[len(note)-1]; last != '.' && last != '!' && last != '?' {
			sb.WriteString(".")
		}
		if total > 0 {
			sb.WriteString(" ")
		}
	}
	if total == 0 {
		sb.WriteString("\n\n")
		return sb.String()
	}

	multi := len(repos) > 1
	if multi {
		if d := dominantRepo(repos); d != nil {
			fmt.Fprintf(&sb, "%s led the day: %s.", repoBaseName(d.repo.Name), strings.TrimSpace(leadClause(d)))
			sb.WriteString(" ")
		}
	}
	sb.WriteString(summaryLine(total, len(repos)))
	if nr := genesisRepoNames(repos); len(nr) == 1 {
		sb.WriteString(fmt.Sprintf(" %s launched as a new repo.", nr[0]))
	} else if len(nr) > 1 {
		sb.WriteString(fmt.Sprintf(" New repos launched: %s.", strings.Join(nr, ", ")))
	}
	if b := breakingRepos(repos); len(b) > 0 {
		sb.WriteString(" Breaking changes flagged in ")
		sb.WriteString(strings.Join(b, ", "))
		sb.WriteString(".")
	}
	if !multi {
		if best := biggestChange(repos); best != nil {
			sb.WriteString(fmt.Sprintf(" Biggest change in %s: %s.",
				repoBaseName(best.repo.Name), strings.ToLower(firstWord(best.commit.Type, verbPast))+clauseBody(best.commit)))
		}
	} else if closer := dayCharacterization(repos, variationSeed(repos, total)); closer != "" {
		sb.WriteString(" " + closer)
	}
	sb.WriteString("\n\n")

	sb.WriteString("## Per-Repo Activity\n")
	for _, r := range repos {
		sb.WriteString("\n### " + repoBaseName(r.Name) + "\n\n")
		sb.WriteString(repoParagraph(r))
		sb.WriteString("\n")
	}
	return sb.String()
}

// repoBaseName strips a "owner/repo" full name down to just "repo" for
// display in prose and headers — the owner segment is redundant once every
// digest is scoped to a single GitHub account, and full names read as noise.
func repoBaseName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

func summaryLine(total, nrepos int) string {
	return fmt.Sprintf("%s across %s.", plural(total, "commit"), plural(nrepos, "repo"))
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

type scored struct {
	repo   facts.RepoFacts
	commit facts.CommitFacts
	sum    float64
}

func biggestChange(repos []facts.RepoFacts) *scored {
	var best *scored
	for _, r := range repos {
		for _, c := range withoutGenesis(r.Commits) {
			if best == nil || c.Score > best.commit.Score {
				bc := c
				br := r
				best = &scored{repo: br, commit: bc}
			}
		}
	}
	return best
}

// dominantRepo picks the repo with the highest total non-genesis score and
// remembers its top commit for the summary lead sentence.
func dominantRepo(repos []facts.RepoFacts) *scored {
	var best *scored
	for _, r := range repos {
		commits := withoutGenesis(r.Commits)
		if len(commits) == 0 {
			continue
		}
		var sum float64
		top := commits[0]
		for _, c := range commits {
			sum += c.Score
			if c.Score > top.Score {
				top = c
			}
		}
		if best == nil || sum > best.sum {
			best = &scored{repo: r, commit: top, sum: sum}
		}
	}
	return best
}

// leadClause renders the dominant repo's top-scoring unit for the summary.
func leadClause(d *scored) string {
	units := buildUnits(sortedByScore(withoutGenesis(d.repo.Commits)))
	if len(units) == 0 {
		return clauseBody(d.commit)
	}
	return unitClause(units[0])
}

// variationSeed derives a stable pseudo-random seed from the digest's own
// content, so phrase selection varies across days but is deterministic for a
// given day.
func variationSeed(repos []facts.RepoFacts, total int) uint32 {
	h := fnv.New32a()
	for _, r := range repos {
		h.Write([]byte(r.Name))
	}
	h.Write([]byte(strconv.Itoa(total)))
	return h.Sum32()
}

func pickVariant(pool []string, seed uint32) string {
	if len(pool) == 0 {
		return ""
	}
	return pool[int(seed%uint32(len(pool)))]
}

// dayCharacterization summarizes a multi-repo day's dominant commit type in
// one grounded sentence. Single-type weighting is by commit count; days with
// no recognizable dominant type get no closer.
func dayCharacterization(repos []facts.RepoFacts, seed uint32) string {
	count := map[string]int{}
	for _, r := range repos {
		for _, c := range withoutGenesis(r.Commits) {
			count[c.Type]++
		}
	}
	keys := make([]string, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	dominant := ""
	for _, k := range keys {
		if count[k] > count[dominant] {
			dominant = k
		}
	}
	return pickVariant(dayPhrases[dominant], seed)
}

// genesisCommit returns a repo's "init" commit, if it shipped one this cycle.
func genesisCommit(commits []facts.CommitFacts) *facts.CommitFacts {
	for i := range commits {
		if commits[i].Genesis {
			return &commits[i]
		}
	}
	return nil
}

func withoutGenesis(commits []facts.CommitFacts) []facts.CommitFacts {
	out := make([]facts.CommitFacts, 0, len(commits))
	for _, c := range commits {
		if !c.Genesis {
			out = append(out, c)
		}
	}
	return out
}

func genesisRepoNames(repos []facts.RepoFacts) []string {
	var out []string
	for _, r := range repos {
		if genesisCommit(r.Commits) != nil {
			out = append(out, repoBaseName(r.Name))
		}
	}
	return out
}

func breakingRepos(repos []facts.RepoFacts) []string {
	var out []string
	for _, r := range repos {
		for _, c := range r.Commits {
			if c.Breaking {
				out = append(out, repoBaseName(r.Name))
				break
			}
		}
	}
	return out
}

func sortedByScore(commits []facts.CommitFacts) []facts.CommitFacts {
	out := make([]facts.CommitFacts, len(commits))
	copy(out, commits)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// unit is one rendered clause: a lone commit or a pair of related commits
// merged into a single thematic mention. Units keep score order — the first
// commit is always the unit's highest-scored.
type unit struct {
	commits []facts.CommitFacts
}

func (u unit) notable() bool {
	for _, c := range u.commits {
		if c.Notable {
			return true
		}
	}
	return false
}

// buildUnits folds adjacent same-type commits sharing an area (or scope)
// into two-commit units, so a burst of related fixes reads as one clause
// instead of a semicolon list.
func buildUnits(sorted []facts.CommitFacts) []unit {
	units := make([]unit, 0, len(sorted))
	for _, c := range sorted {
		if !c.Notable && len(units) > 0 {
			last := &units[len(units)-1]
			prev := last.commits[len(last.commits)-1]
			if !prev.Notable && len(last.commits) < 2 && related(prev, c) {
				last.commits = append(last.commits, c)
				continue
			}
		}
		units = append(units, unit{commits: []facts.CommitFacts{c}})
	}
	return units
}

// related reports whether two commits plausibly belong to the same change:
// identical non-generic type plus a shared area. Scope alone doesn't merge —
// a repo-wide scope sweep isn't one story.
func related(a, b facts.CommitFacts) bool {
	if a.Type != b.Type || a.Type == "other" {
		return false
	}
	set := make(map[string]bool, len(a.Areas))
	for _, ar := range commitAreas(a) {
		set[ar] = true
	}
	for _, ar := range commitAreas(b) {
		if set[ar] {
			return true
		}
	}
	return false
}

// commitAreas uses the precomputed Areas when present, deriving from file
// paths otherwise, so merging never depends on upstream having filled them.
func commitAreas(c facts.CommitFacts) []string {
	if len(c.Areas) > 0 {
		return c.Areas
	}
	var out []string
	for _, f := range c.Files {
		if a := facts.AreaOf(f.Path); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func countCommits(units []unit) int {
	n := 0
	for _, u := range units {
		n += len(u.commits)
	}
	return n
}

// dominantScope returns the conventional-commit scope covering at least half
// the repo's commits (minimum two), for topic-led paragraphs.
func dominantScope(units []unit) string {
	count := map[string]int{}
	total := 0
	for _, u := range units {
		for _, c := range u.commits {
			if c.Scope != "" {
				count[c.Scope]++
			}
			total++
		}
	}
	best := ""
	for scope, n := range count {
		if n >= 2 && n*2 >= total && n > count[best] {
			best = scope
		}
	}
	return best
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// selectMentionedUnits picks the units to mention in a crowded repo
// paragraph. Notable commits (postmortems, incident reports) are guaranteed
// a slot even when their score would otherwise be crowded out — a small diff
// can still be the most important story of the day. Remaining slots fill by
// score, and the result is re-sorted by score for consistent reading order.
func selectMentionedUnits(units []unit, n int) []unit {
	if len(units) <= n {
		return units
	}
	picked := make([]bool, len(units))
	out := make([]unit, 0, n)
	for i, u := range units {
		if u.notable() && len(out) < n {
			out = append(out, u)
			picked[i] = true
		}
	}
	for i, u := range units {
		if len(out) >= n {
			break
		}
		if !picked[i] {
			out = append(out, u)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].commits[0].Score > out[j].commits[0].Score })
	return out
}

func repoParagraph(r facts.RepoFacts) string {
	var sb strings.Builder
	commits := r.Commits

	if g := genesisCommit(commits); g != nil {
		fmt.Fprintf(&sb, "New repo: bootstrapped with an init commit (%s).", plural(len(g.Files), "file"))
		commits = withoutGenesis(commits)
		if len(commits) == 0 {
			return sb.String()
		}
		sb.WriteString(" ")
	}

	units := buildUnits(sortedByScore(commits))

	if len(units) > maxMentioned {
		if scope := dominantScope(units); scope != "" {
			sb.WriteString(capFirst(scope) + ": ")
		} else {
			fmt.Fprintf(&sb, "Shipped %d commits: ", countCommits(units))
		}
		mentioned := selectMentionedUnits(units, maxMentioned)
		clauses := make([]string, 0, len(mentioned))
		for _, u := range mentioned {
			clauses = append(clauses, strings.ToLower(firstWord(u.commits[0].Type, verbPast))+unitClause(u))
		}
		sb.WriteString(strings.Join(clauses, "; "))
		sb.WriteString(".")
	} else {
		sentences := make([]string, 0, len(units))
		for _, u := range units {
			sentences = append(sentences, firstWord(u.commits[0].Type, verbPast)+unitClause(u)+".")
		}
		sb.WriteString(strings.Join(sentences, " "))
	}

	if deps := depSentence(r.Commits); deps != "" {
		sb.WriteString(" " + deps)
	}
	if w := watchSentence(r.Commits); w != "" {
		sb.WriteString(" " + w)
	}
	return sb.String()
}

// firstWord returns the past-tense verb for a commit type.
func firstWord(typ string, verbs map[string]string) string {
	if v, ok := verbs[typ]; ok && v != "" {
		return v
	}
	return verbs["other"]
}

func clauseBody(c facts.CommitFacts) string {
	body := subjectBody(c)
	ev := evidence([]facts.CommitFacts{c})
	tail := riskTail(c.RiskFlags)
	if ev != "" {
		body += " " + ev
	}
	if tail != "" {
		body += " — " + tail
	}
	return " " + body
}

// unitClause renders one unit: template verb, each commit's cleaned subject
// joined with "and", shared evidence, and at most one consequence tail.
func unitClause(u unit) string {
	parts := make([]string, 0, len(u.commits))
	var flags []string
	for _, c := range u.commits {
		if body := subjectBody(c); body != "" {
			parts = append(parts, body)
		}
		flags = append(flags, c.RiskFlags...)
	}
	body := strings.Join(parts, " and ")
	if body == "" {
		body = strings.ToLower(strings.TrimSpace(u.commits[0].Subject))
	}
	if ev := evidence(u.commits); ev != "" {
		body += " " + ev
	}
	if tail := riskTail(flags); tail != "" {
		body += " — " + tail
	}
	return " " + body
}

// subjectBody cleans a commit subject into clause text: the leading verb is
// stripped (the template verb carries it), and comma-separated segments get
// their own verbs conjugated to past tense.
func subjectBody(c facts.CommitFacts) string {
	subj := strings.TrimSpace(c.Subject)
	if subj == "" {
		return ""
	}
	segs := strings.Split(subj, ", ")
	out := cleanSubject(segs[0])
	for _, seg := range segs[1:] {
		out += ", " + conjugateSegment(seg)
	}
	return out
}

// conjugateSegment rewrites a comma-separated subject segment so its verb
// reads as past tense ("dampen data-dump churn" -> "dampened data-dump churn").
func conjugateSegment(seg string) string {
	words := strings.Fields(strings.TrimSpace(seg))
	if len(words) == 0 {
		return seg
	}
	first := strings.ToLower(words[0])
	if past, ok := imperativePast[first]; ok {
		words[0] = past
	} else if !pastForm[first] && !acronymLike(words[0]) {
		head := []rune(words[0])
		head[0] = unicode.ToLower(head[0])
		words[0] = string(head)
	}
	return strings.Join(words, " ")
}

var acronymLike = func(w string) bool {
	r := []rune(w)
	return len(r) > 1 && unicode.IsUpper(r[1])
}

// cleanSubject strips a subject's leading verb when it's a recognized
// imperative (the template verb carries the tense), then lowercases the
// first rune unless it looks like an acronym (JWTAuth).
func cleanSubject(subj string) string {
	words := strings.Fields(strings.TrimSpace(subj))
	if len(words) == 0 {
		return ""
	}
	first := strings.ToLower(words[0])
	stripped := words
	if _, imp := imperativePast[first]; imp || pastForm[first] {
		stripped = words[1:]
	}
	if len(stripped) == 0 {
		return ""
	}
	if !acronymLike(stripped[0]) {
		head := []rune(stripped[0])
		head[0] = unicode.ToLower(head[0])
		stripped[0] = string(head)
	}
	return strings.Join(stripped, " ")
}

var (
	goFlagRe  = regexp.MustCompile(`flag\.\w+(?:Var)?\("([^"]+)"`)
	longOptRe = regexp.MustCompile(`(?:^|[\s"'=(])--([a-z][a-z0-9][a-z0-9-]*)\b`)
	routeRe   = regexp.MustCompile(`["'` + "`" + `](/[a-z0-9][\w./:-]{1,48})["'` + "`" + `]`)
)

// evidence cites the user-facing surface of a change: CLI flags and route
// paths mined from added lines. Function names and directory areas are
// deliberately not cited — parenthesized identifiers read as robotic.
func evidence(commits []facts.CommitFacts) string {
	items := minedTokens(commits)
	if len(items) == 0 {
		return ""
	}
	return "(" + strings.Join(items, ", ") + ")"
}

// minedTokens extracts flag names and route paths from added lines — the
// user-facing surface of a change, which reads better than internal symbols.
func minedTokens(commits []facts.CommitFacts) []string {
	seen := map[string]bool{}
	var subjects []string
	for _, c := range commits {
		subjects = append(subjects, strings.ToLower(c.Subject))
	}
	var out []string
	add := func(tok string) {
		if tok == "" || seen[tok] || len(tok) < 2 || strings.Contains(tok, " ") {
			return
		}
		for _, s := range subjects {
			if strings.Contains(s, strings.ToLower(tok)) {
				return // already named in the subject — don't cite twice
			}
		}
		seen[tok] = true
		out = append(out, tok)
	}
	for _, c := range commits {
		for _, f := range c.Files {
			for _, line := range f.AddedLines {
				for _, m := range goFlagRe.FindAllStringSubmatch(line, -1) {
					add("-" + m[1])
				}
				for _, m := range longOptRe.FindAllStringSubmatch(line, -1) {
					add("--" + m[1])
				}
				for _, m := range routeRe.FindAllStringSubmatch(line, -1) {
					add(m[1])
				}
			}
		}
	}
	if len(out) > 2 {
		out = out[:2]
	}
	return out
}

// riskTail returns the highest-priority consequence phrase for a set of risk
// flags, or "" when nothing was flagged.
func riskTail(flags []string) string {
	set := make(map[string]bool, len(flags))
	for _, f := range flags {
		set[f] = true
	}
	for _, f := range tailPriority {
		if set[f] {
			return riskTails[f]
		}
	}
	return ""
}

func depSentence(commits []facts.CommitFacts) string {
	var parts []string
	seen := map[string]bool{}
	for _, c := range commits {
		for _, d := range c.Deps {
			key := d.Ecosystem + "/" + d.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			switch d.Action {
			case "bumped":
				p := fmt.Sprintf("bumped %s %s -> %s", d.Name, d.Old, d.New)
				if d.Major {
					p += " (major)"
				}
				parts = append(parts, p)
			case "added":
				parts = append(parts, fmt.Sprintf("added %s %s", d.Name, d.New))
			case "removed":
				parts = append(parts, "removed "+d.Name)
			case "downgraded":
				parts = append(parts, fmt.Sprintf("downgraded %s %s -> %s", d.Name, d.Old, d.New))
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > 3 {
		parts = append(parts[:3], fmt.Sprintf("+%d more", len(parts)-3))
	}
	return "Dependency updates: " + strings.Join(parts, ", ") + "."
}

func watchSentence(commits []facts.CommitFacts) string {
	flagSet := map[string]bool{}
	breaking := false
	for _, c := range commits {
		for _, f := range c.RiskFlags {
			flagSet[f] = true
		}
		breaking = breaking || c.Breaking
	}
	if len(flagSet) == 0 && !breaking {
		return ""
	}
	var flags []string
	for f := range flagSet {
		flags = append(flags, f)
	}
	sort.Strings(flags)
	s := "Watch:"
	if len(flags) > 0 {
		s += " " + strings.Join(flags, ", ") + ";"
	}
	if breaking {
		s += " includes breaking changes."
	} else {
		s = strings.TrimSuffix(s, ";") + "."
	}
	return s
}
