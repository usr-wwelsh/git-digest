package main

import "testing"

func TestServeAddrBindsLoopbackByDefault(t *testing.T) {
	if got := serveAddr(false); got != "127.0.0.1:4242" {
		t.Fatalf("got %q", got)
	}
}

func TestServeAddrBindsAllInterfacesWhenLAN(t *testing.T) {
	if got := serveAddr(true); got != ":4242" {
		t.Fatalf("got %q", got)
	}
}
