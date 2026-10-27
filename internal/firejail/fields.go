package firejail

import (
	"encoding/json"
	"slices"
	"time"
)

// Tab is a tab of the build editor (SPEC §6, §7). The Extra rules tab has no
// settings, so it is not here.
type Tab string

// The tabs that hold settings.
const (
	// TabFiles is the Files tab (SPEC §7.1).
	TabFiles Tab = "files"
	// TabNetwork is the Network tab (SPEC §7.2).
	TabNetwork Tab = "network"
	// TabDevices is the Devices tab (SPEC §7.3).
	TabDevices Tab = "devices"
	// TabDesktop is the Desktop tab (SPEC §7.4).
	TabDesktop Tab = "desktop"
	// TabSecurity is the Security tab (SPEC §7.5).
	TabSecurity Tab = "security"
	// TabPrograms is the Programs tab (SPEC §7.6).
	TabPrograms Tab = "programs"
	// TabLimits is the Limits tab (SPEC §7.7).
	TabLimits Tab = "limits"
	// TabLauncher is the Launcher tab (SPEC §7.8).
	TabLauncher Tab = "launcher"
)

// Field names one setting of [Settings]. The folder list is not a field: its rows are
// overridden one by one ([FolderOverride]). The values are the keys of the stored
// overrides (SPEC §12): never change one that was released.
type Field string

// The fields, one per setting of SPEC §7.1 to §7.8.
const (
	// FieldHome is [Settings.Home].
	FieldHome Field = "home"
	// FieldPrivateDir is [Settings.PrivateDir].
	FieldPrivateDir Field = "private_dir"
	// FieldDownloadsReadWrite is [Settings.DownloadsReadWrite].
	FieldDownloadsReadWrite Field = "downloads_read_write"
	// FieldHideRemovable is [Settings.HideRemovable].
	FieldHideRemovable Field = "hide_removable"
	// FieldNoExecHome is [Settings.NoExecHome].
	FieldNoExecHome Field = "no_exec_home"
	// FieldPrivateTmp is [Settings.PrivateTmp].
	FieldPrivateTmp Field = "private_tmp"
	// FieldPrivateDev is [Settings.PrivateDev].
	FieldPrivateDev Field = "private_dev"
	// FieldPrivateCache is [Settings.PrivateCache].
	FieldPrivateCache Field = "private_cache"

	// FieldNetwork is [Settings.Network].
	FieldNetwork Field = "network"
	// FieldDNS is [Settings.DNS].
	FieldDNS Field = "dns"
	// FieldHostsFile is [Settings.HostsFile].
	FieldHostsFile Field = "hosts_file"
	// FieldFirewall is [Settings.Firewall].
	FieldFirewall Field = "firewall"

	// FieldSound is [Settings.Sound].
	FieldSound Field = "sound"
	// FieldCamera is [Settings.Camera].
	FieldCamera Field = "camera"
	// FieldGPU is [Settings.GPU].
	FieldGPU Field = "gpu"
	// FieldPrinters is [Settings.Printers].
	FieldPrinters Field = "printers"
	// FieldU2F is [Settings.U2F].
	FieldU2F Field = "u2f"
	// FieldInput is [Settings.Input].
	FieldInput Field = "input"
	// FieldDVD is [Settings.DVD].
	FieldDVD Field = "dvd"
	// FieldTV is [Settings.TV].
	FieldTV Field = "tv"
	// FieldTPM is [Settings.TPM].
	FieldTPM Field = "tpm"

	// FieldSessionBus is [Settings.SessionBus].
	FieldSessionBus Field = "session_bus"
	// FieldSessionTalk is [Settings.SessionTalk].
	FieldSessionTalk Field = "session_talk"
	// FieldSessionOwn is [Settings.SessionOwn].
	FieldSessionOwn Field = "session_own"
	// FieldSystemBus is [Settings.SystemBus].
	FieldSystemBus Field = "system_bus"
	// FieldX11 is [Settings.X11].
	FieldX11 Field = "x11"

	// FieldSeccomp is [Settings.Seccomp].
	FieldSeccomp Field = "seccomp"
	// FieldDropCaps is [Settings.DropCaps].
	FieldDropCaps Field = "drop_caps"
	// FieldNoNewPrivs is [Settings.NoNewPrivs].
	FieldNoNewPrivs Field = "no_new_privs"
	// FieldNoRoot is [Settings.NoRoot].
	FieldNoRoot Field = "no_root"
	// FieldRestrictNamespaces is [Settings.RestrictNamespaces].
	FieldRestrictNamespaces Field = "restrict_namespaces"
	// FieldMemoryDenyWriteExecute is [Settings.MemoryDenyWriteExecute].
	FieldMemoryDenyWriteExecute Field = "memory_deny_write_execute"
	// FieldLandlock is [Settings.Landlock].
	FieldLandlock Field = "landlock"
	// FieldProtocols is [Settings.Protocols].
	FieldProtocols Field = "protocols"

	// FieldPrivateBin is [Settings.PrivateBin].
	FieldPrivateBin Field = "private_bin"
	// FieldPrivateEtc is [Settings.PrivateEtc].
	FieldPrivateEtc Field = "private_etc"
	// FieldEnv is [Settings.Env].
	FieldEnv Field = "env"
	// FieldExtraArgs is [Settings.ExtraArgs].
	FieldExtraArgs Field = "extra_args"

	// FieldMemory is [Settings.MemoryBytes].
	FieldMemory Field = "memory"
	// FieldCPUs is [Settings.CPUs].
	FieldCPUs Field = "cpus"
	// FieldNice is [Settings.Nice].
	FieldNice Field = "nice"
	// FieldTimeout is [Settings.Timeout].
	FieldTimeout Field = "timeout"
	// FieldMaxFiles is [Settings.MaxFiles].
	FieldMaxFiles Field = "max_files"
	// FieldMaxProcs is [Settings.MaxProcs].
	FieldMaxProcs Field = "max_procs"

	// FieldSandboxName is [Settings.SandboxName].
	FieldSandboxName Field = "sandbox_name"
	// FieldMenuName is [Settings.MenuName].
	FieldMenuName Field = "menu_name"
	// FieldComment is [Settings.Comment].
	FieldComment Field = "comment"
	// FieldIcon is [Settings.Icon].
	FieldIcon Field = "icon"
	// FieldIconSeed is [Settings.IconSeed].
	FieldIconSeed Field = "icon_seed"
	// FieldInstallMenu is [Settings.InstallMenu].
	FieldInstallMenu Field = "install_menu"
	// FieldInstallBin is [Settings.InstallBin].
	FieldInstallBin Field = "install_bin"
	// FieldOpenFileTypes is [Settings.OpenFileTypes].
	FieldOpenFileTypes Field = "open_file_types"
	// FieldSingleInstance is [Settings.SingleInstance].
	FieldSingleInstance Field = "single_instance"
	// FieldTerminal is [Settings.Terminal].
	FieldTerminal Field = "terminal"
	// FieldQuiet is [Settings.Quiet].
	FieldQuiet Field = "quiet"
)

// fieldRow is one row of the field table: everything the package needs to know about
// one setting. Diff, Apply, the JSON and the dots of the tabs are all built on it, so
// a new field is one row.
type fieldRow struct {
	field Field
	tab   Tab
	// get returns the field's value; a slice or pointer is a copy.
	get func(s *Settings) any
	// set stores a value that get returned (or decode made) into s, as a copy.
	set func(s *Settings, v any)
	// equal compares the field of two Settings. A nil and an empty list are equal.
	equal func(a, b *Settings) bool
	// decode reads the stored JSON of the field into the value set accepts.
	decode func(raw json.RawMessage) (any, error)
}

// newRow builds a row from a pointer to the field, how to compare two values and how to
// copy one.
func newRow[T any](f Field, tab Tab, p func(s *Settings) *T, eq func(a, b T) bool, clone func(v T) T) fieldRow {
	return fieldRow{
		field: f,
		tab:   tab,
		get:   func(s *Settings) any { return clone(*p(s)) },
		set:   func(s *Settings, v any) { *p(s) = clone(v.(T)) },
		equal: func(a, b *Settings) bool { return eq(*p(a), *p(b)) },
		decode: func(raw json.RawMessage) (any, error) {
			var v T
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
			return v, nil
		},
	}
}

// scalar is a row for a field that is compared with ==.
func scalar[T comparable](f Field, tab Tab, p func(s *Settings) *T) fieldRow {
	return newRow(f, tab, p, func(a, b T) bool { return a == b }, func(v T) T { return v })
}

// list is a row for a slice field: nil and empty both mean "off" and are equal.
func list[T comparable](f Field, tab Tab, p func(s *Settings) *[]T) fieldRow {
	return newRow(f, tab, p, func(a, b []T) bool { return slices.Equal(a, b) }, func(v []T) []T { return slices.Clone(v) })
}

// fieldTable returns the table of every field, in the order of the tabs. It is a
// function so that no package-level variable holds the closures; each call is a fresh
// slice.
func fieldTable() []fieldRow {
	return []fieldRow{
		scalar(FieldHome, TabFiles, func(s *Settings) *HomeMode { return &s.Home }),
		scalar(FieldPrivateDir, TabFiles, func(s *Settings) *string { return &s.PrivateDir }),
		scalar(FieldDownloadsReadWrite, TabFiles, func(s *Settings) *bool { return &s.DownloadsReadWrite }),
		scalar(FieldHideRemovable, TabFiles, func(s *Settings) *bool { return &s.HideRemovable }),
		scalar(FieldNoExecHome, TabFiles, func(s *Settings) *bool { return &s.NoExecHome }),
		scalar(FieldPrivateTmp, TabFiles, func(s *Settings) *bool { return &s.PrivateTmp }),
		scalar(FieldPrivateDev, TabFiles, func(s *Settings) *bool { return &s.PrivateDev }),
		scalar(FieldPrivateCache, TabFiles, func(s *Settings) *bool { return &s.PrivateCache }),

		scalar(FieldNetwork, TabNetwork, func(s *Settings) *NetMode { return &s.Network }),
		list(FieldDNS, TabNetwork, func(s *Settings) *[]string { return &s.DNS }),
		scalar(FieldHostsFile, TabNetwork, func(s *Settings) *string { return &s.HostsFile }),
		scalar(FieldFirewall, TabNetwork, func(s *Settings) *bool { return &s.Firewall }),

		scalar(FieldSound, TabDevices, func(s *Settings) *bool { return &s.Sound }),
		scalar(FieldCamera, TabDevices, func(s *Settings) *bool { return &s.Camera }),
		scalar(FieldGPU, TabDevices, func(s *Settings) *bool { return &s.GPU }),
		scalar(FieldPrinters, TabDevices, func(s *Settings) *bool { return &s.Printers }),
		scalar(FieldU2F, TabDevices, func(s *Settings) *bool { return &s.U2F }),
		scalar(FieldInput, TabDevices, func(s *Settings) *bool { return &s.Input }),
		scalar(FieldDVD, TabDevices, func(s *Settings) *bool { return &s.DVD }),
		scalar(FieldTV, TabDevices, func(s *Settings) *bool { return &s.TV }),
		scalar(FieldTPM, TabDevices, func(s *Settings) *bool { return &s.TPM }),

		scalar(FieldSessionBus, TabDesktop, func(s *Settings) *BusMode { return &s.SessionBus }),
		list(FieldSessionTalk, TabDesktop, func(s *Settings) *[]string { return &s.SessionTalk }),
		list(FieldSessionOwn, TabDesktop, func(s *Settings) *[]string { return &s.SessionOwn }),
		scalar(FieldSystemBus, TabDesktop, func(s *Settings) *BusMode { return &s.SystemBus }),
		scalar(FieldX11, TabDesktop, func(s *Settings) *X11Mode { return &s.X11 }),

		scalar(FieldSeccomp, TabSecurity, func(s *Settings) *bool { return &s.Seccomp }),
		scalar(FieldDropCaps, TabSecurity, func(s *Settings) *bool { return &s.DropCaps }),
		scalar(FieldNoNewPrivs, TabSecurity, func(s *Settings) *bool { return &s.NoNewPrivs }),
		scalar(FieldNoRoot, TabSecurity, func(s *Settings) *bool { return &s.NoRoot }),
		scalar(FieldRestrictNamespaces, TabSecurity, func(s *Settings) *bool { return &s.RestrictNamespaces }),
		scalar(FieldMemoryDenyWriteExecute, TabSecurity, func(s *Settings) *bool { return &s.MemoryDenyWriteExecute }),
		scalar(FieldLandlock, TabSecurity, func(s *Settings) *bool { return &s.Landlock }),
		list(FieldProtocols, TabSecurity, func(s *Settings) *[]string { return &s.Protocols }),

		list(FieldPrivateBin, TabPrograms, func(s *Settings) *[]string { return &s.PrivateBin }),
		list(FieldPrivateEtc, TabPrograms, func(s *Settings) *[]string { return &s.PrivateEtc }),
		list(FieldEnv, TabPrograms, func(s *Settings) *[]EnvVar { return &s.Env }),
		list(FieldExtraArgs, TabPrograms, func(s *Settings) *[]string { return &s.ExtraArgs }),

		scalar(FieldMemory, TabLimits, func(s *Settings) *int64 { return &s.MemoryBytes }),
		list(FieldCPUs, TabLimits, func(s *Settings) *[]int { return &s.CPUs }),
		newRow(FieldNice, TabLimits, func(s *Settings) **int { return &s.Nice }, equalInt, cloneInt),
		scalar(FieldTimeout, TabLimits, func(s *Settings) *time.Duration { return &s.Timeout }),
		scalar(FieldMaxFiles, TabLimits, func(s *Settings) *int64 { return &s.MaxFiles }),
		scalar(FieldMaxProcs, TabLimits, func(s *Settings) *int64 { return &s.MaxProcs }),

		scalar(FieldSandboxName, TabLauncher, func(s *Settings) *string { return &s.SandboxName }),
		scalar(FieldMenuName, TabLauncher, func(s *Settings) *string { return &s.MenuName }),
		scalar(FieldComment, TabLauncher, func(s *Settings) *string { return &s.Comment }),
		scalar(FieldIcon, TabLauncher, func(s *Settings) *IconMode { return &s.Icon }),
		scalar(FieldIconSeed, TabLauncher, func(s *Settings) *int64 { return &s.IconSeed }),
		scalar(FieldInstallMenu, TabLauncher, func(s *Settings) *bool { return &s.InstallMenu }),
		scalar(FieldInstallBin, TabLauncher, func(s *Settings) *bool { return &s.InstallBin }),
		scalar(FieldOpenFileTypes, TabLauncher, func(s *Settings) *bool { return &s.OpenFileTypes }),
		scalar(FieldSingleInstance, TabLauncher, func(s *Settings) *bool { return &s.SingleInstance }),
		scalar(FieldTerminal, TabLauncher, func(s *Settings) *bool { return &s.Terminal }),
		scalar(FieldQuiet, TabLauncher, func(s *Settings) *bool { return &s.Quiet }),
	}
}

// equalInt compares two optional ints by value; nil is off and equals only nil.
func equalInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// cloneInt copies an optional int.
func cloneInt(v *int) *int {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// findRow returns the row of f, or false when f is not a known field.
func findRow(f Field) (fieldRow, bool) {
	for _, row := range fieldTable() {
		if row.field == f {
			return row, true
		}
	}
	return fieldRow{}, false
}

// Fields returns every field, in the order of the tabs, as a fresh slice.
func Fields() []Field {
	table := fieldTable()
	out := make([]Field, len(table))
	for i, row := range table {
		out[i] = row.field
	}
	return out
}

// Tab returns the tab that shows the field, or "" for a name that is not a field.
func (f Field) Tab() Tab {
	row, _ := findRow(f)
	return row.tab
}

// Equal reports whether a and b have the same value in this field. A name that is not
// a field is never equal.
func (f Field) Equal(a, b Settings) bool {
	row, ok := findRow(f)
	return ok && row.equal(&a, &b)
}
