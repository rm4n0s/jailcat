package icons

import (
	"bytes"
	"strconv"
	"strings"
)

// iniFile is a minimal reader for the key-file format of index.theme, GTK's settings.ini
// and KDE's kdeglobals: groups, key=value lines and comments, nothing else. A line it
// does not understand is skipped.
type iniFile struct {
	groups map[string]map[string]string
}

// parseINI reads data. Lines before the first group, lines without "=", and the keys of
// a malformed group header are ignored; when a key repeats in a group, the first one wins.
func parseINI(data []byte) iniFile {
	f := iniFile{groups: map[string]map[string]string{}}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	var group map[string]string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			if !strings.HasSuffix(line, "]") {
				group = nil
				continue
			}
			name := line[1 : len(line)-1]
			g, ok := f.groups[name]
			if !ok {
				g = map[string]string{}
				f.groups[name] = g
			}
			group = g
			continue
		}
		if group == nil {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		if key == "" {
			continue
		}
		if _, dup := group[key]; !dup {
			group[key] = strings.TrimSpace(line[i+1:])
		}
	}
	return f
}

// get returns the value of key in group, and whether it is there and not empty.
func (f iniFile) get(group, key string) (string, bool) {
	v, ok := f.groups[group][key]
	return v, ok && v != ""
}

// list splits the comma-separated value of key in group, dropping empty items.
func (f iniFile) list(group, key string) []string {
	v, _ := f.get(group, key)
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// intValue returns the integer value of key in group, and whether it is there and valid.
func (f iniFile) intValue(group, key string) (int, bool) {
	v, ok := f.get(group, key)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}
