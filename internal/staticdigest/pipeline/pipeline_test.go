package pipeline

import (
	"strings"
	"testing"
)

const activityJSON = `{
  "usr-wwelsh/turbolab": {
    "commits": [
      {
        "sha": "d49b6c9abc1234",
        "message": "fix(systemd): absolute unit path",
        "url": "https://github.com/usr-wwelsh/turbolab/commit/d49b6c9",
        "files": [
          {
            "filename": "cmd/setup.go",
            "additions": 38,
            "deletions": 6,
            "patch": "@@ -198,7 +198,9 @@ func promptSystemd() error {\n-\tself := \"systemd\"\n+\tself, err := exec.LookPath(\"systemd\")\n+\tif err != nil {\n+\t\treturn err\n"
          },
          {
            "filename": "go.mod",
            "additions": 1,
            "deletions": 1,
            "patch": "@@ -5,3 +5,3 @@\n-\tgithub.com/google/go-github v62.0.0\n+\tgithub.com/google/go-github v63.0.0\n"
          }
        ]
      }
    ]
  }
}`

func TestBuildExtractsAllFactKinds(t *testing.T) {
	repos, err := Build(strings.NewReader(activityJSON))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "usr-wwelsh/turbolab" {
		t.Fatalf("repos = %+v", repos)
	}
	c := repos[0].Commits[0]

	if c.SHA != "d49b6c9abc1234" || c.Type != "fix" || c.Scope != "systemd" ||
		c.Subject != "absolute unit path" {
		t.Errorf("classify wrong: %+v", c)
	}
	if len(c.Files) != 2 {
		t.Fatalf("files = %+v", c.Files)
	}
	f := c.Files[0]
	if f.Path != "cmd/setup.go" || f.Additions != 38 || f.Deletions != 6 {
		t.Errorf("file stats wrong: %+v", f)
	}
	if len(f.FuncContexts) == 0 || f.FuncContexts[0] != "func promptSystemd() error {" {
		t.Errorf("func contexts wrong: %q", f.FuncContexts)
	}
	if len(c.Deps) != 1 || c.Deps[0].Name != "github.com/google/go-github" ||
		c.Deps[0].Action != "bumped" {
		t.Errorf("deps wrong: %+v", c.Deps)
	}
	if c.Areas == nil || len(c.Areas) == 0 || c.Areas[0] != "cmd" {
		t.Errorf("areas wrong: %q", c.Areas)
	}
	if c.Score <= 0 {
		t.Errorf("score not applied: %v", c.Score)
	}
}

func TestBuildToleratesEmptyPatch(t *testing.T) {
	in := `{"r/x":{"commits":[{"sha":"a","message":"docs: readme","url":"","files":[{"filename":"README.md","additions":2,"deletions":0,"patch":""}]}]}}`
	repos, err := Build(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	c := repos[0].Commits[0]
	if c.Type != "docs" || len(c.Files) != 1 || c.Files[0].FuncContexts != nil {
		t.Errorf("got %+v", c)
	}
}

func TestBuildFlagsInitCommitAsGenesis(t *testing.T) {
	in := `{"r/new":{"commits":[{"sha":"a","message":"init","url":"","files":[{"filename":"README.md","additions":5,"deletions":0,"patch":""}]}]}}`
	repos, err := Build(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !repos[0].Commits[0].Genesis {
		t.Errorf("commit with subject %q should be flagged Genesis", repos[0].Commits[0].Subject)
	}
}

func TestBuildDoesNotFlagOrdinaryCommitAsGenesis(t *testing.T) {
	repos, err := Build(strings.NewReader(activityJSON))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if repos[0].Commits[0].Genesis {
		t.Errorf("ordinary commit should not be flagged Genesis")
	}
}

func TestBuildFlagsPostmortemAsNotable(t *testing.T) {
	in := `{"r/x":{"commits":[{"sha":"a","message":"docs: add postmortem, retire finetuning route from release path","url":"","files":[{"filename":"README.md","additions":30,"deletions":0,"patch":""}]}]}}`
	repos, err := Build(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !repos[0].Commits[0].Notable {
		t.Errorf("postmortem commit should be flagged Notable")
	}
}

func TestBuildDoesNotFlagOrdinaryDocsAsNotable(t *testing.T) {
	in := `{"r/x":{"commits":[{"sha":"a","message":"docs: update README","url":"","files":[{"filename":"README.md","additions":5,"deletions":0,"patch":""}]}]}}`
	repos, err := Build(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if repos[0].Commits[0].Notable {
		t.Errorf("ordinary docs commit should not be flagged Notable")
	}
}

func TestBuildEmptyInput(t *testing.T) {
	repos, err := Build(strings.NewReader("{}"))
	if err != nil || len(repos) != 0 {
		t.Errorf("got %v, %v", repos, err)
	}
}
