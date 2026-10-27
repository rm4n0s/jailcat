package firejail

import (
	"testing"

	jerrors "github.com/rm4n0s/errors"
)

func TestShellQuote(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "code", `'code'`},
		{"empty", "", `''`},
		{"spaces", "a b  c", `'a b  c'`},
		{"single quote", "it's", `'it'\''s'`},
		{"two single quotes", "''", `''\'''\'''`},
		{"only a single quote", "'", `''\'''`},
		{"double quote", `say "hi"`, `'say "hi"'`},
		{"dollar", "$HOME ${X} $(id)", `'$HOME ${X} $(id)'`},
		{"backtick", "`id`", "'`id`'"},
		{"backslash", `a\nb\`, `'a\nb\'`},
		{"bang", "a!b", `'a!b'`},
		{"star", "*.go", `'*.go'`},
		{"tilde", "~/x", `'~/x'`},
		{"newline", "a\nb", "'a\nb'"},
		{"tab", "a\tb", "'a\tb'"},
		{"non-ASCII", "Ünï cödé 日本 🐱", "'Ünï cödé 日本 🐱'"},
		{"percent and hash", "%F #x", `'%F #x'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShellQuote(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("ShellQuote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestShellQuoteNul(t *testing.T) {
	_, err := ShellQuote("a\x00b")
	appErr, ok := jerrors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T", err)
	}
	if !appErr.HasRoute("ShellQuote.ShellWordHasNul") {
		t.Errorf("unexpected failure path: %s", appErr.Route())
	}
}
