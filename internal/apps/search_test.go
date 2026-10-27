package apps

import (
	"reflect"
	"testing"
)

// searchApps is a small list for the search tests, in a fixed input order.
func searchApps() []App {
	return []App{
		{ID: "code.desktop", Kind: KindDesktop, Name: "Visual Studio Code", GenericName: "Text Editor",
			Keywords: []string{"vscode", "Development"}, Binary: "/usr/share/code/bin/code", BinaryName: "code"},
		{ID: "codium.desktop", Kind: KindDesktop, Name: "Codium", GenericName: "Editor",
			Binary: "/usr/bin/codium", BinaryName: "codium"},
		{ID: "gedit.desktop", Kind: KindDesktop, Name: "gedit", GenericName: "Text Editor",
			Keywords: []string{"Notepad"}, Binary: "/usr/bin/gedit", BinaryName: "gedit"},
		{ID: "helper.desktop", Kind: KindDesktop, Name: "Code Helper", NoDisplay: true,
			Binary: "/usr/libexec/helper", BinaryName: "helper"},
		{ID: "ed", Kind: KindBinary, Name: "ed", Binary: "/usr/bin/ed", BinaryName: "ed", Terminal: true},
		{ID: "node", Kind: KindBinary, Name: "node", Binary: "/opt/code-tools/bin/node", BinaryName: "node", Terminal: true},
		{ID: "Codec", Kind: KindBinary, Name: "Codec", Binary: "/usr/bin/Codec", BinaryName: "Codec", Terminal: true},
	}
}

func TestSearch(t *testing.T) {
	built := func(a App) bool { return a.ID == "gedit.desktop" || a.ID == "node" || a.ID == "helper.desktop" }
	tests := []struct {
		name   string
		query  string
		filter Filter
		opts   SearchOpts
		want   []string
	}{
		{"empty query keeps the order, hides NoDisplay", "", FilterAll, SearchOpts{},
			[]string{"code.desktop", "codium.desktop", "gedit.desktop", "ed", "node", "Codec"}},
		{"blank query is empty", "  \t ", FilterAll, SearchOpts{},
			[]string{"code.desktop", "codium.desktop", "gedit.desktop", "ed", "node", "Codec"}},
		{"ShowHidden brings NoDisplay back", "", FilterAll, SearchOpts{ShowHidden: true},
			[]string{"code.desktop", "codium.desktop", "gedit.desktop", "helper.desktop", "ed", "node", "Codec"}},
		{"query in the name", "studio", FilterAll, SearchOpts{}, []string{"code.desktop"}},
		{"query in the generic name", "text editor", FilterAll, SearchOpts{}, []string{"code.desktop", "gedit.desktop"}},
		{"query in a keyword", "notepad", FilterAll, SearchOpts{}, []string{"gedit.desktop"}},
		{"keyword, other case", "VSCODE", FilterAll, SearchOpts{}, []string{"code.desktop"}},
		{"query in the binary name", "gedi", FilterAll, SearchOpts{}, []string{"gedit.desktop"}},
		{"query in the binary path", "/opt/", FilterAll, SearchOpts{}, []string{"node"}},
		{"query in a folder of the path", "libexec", FilterAll, SearchOpts{ShowHidden: true}, []string{"helper.desktop"}},
		{"query is trimmed", "  codium ", FilterAll, SearchOpts{}, []string{"codium.desktop"}},
		{"no match", "zzz", FilterAll, SearchOpts{}, []string{}},
		{"name prefix first, then the rest in input order", "code", FilterAll, SearchOpts{},
			[]string{"Codec", "code.desktop", "node"}},
		{"prefix is case-insensitive", "CODE", FilterAll, SearchOpts{},
			[]string{"Codec", "code.desktop", "node"}},
		{"a later app with a prefix match jumps ahead", "ed", FilterAll, SearchOpts{},
			[]string{"ed", "code.desktop", "codium.desktop", "gedit.desktop"}},
		{"stable inside both groups", "co", FilterAll, SearchOpts{},
			[]string{"codium.desktop", "Codec", "code.desktop", "node"}},
		{"menu filter", "", FilterMenu, SearchOpts{}, []string{"code.desktop", "codium.desktop", "gedit.desktop"}},
		{"menu filter with hidden", "", FilterMenu, SearchOpts{ShowHidden: true},
			[]string{"code.desktop", "codium.desktop", "gedit.desktop", "helper.desktop"}},
		{"binaries filter", "", FilterBinaries, SearchOpts{}, []string{"ed", "node", "Codec"}},
		{"binaries filter and query", "no", FilterBinaries, SearchOpts{}, []string{"node"}},
		{"built filter", "", FilterBuilt, SearchOpts{Built: built}, []string{"gedit.desktop", "node"}},
		{"built filter with hidden", "", FilterBuilt, SearchOpts{Built: built, ShowHidden: true},
			[]string{"gedit.desktop", "helper.desktop", "node"}},
		{"built filter and query", "ged", FilterBuilt, SearchOpts{Built: built}, []string{"gedit.desktop"}},
		{"built filter without Built keeps none", "", FilterBuilt, SearchOpts{}, []string{}},
		{"unknown filter keeps all", "", Filter(99), SearchOpts{},
			[]string{"code.desktop", "codium.desktop", "gedit.desktop", "ed", "node", "Codec"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := searchApps()
			got := ids(Search(in, tt.query, tt.filter, tt.opts))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Search(%q) = %v, want %v", tt.query, got, tt.want)
			}
			if !reflect.DeepEqual(in, searchApps()) {
				t.Error("Search changed its input")
			}
		})
	}
}

func TestSearchNilInput(t *testing.T) {
	if got := Search(nil, "x", FilterAll, SearchOpts{}); len(got) != 0 {
		t.Errorf("Search(nil) = %v", got)
	}
}
