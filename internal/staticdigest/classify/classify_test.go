package classify

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		wantType  string
		wantScope string
		wantBreak bool
		wantSubj  string
	}{
		{"plain conventional", "chore: build warning fixes", "chore", "", false, "build warning fixes"},
		{"scoped", "feat(auth): require MFA", "feat", "auth", false, "require MFA"},
		{"breaking bang", "feat(api)!: drop v1 endpoints", "feat", "api", true, "drop v1 endpoints"},
		{"bare breaking", "refactor!: rename Manager to Runner", "refactor", "", true, "rename Manager to Runner"},
		{"body ignored", "fix: handle nil\n\nlonger body here", "fix", "", false, "handle nil"},
		{"fallback fix verb", "Fix systemd absolute path", "fix", "", false, "Fix systemd absolute path"},
		{"fallback add verb", "Add rate limiting to login", "feat", "", false, "Add rate limiting to login"},
		{"fallback bump verb", "bump go-github to v63", "chore", "", false, "bump go-github to v63"},
		{"merge commit", "Merge branch 'main' into dev", "other", "", false, "Merge branch 'main' into dev"},
		{"unknown prefix word", "random text here", "other", "", false, "random text here"},
		{"empty message", "", "other", "", false, ""},
		{"type not whitelisted", "wip: halfway through", "other", "", false, "halfway through"},
		{"perf type", "perf(cache): memoize lookups", "perf", "cache", false, "memoize lookups"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.message)
			if got.Type != tt.wantType || got.Scope != tt.wantScope ||
				got.Breaking != tt.wantBreak || got.Subject != tt.wantSubj {
				t.Errorf("Parse(%q) = %+v, want type=%q scope=%q breaking=%v subject=%q",
					tt.message, got, tt.wantType, tt.wantScope, tt.wantBreak, tt.wantSubj)
			}
		})
	}
}
