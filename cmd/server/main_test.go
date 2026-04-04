package main

import (
	"os"
	"testing"
)

func TestEnsureProtoRegistrationConflictModeSetsDefault(t *testing.T) {
	t.Setenv("GOLANG_PROTOBUF_REGISTRATION_CONFLICT", "")

	ensureProtoRegistrationConflictMode()

	if got := os.Getenv("GOLANG_PROTOBUF_REGISTRATION_CONFLICT"); got != "warn" {
		t.Fatalf("expected default conflict mode warn, got %q", got)
	}
}

func TestEnsureProtoRegistrationConflictModePreservesExplicitValue(t *testing.T) {
	t.Setenv("GOLANG_PROTOBUF_REGISTRATION_CONFLICT", "panic")

	ensureProtoRegistrationConflictMode()

	if got := os.Getenv("GOLANG_PROTOBUF_REGISTRATION_CONFLICT"); got != "panic" {
		t.Fatalf("expected explicit conflict mode to be preserved, got %q", got)
	}
}
