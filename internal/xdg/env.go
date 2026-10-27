package xdg

import (
	"os"
	"path/filepath"

	"github.com/rm4n0s/errors"
)

// Env is the environment Jailcat resolves its folders from: a variable lookup and the
// home folder. Only [FromOS] reads the real process; everything else gets an Env.
type Env struct {
	lookup func(string) (string, bool)
	home   string
}

// NewEnv builds an Env from a home folder and a map of variables. A variable that is
// not in the map is unset. It is meant for tests and for fixtures.
func NewEnv(home string, vars map[string]string) Env {
	return Env{
		lookup: func(name string) (string, bool) {
			v, ok := vars[name]
			return v, ok
		},
		home: home,
	}
}

// NewEnvFunc builds an Env from a home folder and a lookup function with the shape of
// [os.LookupEnv].
func NewEnvFunc(home string, lookup func(string) (string, bool)) Env {
	return Env{lookup: lookup, home: home}
}

// FromOS builds an Env from the running process: [os.LookupEnv] and [os.UserHomeDir].
// It is the only function of Jailcat that reads the process environment.
func FromOS() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, errors.NewErr(tagHomeLookupFailed, err)
	}
	return Env{lookup: os.LookupEnv, home: home}, nil
}

// Home returns the home folder as given, not yet checked.
func (e Env) Home() string {
	return e.home
}

// Lookup returns the value of the variable name and whether it is set. An Env with no
// lookup function has no variables.
func (e Env) Lookup(name string) (string, bool) {
	if e.lookup == nil {
		return "", false
	}
	return e.lookup(name)
}

// absVar returns the cleaned value of the variable name when it is set, not empty and
// absolute. The XDG Base Directory Specification says any other value is invalid and
// must be ignored.
func (e Env) absVar(name string) (string, bool) {
	v, ok := e.Lookup(name)
	if !ok || v == "" || !filepath.IsAbs(v) {
		return "", false
	}
	return filepath.Clean(v), true
}

const tagHomeLookupFailed = "HomeLookupFailed"
