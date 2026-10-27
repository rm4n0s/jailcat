package ui

import (
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"time"

	m "github.com/rm4n0s/glutter/material"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// settingLabel is the name of a setting as the tabs show it (SPEC §7).
func settingLabel(f firejail.Field) string {
	labels := map[firejail.Field]string{
		firejail.FieldHome: "Home folder", firejail.FieldPrivateDir: "Private home folder",
		firejail.FieldDownloadsReadWrite: "Downloads read-write", firejail.FieldHideRemovable: "Hide removable drives",
		firejail.FieldNoExecHome: "No programs from home", firejail.FieldPrivateTmp: "Private /tmp",
		firejail.FieldPrivateDev: "Private /dev", firejail.FieldPrivateCache: "Private cache",
		firejail.FieldNetwork: "Network", firejail.FieldDNS: "DNS servers", firejail.FieldHostsFile: "Hosts file",
		firejail.FieldFirewall: "Firewall",
		firejail.FieldSound:    "Sound and microphone", firejail.FieldCamera: "Camera", firejail.FieldGPU: "GPU / 3D acceleration",
		firejail.FieldPrinters: "Printers", firejail.FieldU2F: "U2F security keys", firejail.FieldInput: "Input devices",
		firejail.FieldDVD: "DVD drives", firejail.FieldTV: "TV and radio tuners", firejail.FieldTPM: "TPM",
		firejail.FieldSessionBus: "Session D-Bus", firejail.FieldSessionTalk: "Session D-Bus names to talk to",
		firejail.FieldSessionOwn: "Session D-Bus names to own", firejail.FieldSystemBus: "System D-Bus",
		firejail.FieldX11:     "X11 isolation",
		firejail.FieldSeccomp: "Seccomp filter", firejail.FieldDropCaps: "Drop all capabilities",
		firejail.FieldNoNewPrivs: "No new privileges", firejail.FieldNoRoot: "No root user in the sandbox",
		firejail.FieldRestrictNamespaces: "Restrict namespaces", firejail.FieldMemoryDenyWriteExecute: "Block writable-and-executable memory",
		firejail.FieldLandlock: "Landlock", firejail.FieldProtocols: "Socket types",
		firejail.FieldPrivateBin: "Only these programs", firejail.FieldPrivateEtc: "Only these /etc files",
		firejail.FieldEnv: "Environment variables", firejail.FieldExtraArgs: "Extra arguments",
		firejail.FieldMemory: "Memory", firejail.FieldCPUs: "CPU cores", firejail.FieldNice: "Priority",
		firejail.FieldTimeout: "Time limit", firejail.FieldMaxFiles: "Maximum open files", firejail.FieldMaxProcs: "Maximum processes",
		firejail.FieldSandboxName: "Sandbox name", firejail.FieldMenuName: "Menu name", firejail.FieldComment: "Comment",
		firejail.FieldIcon: "Icon", firejail.FieldIconSeed: "Random icon seed",
		firejail.FieldInstallMenu: "Install in the menu", firejail.FieldInstallBin: "Install in the bin folder",
		firejail.FieldOpenFileTypes: "Open file types", firejail.FieldSingleInstance: "Single instance",
		firejail.FieldTerminal: "Run in a terminal", firejail.FieldQuiet: "Hide firejail's messages",
	}
	if l, ok := labels[f]; ok {
		return l
	}
	return string(f)
}

// formatValue writes the value of a setting for the tabs.
func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "off"
	case bool:
		if x {
			return "on"
		}
		return "off"
	case []string:
		if len(x) == 0 {
			return "off"
		}
		return strings.Join(x, ", ")
	case []int:
		if len(x) == 0 {
			return "off"
		}
		parts := make([]string, len(x))
		for i, n := range x {
			parts[i] = strconv.Itoa(n)
		}
		return strings.Join(parts, ", ")
	case []firejail.EnvVar:
		if len(x) == 0 {
			return "off"
		}
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = e.Name + "=" + e.Value
		}
		return strings.Join(parts, ", ")
	case *int:
		if x == nil {
			return "off"
		}
		return strconv.Itoa(*x)
	case time.Duration:
		if x == 0 {
			return "off"
		}
		return x.String()
	case string:
		if x == "" {
			return "not set"
		}
		return x
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		if rv.String() == "" {
			return "not set"
		}
		return rv.String()
	case reflect.Int, reflect.Int64:
		if rv.Int() == 0 {
			return "off"
		}
		return strconv.FormatInt(rv.Int(), 10)
	}
	return fmt.Sprint(v)
}

// tabActions are what the body of a tab can ask the page to do.
type tabActions struct {
	// reset puts a setting back to its base value.
	reset func(f firejail.Field)
	// change runs a function that changes the editor inside the page's SetState, from any
	// goroutine.
	change func(apply func())
	// logger gets what the user must not read.
	logger *slog.Logger
}

// settingsOfTab returns the fields shown in a tab, in the order of the table.
func settingsOfTab(tab firejail.Tab) []firejail.Field {
	var out []firejail.Field
	for _, f := range firejail.Fields() {
		if f.Tab() == tab {
			out = append(out, f)
		}
	}
	return out
}

// tabBody is the body of a tab: the list of its settings and their current values,
// with a [ChangedMark] next to each one that differs from the base profile.
func tabBody(bc m.BuildContext, e *Editor, tab firejail.Tab, act tabActions) m.Widget {
	if tab == firejail.TabFiles {
		return NewFilesTab(e, act.logger, act.change)
	}
	var rows []m.Widget
	if tab == TabExtraRules {
		rows = extraRulesRows(bc, e)
	} else {
		final := e.Final()
		for _, f := range settingsOfTab(tab) {
			v, _ := f.Value(final)
			f := f
			var mark m.Widget
			if e.Changed(f) {
				mark = NewChangedMark(func() { act.reset(f) })
			}
			rows = append(rows, settingRow(bc, settingLabel(f), formatValue(v), mark, m.NewValueKey("setting:"+string(f))))
		}
	}
	return m.NewListView(rows, m.ListViewOpts{Key: m.NewValueKey("body:" + string(tab)), Padding: m.NewEdgeInsetsSymmetric(8, 16)})
}

// settingRow is one line of a tab: the name, the value and the mark.
func settingRow(bc m.BuildContext, label, value string, mark m.Widget, key m.Key) m.Widget {
	theme := m.ThemeOf(bc)
	muted := theme.TextTheme.BodyMedium.CopyWith(m.TextStyleCopy{Color: m.Some(theme.ColorScheme.OnSurfaceVariant())})
	children := []m.Widget{
		m.NewSizedBox(m.NewText(label, m.TextOpts{Style: theme.TextTheme.BodyMedium}), m.SizedBoxOpts{Width: m.Some(260.0)}),
		m.NewExpanded(m.NewText(value, m.TextOpts{Style: muted, MaxLines: m.Some(2), Overflow: m.TextOverflowEllipsis})),
	}
	if mark != nil {
		children = append(children, mark)
	}
	return m.NewSizedBox(m.NewRow(children, m.RowOpts{CrossAxisAlignment: m.CrossAxisAlignmentCenter}),
		m.SizedBoxOpts{Key: key, Height: m.Some(36.0)})
}

// extraRulesRows are the lines of the Extra rules tab: the rows the user wrote.
func extraRulesRows(bc m.BuildContext, e *Editor) []m.Widget {
	theme := m.ThemeOf(bc)
	var rows []m.Widget
	for i, r := range e.ExtraRules() {
		rows = append(rows, m.NewSizedBox(m.NewText(r, m.TextOpts{Style: theme.TextTheme.BodyMedium}),
			m.SizedBoxOpts{Key: m.NewValueKey("rule:" + strconv.Itoa(i)), Height: m.Some(28.0)}))
	}
	if len(rows) == 0 {
		rows = append(rows, m.NewText("No extra rules.", m.TextOpts{
			Style: theme.TextTheme.BodyMedium.CopyWith(m.TextStyleCopy{Color: m.Some(theme.ColorScheme.OnSurfaceVariant())}),
		}))
	}
	return rows
}
