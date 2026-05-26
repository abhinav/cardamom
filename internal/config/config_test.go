package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default should validate: %v", err)
	}
}

func TestValidatePrefix(t *testing.T) {
	cases := map[string]bool{ // input -> wantValid
		"clu-":       true,
		"acme-":      true,
		"a-":         true,
		"my-team-":   true,
		"a1-":        true,
		"123-":       true,
		"":           false,
		"clu":        false, // no trailing dash
		"-":          false, // starts with dash
		"--":         false, // empty segments
		"clu--":      false, // empty segment
		"clu_":       false, // underscore not allowed
		"CLU-":       false, // uppercase
		"too-very-long-prefix-": false, // > 16 chars
	}
	for in, wantValid := range cases {
		err := Config{IDPrefix: in}.Validate()
		gotValid := err == nil
		if gotValid != wantValid {
			t.Errorf("%q: want valid=%v, got err=%v", in, wantValid, err)
		}
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("missing config should not error: %v", err)
	}
	if got.IDPrefix != Default().IDPrefix {
		t.Fatalf("expected default, got %+v", got)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	body := "id_prefix: acme-\nbogus_field: 42\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error on unknown field")
	}
	if !strings.Contains(err.Error(), "bogus_field") {
		t.Fatalf("error should mention the bad field: %v", err)
	}
}

func TestLoadRejectsBadPrefix(t *testing.T) {
	dir := t.TempDir()
	body := "id_prefix: NOPE\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestWriteThenLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Config{IDPrefix: "acme-"}
	if err := Write(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.IDPrefix != want.IDPrefix {
		t.Fatalf("roundtrip mismatch: want %q, got %q", want.IDPrefix, got.IDPrefix)
	}
	// Written content has at least one comment line so humans see the doc.
	body, _ := os.ReadFile(Path(dir))
	if !strings.Contains(string(body), "# clu project configuration") {
		t.Fatalf("expected comments in written config:\n%s", body)
	}
}

func TestWriteRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, Default()); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, Default()); err == nil {
		t.Fatal("Write should refuse to overwrite an existing config")
	}
}
