package icons

import (
	"reflect"
	"testing"
)

func TestParseINI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]map[string]string
	}{
		{"empty", "", map[string]map[string]string{}},
		{
			"groups, spaces and comments",
			"# top\n[A]\nk = v \n; semicolon\n\n  [B]  \nx=1=2\n",
			map[string]map[string]string{"A": {"k": "v"}, "B": {"x": "1=2"}},
		},
		{"bom and CRLF", "\xef\xbb\xbf[A]\r\nk=v\r\n", map[string]map[string]string{"A": {"k": "v"}}},
		{"first key wins", "[A]\nk=1\nk=2\n", map[string]map[string]string{"A": {"k": "1"}}},
		{"group repeated is merged", "[A]\na=1\n[B]\nb=2\n[A]\nc=3\n", map[string]map[string]string{"A": {"a": "1", "c": "3"}, "B": {"b": "2"}}},
		{"keys before a group are dropped", "k=v\n[A]\nx=y\n", map[string]map[string]string{"A": {"x": "y"}}},
		{"junk lines are skipped", "[A]\nno equals\n=novalue\n  =  \nk=v\n", map[string]map[string]string{"A": {"k": "v"}}},
		{"malformed header drops its keys", "[A]\na=1\n[B\nb=2\n[C]\nc=3\n", map[string]map[string]string{"A": {"a": "1"}, "C": {"c": "3"}}},
		{"empty value is kept", "[A]\nk=\n", map[string]map[string]string{"A": {"k": ""}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseINI([]byte(tt.in)).groups
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseINI = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestINIAccessors(t *testing.T) {
	f := parseINI([]byte("[G]\nl = a, b,,c ,\nn=12\nbad=x1\nempty=\n"))
	if got, want := f.list("G", "l"), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("list = %q, want %q", got, want)
	}
	if got := f.list("G", "missing"); len(got) != 0 {
		t.Errorf("list of a missing key = %q", got)
	}
	if n, ok := f.intValue("G", "n"); !ok || n != 12 {
		t.Errorf("intValue(n) = %d, %v", n, ok)
	}
	for _, k := range []string{"bad", "empty", "missing"} {
		if n, ok := f.intValue("G", k); ok {
			t.Errorf("intValue(%s) = %d, want not ok", k, n)
		}
	}
	if v, ok := f.get("G", "empty"); ok {
		t.Errorf("get(empty) = %q, want not ok", v)
	}
	if _, ok := f.get("Nope", "k"); ok {
		t.Error("get in a missing group is ok")
	}
}
