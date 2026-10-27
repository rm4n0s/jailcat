package ui

import (
	"log/slog"

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

// tabActions are what the body of a tab can ask the page to do.
type tabActions struct {
	// reset puts a setting back to its base value.
	reset func(f firejail.Field)
	// change runs a function that changes the editor inside the page's SetState, from any
	// goroutine.
	change func(apply func())
	// logger gets what the user must not read.
	logger *slog.Logger
	// backend answers what the system has, for the tabs that ask.
	backend Backend
}

// tabBody is the body of a tab: the tab's own widget for the editor.
func tabBody(bc m.BuildContext, e *Editor, tab firejail.Tab, act tabActions) m.Widget {
	switch tab {
	case firejail.TabFiles:
		return NewFilesTab(e, act.logger, act.change)
	case firejail.TabNetwork:
		return NewNetworkTab(e, act.logger, act.change)
	case firejail.TabDevices:
		return NewDevicesTab(e, act.change)
	case firejail.TabDesktop:
		return NewDesktopTab(e, act.backend, act.change)
	case firejail.TabSecurity:
		return NewSecurityTab(e, act.change)
	case firejail.TabPrograms:
		return NewProgramsTab(e, act.change)
	case firejail.TabLimits:
		return NewLimitsTab(e, act.backend, act.change)
	case firejail.TabLauncher:
		return NewLauncherTab(e, act.backend, act.logger, act.change)
	case TabExtraRules:
		return NewExtraRulesTab(e, act.backend, act.change)
	}
	return m.NewSizedBox(nil, m.SizedBoxOpts{})
}
