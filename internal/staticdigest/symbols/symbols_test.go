package symbols

import "testing"

func TestCleanFuncName(t *testing.T) {
	tests := []struct{ ctx, want string }{
		{"func promptSystemd() error {", "promptSystemd()"},
		{"func (m *Manager) waitReady() error {", "waitReady()"},
		{"def apply_discount(price, pct):", "apply_discount()"},
		{"async function loadData() {", "loadData()"},
		{"public static void main(String[] args)", "main()"},
		{"fn parse_config(cfg: &Config) -> Result<()> {", "parse_config()"},
		{"export default function handler(req, res) {", "handler()"},
		{"", ""},
		{"if (x > 3) {", ""},
	}
	for _, tt := range tests {
		if got := CleanFuncName(tt.ctx); got != tt.want {
			t.Errorf("CleanFuncName(%q) = %q, want %q", tt.ctx, got, tt.want)
		}
	}
}

func TestImportsPython(t *testing.T) {
	got := Imports("app.py", []string{
		"import requests",
		"from pathlib import Path",
		"import requests", // dup
		"x = 1",
	})
	want := []string{"requests", "pathlib"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestImportsGo(t *testing.T) {
	got := Imports("cmd/main.go", []string{
		`"github.com/usr-wwelsh/git-digest/internal/x"`,
		`"fmt"`,
	})
	want := []string{"github.com/usr-wwelsh/git-digest/internal/x", "fmt"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestImportsJS(t *testing.T) {
	got := Imports("web/src/app.js", []string{
		"import { useState } from 'react';",
		"const fs = require('fs');",
	})
	want := []string{"react", "fs"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestImportsRust(t *testing.T) {
	got := Imports("src/lib.rs", []string{"use serde::Deserialize;"})
	if len(got) != 1 || got[0] != "serde::Deserialize" {
		t.Errorf("got %q", got)
	}
}

func TestImportsNonSourceFile(t *testing.T) {
	if got := Imports("README.md", []string{"import x from 'y'"}); got != nil {
		t.Errorf("markdown should yield no imports, got %q", got)
	}
}
