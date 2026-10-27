package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSetupDesktop(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", data)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-setup-desktop"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	desktop := filepath.Join(data, "applications", "io.github.rm4n0s.jailcat.desktop")
	icon := filepath.Join(data, "icons", "hicolor", "scalable", "apps", "io.github.rm4n0s.jailcat.svg")
	for _, p := range []string{desktop, icon} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("not written: %v", err)
		}
		if !strings.Contains(stdout.String(), p) {
			t.Errorf("stdout does not say where %s is:\n%s", p, stdout.String())
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(desktop)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Exec="+exe+"\n") {
		t.Errorf("Exec is not the running program %s:\n%s", exe, got)
	}
	// Nothing but the two files: no folder of Jailcat's data, no database.
	if _, err := os.Stat(filepath.Join(data, "jailcat")); !os.IsNotExist(err) {
		t.Errorf("Jailcat's data folder was made: %v", err)
	}
}

func TestRunSetupDesktopFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A file where the data folder should be.
	blocked := filepath.Join(home, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", blocked)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-setup-desktop"}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "cannot add Jailcat to the application menu") || stdout.Len() != 0 {
		t.Errorf("stdout %q, stderr %q", stdout.String(), stderr.String())
	}
}

func TestRunBadArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"unknown flag", []string{"-nope"}, 2},
		{"stray argument", []string{"here"}, 2},
		{"help", []string{"-h"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != tt.code {
				t.Errorf("exit code %d, want %d (stderr %s)", code, tt.code, stderr.String())
			}
		})
	}
}
