// Command jailcat is a desktop GUI that builds firejail sandboxes for the applications of
// a Linux system and installs them in the application menu and the bin folder (SPEC §1).
//
// With -setup-desktop it only adds Jailcat itself to the application menu and exits
// (SPEC §3.2).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/rm4n0s/errors"
	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/apps"
	"github.com/rm4n0s/jailcat/internal/install"
	"github.com/rm4n0s/jailcat/internal/store"
	"github.com/rm4n0s/jailcat/internal/ui"
	"github.com/rm4n0s/jailcat/internal/xdg"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main with its arguments and output streams, and returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("jailcat", flag.ContinueOnError)
	flags.SetOutput(stderr)
	setupDesktop := flags.Bool("setup-desktop", false,
		"add Jailcat to the application menu (an entry and an icon under $XDG_DATA_HOME) and exit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "jailcat: unexpected argument %q\n", flags.Arg(0))
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	env, err := xdg.FromOS()
	if err != nil {
		return fail(stderr, "Jailcat could not find your home folder.", err)
	}
	dirs, err := xdg.Resolve(env)
	if err != nil {
		return fail(stderr, "Jailcat could not work out its folders.", err)
	}

	if *setupDesktop {
		return runSetupDesktop(ctx, dirs, stdout, stderr)
	}
	return runApp(ctx, env, dirs, stderr)
}

// runSetupDesktop writes Jailcat's menu entry and icon for the running program and prints
// where they are. It touches nothing else: no window, no log, no database, no folder of
// Jailcat's data.
func runSetupDesktop(ctx context.Context, dirs xdg.Dirs, stdout, stderr io.Writer) int {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jailcat: cannot find the running program: %v\n", err)
		return 1
	}
	res, err := install.SetupDesktop(ctx, dirs, exe)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jailcat: cannot add Jailcat to the application menu: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Wrote %s\nWrote %s\n", res.DesktopFile, res.IconFile)
	return 0
}

// runApp opens the database and the window.
func runApp(ctx context.Context, env xdg.Env, dirs xdg.Dirs, stderr io.Writer) int {
	level := slog.LevelInfo
	if v, _ := env.Lookup("JAILCAT_LOG"); v == "debug" {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))

	if err := xdg.EnsureDirs(ctx, dirs); err != nil {
		logger.Error("creating the folders failed", "err", err)
		return 1
	}
	st, err := store.Open(ctx, dirs.Database)
	if err != nil {
		logger.Error("opening the database failed", "err", err)
		return 1
	}
	defer func() {
		if err := st.Close(); err != nil {
			logger.Error("closing the database failed", "err", err)
		}
	}()

	backend := ui.NewLocalBackend(dirs, st, apps.LocaleFromEnv(env), logger)
	err = m.RunApp(ui.NewApp(backend, logger), m.RunAppOpts{
		AppID:  install.AppID,
		Logger: logger,
		Window: m.WindowConfig{Title: "Jailcat", Size: m.Size{Width: 1100, Height: 720}},
	})
	if err != nil {
		logger.Error("the window failed", "err", err)
		return 1
	}
	return 0
}

// fail prints a message for the user and the reason, and returns the exit code 1.
func fail(stderr io.Writer, message string, err error) int {
	_, _ = fmt.Fprintf(stderr, "jailcat: %s %v\n", message, err)
	return 1
}
