package main

import "testing"

func TestDryRunRejectsServeAndPush(t *testing.T) {
	if err := validateFlags(true, true, false); err == nil {
		t.Fatal("dry-run with serve should error")
	}
	if err := validateFlags(true, false, true); err == nil {
		t.Fatal("dry-run with push should error")
	}
}

func TestDryRunAloneIsValid(t *testing.T) {
	if err := validateFlags(true, false, false); err != nil {
		t.Fatal(err)
	}
}
