package desktopentry

import (
	"reflect"
	"strings"
	"testing"
)

// bs is one backslash.
const bs = "\\"

func TestLocaleString(t *testing.T) {
	const entry = `[Desktop Entry]
Name=Foo
Name[sr_YU]=sr_YU
Name[sr@Latn]=sr@Latn
Name[sr]=sr
Name[de]=de
Name[de_AT]=de_AT
Name[pt_BR@informal]=pt_BR@informal
Name[pt]=pt
Name[ca@valencia]=ca@valencia
Only[fr]=only french
Comment=default comment
`
	g := mustParse(t, entry).Main()
	tests := []struct {
		name   string
		key    string
		locale string
		want   string
	}{
		{"the specification's example: sr_YU wins over sr@Latn", "Name", "sr_YU@Latn", "sr_YU"},
		{"lang_COUNTRY@MODIFIER exact", "Name", "pt_BR@informal", "pt_BR@informal"},
		{"lang_COUNTRY@MODIFIER falls back to lang", "Name", "pt_BR@other", "pt"},
		{"lang_COUNTRY exact", "Name", "de_AT", "de_AT"},
		{"lang_COUNTRY falls back to lang", "Name", "de_CH", "de"},
		{"encoding is ignored", "Name", "de_AT.UTF-8", "de_AT"},
		{"encoding and modifier", "Name", "pt_BR.ISO-8859-1@informal", "pt_BR@informal"},
		{"lang@MODIFIER", "Name", "ca@valencia", "ca@valencia"},
		{"lang@MODIFIER with encoding", "Name", "ca.UTF-8@valencia", "ca@valencia"},
		{"no country in the locale: no key with a country is matched", "Name", "pt", "pt"},
		{"no modifier in the locale: no key with a modifier is matched", "Name", "ca", "Foo"},
		{"lang_COUNTRY then lang@MODIFIER", "Name", "sr_XX@Latn", "sr@Latn"},
		{"plain lang", "Name", "de", "de"},
		{"unknown language uses the key without a locale", "Name", "ja_JP.UTF-8", "Foo"},
		{"empty locale uses the key without a locale", "Name", "", "Foo"},
		{"C locale uses the key without a locale", "Name", "C", "Foo"},
		{"POSIX locale uses the key without a locale", "Name", "POSIX", "Foo"},
		{"key with no translation", "Comment", "de_DE", "default comment"},
		{"missing key", "Missing", "de", ""},
		{"localized key without the plain key is still found for its locale", "Only", "fr_FR", "only french"},
		{"localized key without the plain key is missing for another locale", "Only", "de", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := g.LocaleString(tt.key, tt.locale); got != tt.want {
				t.Errorf("LocaleString(%q, %q) = %q, want %q", tt.key, tt.locale, got, tt.want)
			}
		})
	}
	var nilGroup *Group
	if got := nilGroup.LocaleString("Name", "de"); got != "" {
		t.Errorf("LocaleString on a nil group = %q", got)
	}
}

func TestLocaleStrings(t *testing.T) {
	g := mustParse(t, "[G]\nKeywords=a;b;\nKeywords[de]=c\\;d;e;\n").Lookup("G")
	tests := []struct {
		locale string
		want   []string
	}{
		{"de_DE.UTF-8", []string{"c;d", "e"}},
		{"fr", []string{"a", "b"}},
	}
	for _, tt := range tests {
		if got := g.LocaleStrings("Keywords", tt.locale); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("LocaleStrings(%q) = %q, want %q", tt.locale, got, tt.want)
		}
	}
}

func TestStrings(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"one item, no terminator", "a", []string{"a"}},
		{"one item, terminated", "a;", []string{"a"}},
		{"several", "a;b;c", []string{"a", "b", "c"}},
		{"several, terminated", "a;b;c;", []string{"a", "b", "c"}},
		{"escaped semicolon", `a\;b;c;`, []string{"a;b", "c"}},
		{"escaped backslash before the separator", "a" + bs + bs + ";b", []string{"a" + bs, "b"}},
		{"escaped backslash then escaped semicolon", "a" + bs + bs + bs + ";b", []string{"a" + bs + ";b"}},
		{"only a semicolon is one empty item", ";", []string{""}},
		{"trailing empty item must be terminated", "a;;", []string{"a", ""}},
		{"empty item in the middle", "a;;b", []string{"a", "", "b"}},
		{"general escapes", `a\sb;c\td\n;`, []string{"a b", "c\td\n"}},
		{"unknown escape is kept", `a\xb;`, []string{`a\xb`}},
		{"non-ASCII", "café;日本語;", []string{"café", "日本語"}},
		{"trailing lone backslash", `a\`, []string{`a\`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := mustParse(t, "[G]\nK="+tt.raw+"\n").Lookup("G")
			if got := g.Strings("K"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Strings = %q, want %q", got, tt.want)
			}
		})
	}
	g := mustParse(t, "[G]\n").Lookup("G")
	if got := g.Strings("Missing"); got != nil {
		t.Errorf("Strings of a missing key = %q", got)
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"plain", "Hello", "Hello"},
		{"spaces escape", `\sa\s`, " a "},
		{"newline tab return", `a\nb\tc\rd`, "a\nb\tc\rd"},
		{"backslash", `C:\\temp`, `C:\temp`},
		{"four backslashes are two", `\\\\`, `\\`},
		{"escaped n after escaped backslash is text", `\\n`, `\n`},
		{"unknown escape kept", `\x\;`, `\x\;`},
		{"trailing lone backslash kept", `a\`, `a\`},
		{"leading and trailing spaces dropped", "   a b   ", "a b"},
		{"equals sign in the value", "a=b=c", "a=b=c"},
		{"non-ASCII", "Café 日本", "Café 日本"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := mustParse(t, "[G]\nK="+tt.raw+"\n").Lookup("G")
			if got := g.String("K"); got != tt.want {
				t.Errorf("String = %q, want %q", got, tt.want)
			}
		})
	}
	g := mustParse(t, "[G]\nK=v\nK[de]=w\n").Lookup("G")
	if !g.Has("K") || !g.Has("K[de]") || g.Has("K[fr]") || g.Has("Z") {
		t.Error("Has gives the wrong answer")
	}
	if g.String("K[de]") != "w" {
		t.Errorf("String(K[de]) = %q", g.String("K[de]"))
	}
	var nilGroup *Group
	if nilGroup.String("K") != "" || nilGroup.Has("K") || nilGroup.Keys() != nil || nilGroup.Strings("K") != nil || nilGroup.Name() != "" {
		t.Error("a nil group should read as empty")
	}
}

func TestBool(t *testing.T) {
	tests := []struct {
		name  string
		raw   string // "-" means the key is missing
		want  bool
		route string
	}{
		{name: "true", raw: "true", want: true},
		{name: "false", raw: "false"},
		{name: "missing is false", raw: "-"},
		{name: "True is not a boolean", raw: "True", route: "Group.Bool.InvalidBool"},
		{name: "1 is not a boolean", raw: "1", route: "Group.Bool.InvalidBool"},
		{name: "yes is not a boolean", raw: "yes", route: "Group.Bool.InvalidBool"},
		{name: "empty is not a boolean", raw: "", route: "Group.Bool.InvalidBool"},
		{name: "true with a semicolon is not a boolean", raw: "true;", route: "Group.Bool.InvalidBool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := "[G]\n"
			if tt.raw != "-" {
				text += "K=" + tt.raw + "\n"
			}
			got, err := mustParse(t, text).Lookup("G").Bool("K")
			if tt.route != "" {
				if err == nil {
					t.Fatalf("expected an error, got %v", got)
				}
				checkRoute(t, err, tt.route)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Bool = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEscapeValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Hello", "Hello"},
		{"empty", "", ""},
		{"backslash", `a\b`, `a\\b`},
		{"newline", "a\nb", `a\nb`},
		{"tab", "a\tb", `a\tb`},
		{"carriage return", "a\rb", `a\rb`},
		{"windows line break", "a\r\nb", `a\r\nb`},
		{"backslash then n is not a newline", `a\nb`, `a\\nb`},
		{"inner spaces stay", "a b", "a b"},
		{"leading space", " a", `\sa`},
		{"trailing space", "a ", `a\s`},
		{"both ends", "  a  ", `\s\sa\s\s`},
		{"only spaces", "   ", `\s\s\s`},
		{"semicolon is not escaped in a plain value", "a;b", "a;b"},
		{"non-ASCII", "Café 日本", "Café 日本"},
		{"percent", "100%", "100%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeValue(tt.in)
			if got != tt.want {
				t.Fatalf("EscapeValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if strings.ContainsAny(got, "\n\r") {
				t.Errorf("the result %q holds a line break", got)
			}
			if back := unescape(strings.Trim(got, " \t")); back != tt.in {
				t.Errorf("unescape(EscapeValue(%q)) = %q", tt.in, back)
			}
		})
	}
}

func TestEscapeList(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  string
	}{
		{"none", nil, ""},
		{"one", []string{"a"}, "a;"},
		{"several", []string{"a", "b"}, "a;b;"},
		{"semicolon in an item", []string{"a;b", "c"}, `a\;b;c;`},
		{"backslash then semicolon", []string{"a" + bs, "b"}, "a" + bs + bs + ";b;"},
		{"empty last item", []string{"a", ""}, "a;;"},
		{"only an empty item", []string{""}, ";"},
		{"line break", []string{"a\nb"}, `a\nb;`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeList(tt.items)
			if got != tt.want {
				t.Fatalf("EscapeList = %q, want %q", got, tt.want)
			}
			if back := splitList(got); !reflect.DeepEqual(back, tt.items) && (len(back) != 0 || len(tt.items) != 0) {
				t.Errorf("splitList(EscapeList(%q)) = %q", tt.items, back)
			}
		})
	}
}
