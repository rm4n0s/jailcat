package icons

import (
	"encoding/xml"
	"slices"
	"testing"
)

func TestRewriteURLs(t *testing.T) {
	tests := []struct {
		name, in, want string
		ok             bool
	}{
		{"plain", "red", "red", true},
		{"id", "url(#a)", "url(#p-a)", true},
		{"id with fallback", "url(#a) red", "url(#p-a) red", true},
		{"quoted and spaced", `url( "#a" )`, "url(#p-a)", true},
		{"upper case", "URL(#a)", "url(#p-a)", true},
		{"two", "url(#a) url('#b')", "url(#p-a) url(#p-b)", true},
		{"png data", "url(data:image/png;base64,AAAA)", "url(data:image/png;base64,AAAA)", true},
		{"html data", "url(data:text/html;base64,AAAA)", "", false},
		{"file", "url(file:///etc/passwd)", "", false},
		{"http", "url(http://example.com/a.svg#a)", "", false},
		{"relative", "url(a.svg#a)", "", false},
		{"id of another file", "url(a.svg#a)", "", false},
		{"unterminated", "url(#a", "", false},
		{"empty id", "url(#)", "", false},
		{"id with quote", `url("#a'b")`, "", false},
		{"second is bad", "url(#a) url(//x)", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := rewriteURLs(tt.in, "p-")
			if ok != tt.ok || got != tt.want {
				t.Errorf("rewriteURLs(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestCleanStyle(t *testing.T) {
	tests := []struct {
		name, in, want string
		ok             bool
	}{
		{"plain", "fill:red;stroke:blue", "fill:red; stroke:blue", true},
		{"id", "fill:url(#a)", "fill:url(#p-a)", true},
		{"bad declaration dropped", "fill:url(http://x/y); stroke:blue", "stroke:blue", true},
		{"data uri keeps its semicolon", "fill:url(data:image/png;base64,AAAA);stroke:red", "fill:url(data:image/png;base64,AAAA); stroke:red", true},
		{"only bad", "fill:url(file:///x)", "", false},
		{"escape", `fill:\75rl(http://x)`, "", false},
		{"comment", "fill:red/* x */", "", false},
		{"empty", " ; ", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cleanStyle(tt.in, "p-")
			if ok != tt.ok || got != tt.want {
				t.Errorf("cleanStyle(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestCleanHref(t *testing.T) {
	tests := []struct {
		name, in, want string
		ok             bool
	}{
		{"id", "#a", "#p-a", true},
		{"id spaced", "  #a ", "#p-a", true},
		{"png", "data:image/png;base64,AA AA\nBB", "data:image/png;base64,AAAABB", true},
		{"jpeg", "data:image/jpeg;base64,AAAA", "data:image/jpeg;base64,AAAA", true},
		{"svg", "DATA:image/svg+xml;utf8,x", "DATA:image/svg+xml;utf8,x", true},
		{"gif", "data:image/gif;base64,AAAA", "", false},
		{"text", "data:text/plain,hi", "", false},
		{"bare data", "data:image/png", "", false},
		{"javascript", "javascript:alert(1)", "", false},
		{"file", "file:///etc/passwd", "", false},
		{"path", "/etc/passwd", "", false},
		{"other file's id", "other.svg#a", "", false},
		{"empty id", "#", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cleanHref(tt.in, "p-")
			if ok != tt.ok || got != tt.want {
				t.Errorf("cleanHref(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSourceViewBox(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]string
		want  string
		ok    bool
	}{
		{"view box", map[string]string{"viewBox": "0 0 24 24"}, "0 0 24 24", true},
		{"commas", map[string]string{"viewBox": "0,0,24.5,12"}, "0 0 24.5 12", true},
		{"view box wins", map[string]string{"viewBox": "0 0 10 10", "width": "99", "height": "99"}, "0 0 10 10", true},
		{"pixels", map[string]string{"width": "32px", "height": "24px"}, "0 0 32 24", true},
		{"no unit", map[string]string{"width": "32", "height": "24"}, "0 0 32 24", true},
		{"points", map[string]string{"width": "48pt", "height": "24pt"}, "0 0 64 32", true},
		{"inches", map[string]string{"width": "1in", "height": "0.5in"}, "0 0 96 48", true},
		{"zero view box falls back", map[string]string{"viewBox": "0 0 0 0", "width": "8", "height": "8"}, "0 0 8 8", true},
		{"bad view box falls back", map[string]string{"viewBox": "a b c d", "width": "8", "height": "8"}, "0 0 8 8", true},
		{"three numbers", map[string]string{"viewBox": "0 0 8"}, "", false},
		{"percent", map[string]string{"width": "100%", "height": "100%"}, "", false},
		{"em", map[string]string{"width": "2em", "height": "2em"}, "", false},
		{"only width", map[string]string{"width": "8"}, "", false},
		{"negative", map[string]string{"width": "-8", "height": "8"}, "", false},
		{"infinity", map[string]string{"width": "Inf", "height": "8"}, "", false},
		{"nothing", map[string]string{}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attrs []xml.Attr
			for k, v := range tt.attrs {
				attrs = append(attrs, xml.Attr{Name: xml.Name{Local: k}, Value: v})
			}
			got, ok := sourceViewBox(attrs)
			if ok != tt.ok || got != tt.want {
				t.Errorf("sourceViewBox(%v) = %q, %v; want %q, %v", tt.attrs, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseStyleRules(t *testing.T) {
	tests := []struct {
		name, in string
		want     []cssRule
		ok       bool
	}{
		{"empty", "  \n ", nil, true},
		{"breeze", "\n .ColorScheme-Text {\n color:#232629;\n }\n", []cssRule{{"ColorScheme-Text", []attr{{"color", "#232629"}}}}, true},
		{"two rules, no last semicolon", ".a{fill:#fff}.b { stroke : Red ; opacity: 0.5 }",
			[]cssRule{{"a", []attr{{"fill", "#fff"}}}, {"b", []attr{{"stroke", "Red"}, {"opacity", "0.5"}}}}, true},
		{"current colour", ".a { FILL: CURRENTCOLOR; stop-color: none }", []cssRule{{"a", []attr{{"fill", "currentColor"}, {"stop-color", "none"}}}}, true},
		{"opacity bounds", ".a { opacity: 0; fill-opacity: 1; stroke-opacity: .25; stop-opacity: 1.0 }",
			[]cssRule{{"a", []attr{{"opacity", "0"}, {"fill-opacity", "1"}, {"stroke-opacity", "0.25"}, {"stop-opacity", "1"}}}}, true},
		{"underscore class", "._a-1 { color: red }", []cssRule{{"_a-1", []attr{{"color", "red"}}}}, true},
		{"opacity 1.5", ".a { opacity: 1.5 }", nil, false},
		{"opacity negative", ".a { opacity: -0.5 }", nil, false},
		{"opacity exponent", ".a { opacity: 1e-1 }", nil, false},
		{"percent", ".a { opacity: 50% }", nil, false},
		{"hex 5", ".a { fill: #12345 }", nil, false},
		{"hex 8", ".a { fill: #12345678 }", nil, false},
		{"unknown property", ".a { display: none }", nil, false},
		{"at", "@charset \"x\"; .a { fill: red }", nil, false},
		{"comment", "/* c */ .a { fill: red }", nil, false},
		{"escape", ".a { fill: r\\65 d }", nil, false},
		{"id", "#a { fill: red }", nil, false},
		{"empty rule", ".a {}", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseStyleRules(tt.in)
			if ok != tt.ok || !slices.EqualFunc(got, tt.want, func(a, b cssRule) bool {
				return a.class == b.class && slices.Equal(a.decls, b.decls)
			}) {
				t.Errorf("parseStyleRules(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}
