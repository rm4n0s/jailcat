package desktopentry

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitExec(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{"empty", "", nil},
		{"only spaces", "   ", nil},
		{"program only", "firefox", []string{"firefox"}},
		{"program and field code", "firefox %u", []string{"firefox", "%u"}},
		{"several spaces and tabs separate", "a  b\tc", []string{"a", "b", "c"}},
		{"leading and trailing spaces", "  a b  ", []string{"a", "b"}},
		{"VS Code", "/usr/share/code/code --new-window %F", []string{"/usr/share/code/code", "--new-window", "%F"}},
		{"quoted program with a space", `"/opt/My App/bin/app" --flag "a b" %F`, []string{"/opt/My App/bin/app", "--flag", "a b", "%F"}},
		{"quoted with reserved characters", `kde-geo --t "https://x.org/?a=<A>&b=<B>#c" %u`, []string{"kde-geo", "--t", "https://x.org/?a=<A>&b=<B>#c", "%u"}},
		{"escaped quote", `"say \"hi\""`, []string{`say "hi"`}},
		{"escaped backtick", "\"a\\`b\"", []string{"a`b"}},
		{"escaped dollar", `"a\$HOME"`, []string{"a$HOME"}},
		{"escaped backslash", `"a\\b"`, []string{`a\b`}},
		{"other backslash inside quotes is kept", `"a\xb"`, []string{`a\xb`}},
		{"backslash at the end inside quotes is kept", `"a\\"`, []string{`a\`}},
		{"single quote inside double quotes", `"it's"`, []string{"it's"}},
		{"empty quoted argument", `a "" b`, []string{"a", "", "b"}},
		{"newline inside quotes", "\"a\nb\"", []string{"a\nb"}},
		{"tab inside quotes", "\"a\tb\"", []string{"a\tb"}},
		{"non-ASCII", `"café 日本" ü`, []string{"café 日本", "ü"}},
		{"equals sign", "env FOO=bar app", []string{"env", "FOO=bar", "app"}},
		{"percent percent", "a 100%%", []string{"a", "100%"}},
		{"percent percent inside quotes", `"100%% sure"`, []string{"100% sure"}},
		{"field code inside an argument", "app --file=%f", []string{"app", "--file=%f"}},
		{"all field codes", "a %f %F %u %U %i %c %k", []string{"a", "%f", "%F", "%u", "%U", "%i", "%c", "%k"}},
		{"deprecated field codes are removed", "a %d %D %n %N %v %m b", []string{"a", "b"}},
		{"deprecated code inside an argument is removed", "a --x=%d", []string{"a", "--x="}},
		{"deprecated code inside quotes is removed", `a "%d"`, []string{"a", ""}},
		{"escaped percent is not a deprecated code", "a %%d", []string{"a", "%d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitExec(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitExec(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestSplitExecErrors(t *testing.T) {
	tests := []struct {
		name  string
		value string
		route string
	}{
		{"unterminated quote", `app "a b`, "SplitExec->execScanner.next.UnterminatedQuote"},
		{"quote that only starts", `"`, "SplitExec->execScanner.next.UnterminatedQuote"},
		{"escaped quote at the end leaves it open", `"a\"`, "SplitExec->execScanner.next.UnterminatedQuote"},
		{"text after the closing quote", `"a b"c`, "SplitExec->execScanner.next.TextAfterQuote"},
		{"quote right after a quote", `"a""b"`, "SplitExec->execScanner.next.TextAfterQuote"},
		{"quote in the middle of an argument", `a"b c"`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"single quote unquoted", `sh -c 'x'`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"backslash unquoted", `a\b`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"less-than unquoted", `a <b`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"greater-than unquoted", `a >b`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"tilde unquoted", `a ~/b`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"pipe unquoted", `a | b`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"ampersand unquoted", `a&b`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"semicolon unquoted", `a;b`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"dollar unquoted", `a $HOME`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"asterisk unquoted", `a *`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"question mark unquoted", `a ?`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"hash unquoted", `a #b`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"parenthesis unquoted", `a (b)`, "SplitExec->execScanner.next.ReservedCharacter"},
		{"backtick unquoted", "a `b`", "SplitExec->execScanner.next.ReservedCharacter"},
		{"newline unquoted", "a\nb", "SplitExec->execScanner.next.ReservedCharacter"},
		{"unknown field code", "a %x", "SplitExec->execScanner.next->execScanner.percent.InvalidFieldCode"},
		{"unknown field code inside quotes", `a "%x"`, "SplitExec->execScanner.next->execScanner.percent.InvalidFieldCode"},
		{"lone percent at the end", "a 100%", "SplitExec->execScanner.next->execScanner.percent.InvalidFieldCode"},
		{"percent and a space", "a % b", "SplitExec->execScanner.next->execScanner.percent.InvalidFieldCode"},
		{"percent and a digit", "a %1", "SplitExec->execScanner.next->execScanner.percent.InvalidFieldCode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitExec(tt.value)
			if err == nil {
				t.Fatalf("expected an error, got %q", got)
			}
			appErr := requireRoute(t, err, tt.route)
			if appErr.Metadata["value"] != tt.value {
				t.Errorf("metadata value = %v", appErr.Metadata["value"])
			}
		})
	}
}

func TestQuoteExecArg(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{"plain", "firefox", "firefox"},
		{"path", "/usr/bin/app", "/usr/bin/app"},
		{"option with equals", "--opt=value", "--opt=value"},
		{"empty", "", `""`},
		{"space", "a b", `"a b"`},
		{"tab", "a\tb", "\"a\tb\""},
		{"newline", "a\nb", "\"a\nb\""},
		{"carriage return", "a\rb", "\"a\rb\""},
		{"double quote", `a"b`, `"a\"b"`},
		{"single quote", "a'b", `"a'b"`},
		{"dollar", "$HOME", `"\$HOME"`},
		{"backtick", "a`b", "\"a\\`b\""},
		{"backslash", `a\b`, `"a\\b"`},
		{"percent", "100%", "100%%"},
		{"percent and space", "100% sure", `"100%% sure"`},
		{"field code looking text", "%F", "%%F"},
		{"semicolon", "a;b", `"a;b"`},
		{"less-than", "a<b", `"a<b"`},
		{"greater-than", "a>b", `"a>b"`},
		{"tilde", "~/x", `"~/x"`},
		{"pipe", "a|b", `"a|b"`},
		{"ampersand", "a&b", `"a&b"`},
		{"asterisk", "*", `"*"`},
		{"question mark", "?", `"?"`},
		{"hash", "#x", `"#x"`},
		{"parentheses", "(x)", `"(x)"`},
		{"non-ASCII", "café", "café"},
		{"non-ASCII with a space", "café 日本", `"café 日本"`},
		{"equals", "A=B", "A=B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuoteExecArg(tt.arg)
			if got != tt.want {
				t.Fatalf("QuoteExecArg(%q) = %s, want %s", tt.arg, got, tt.want)
			}
			back, err := SplitExec(got)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(back, []string{tt.arg}) {
				t.Errorf("SplitExec(%s) = %q, want [%q]", got, back, tt.arg)
			}
		})
	}
}

// TestExecRoundTrip joins awkward arguments and splits them again, alone and
// together, and through the general escapes of a file.
func TestExecRoundTrip(t *testing.T) {
	special := []string{
		" ", "\t", "\n", "\r", `"`, "'", `\`, ">", "<", "~", "|", "&", ";", "$", "*", "?", "#", "(", ")", "`",
		"%", "%%", "%F", "a", "é", "日", "=", "-",
	}
	var args []string
	args = append(args, "")
	for _, a := range special {
		args = append(args, a)
		for _, b := range special {
			args = append(args, a+b, "x"+a+b+"y")
			for _, c := range special[:12] {
				args = append(args, a+b+c)
			}
		}
	}
	for _, arg := range args {
		quoted := QuoteExecArg(arg)
		got, err := SplitExec(quoted)
		if err != nil || !reflect.DeepEqual(got, []string{arg}) {
			t.Fatalf("SplitExec(QuoteExecArg(%q) = %s) = %q, %v", arg, quoted, got, err)
		}
		// Through a file: Set escapes, Parse reads, String decodes.
		e := NewEntry()
		if err := e.Group(MainGroup).Set("Exec", quoted); err != nil {
			t.Fatal(err)
		}
		back := mustParse(t, mustWrite(t, e)).Main().String("Exec")
		if back != quoted {
			t.Fatalf("Exec through a file: %q became %q", quoted, back)
		}
	}

	all := []string{"/opt/My App/bin/app", "", "a b", `say "hi"`, "it's", "$HOME", "`x`", `C:\dir`, "100%", "a;b", "x<y>z", "~", "a|b&c", "*?#", "(x)", "a\nb", "café", "日本 語"}
	value, err := JoinExec(all, "%U")
	if err != nil {
		t.Fatal(err)
	}
	got, err := SplitExec(value)
	if err != nil {
		t.Fatal(err)
	}
	if want := append(append([]string(nil), all...), "%U"); !reflect.DeepEqual(got, want) {
		t.Errorf("SplitExec(JoinExec(...)) = %q, want %q", got, want)
	}
}

func TestJoinExec(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		codes []string
		want  string
	}{
		{"nothing", nil, nil, ""},
		{"program only", []string{"/usr/bin/app"}, nil, "/usr/bin/app"},
		{"program and a field code", []string{"/usr/bin/app"}, []string{"%F"}, "/usr/bin/app %F"},
		{"quoted program with arguments", []string{"/opt/My App/app", "--flag", "a b"}, []string{"%u"}, `"/opt/My App/app" --flag "a b" %u`},
		{"field codes only", nil, []string{"%f", "%i"}, "%f %i"},
		{"percent in an argument is doubled, in a code it is not", []string{"100%"}, []string{"%U"}, "100%% %U"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JoinExec(tt.args, tt.codes...)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("JoinExec = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJoinExecErrors(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"unknown code", "%x"},
		{"deprecated code", "%d"},
		{"no percent", "F"},
		{"two percents", "%%"},
		{"code with an argument", "%F x"},
		{"empty", ""},
		{"percent only", "%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JoinExec([]string{"app"}, tt.code)
			if err == nil {
				t.Fatalf("expected an error, got %q", got)
			}
			checkRoute(t, err, "JoinExec.InvalidFieldCode")
		})
	}
}

func TestNewlineInExecArg(t *testing.T) {
	// The specification lists the line feed as a reserved character, which is
	// quoted, and the general escape \n writes it in the file.
	value, err := JoinExec([]string{"/bin/app", "a\nb"})
	if err != nil {
		t.Fatal(err)
	}
	e := NewEntry()
	if err := e.Group(MainGroup).Set("Exec", value); err != nil {
		t.Fatal(err)
	}
	text := mustWrite(t, e)
	if want := "Exec=/bin/app \"a\\nb\"\n"; !strings.Contains(text, want) {
		t.Errorf("file = %q, want it to hold %q", text, want)
	}
	args, err := SplitExec(mustParse(t, text).Main().String("Exec"))
	if err != nil || !reflect.DeepEqual(args, []string{"/bin/app", "a\nb"}) {
		t.Errorf("args = %q, %v", args, err)
	}
}
