package icons

import (
	"slices"
	"strings"
)

// maxStyleBytes and maxStyleRules bound the <style> element of a source that is read.
const (
	maxStyleBytes = 64 << 10
	maxStyleRules = 256
)

// colourProperties and opacityProperties are the only properties a <style> rule of a source may
// set (SPEC §10.2): the ones of KDE's colour-scheme icons.
var (
	colourProperties  = []string{"color", "fill", "stroke", "stop-color"}
	opacityProperties = []string{"fill-opacity", "stroke-opacity", "opacity", "stop-opacity"}
)

// cssRule is one rule ".class { prop: value; ... }" of a source's <style> element.
type cssRule struct {
	class string
	decls []attr // property and value, as presentation attributes
}

// parseStyleRules reads the text of a <style> element. It accepts only a sequence of rules
//
//	.class { property: value; property: value }
//
// where class is an identifier ([A-Za-z_][A-Za-z0-9_-]*), property is one of colourProperties
// (with a hex colour, a colour name, currentColor or none as value) or of opacityProperties
// (with a number from 0 to 1), and the last ";" of a rule is optional. This is the pattern of
// KDE's colour-scheme icons (".ColorScheme-Text { color:#232629; }"). Anything else (another
// selector, an @rule, a comment, an escape, a url(), another property or value, a rule with no
// declaration) makes the whole text unusable: ok is false and no rule is returned.
func parseStyleRules(text string) (rules []cssRule, ok bool) {
	if len(text) > maxStyleBytes || strings.ContainsAny(text, `\@`) || strings.Contains(text, "/*") {
		return nil, false
	}
	rest := strings.TrimSpace(text)
	for rest != "" {
		if rest[0] != '.' {
			return nil, false
		}
		end := 1
		for end < len(rest) && isClassChar(rest[end], end == 1) {
			end++
		}
		class := rest[1:end]
		if class == "" {
			return nil, false
		}
		rest = strings.TrimLeft(rest[end:], " \t\r\n")
		if !strings.HasPrefix(rest, "{") {
			return nil, false
		}
		closeAt := strings.IndexByte(rest, '}')
		if closeAt < 0 {
			return nil, false
		}
		rule := cssRule{class: class}
		for decl := range strings.SplitSeq(rest[1:closeAt], ";") {
			if decl = strings.TrimSpace(decl); decl == "" {
				continue
			}
			prop, value, found := strings.Cut(decl, ":")
			prop, value = asciiLower(strings.TrimSpace(prop)), strings.TrimSpace(value)
			if !found {
				return nil, false
			}
			value, valueOK := styleValue(prop, value)
			if !valueOK {
				return nil, false
			}
			rule.decls = append(rule.decls, attr{prop, value})
		}
		if len(rule.decls) == 0 || len(rules) == maxStyleRules {
			return nil, false
		}
		rules = append(rules, rule)
		rest = strings.TrimSpace(rest[closeAt+1:])
	}
	return rules, true
}

// isClassChar says whether c can be at this place of a class name.
func isClassChar(c byte, first bool) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', c == '_':
		return true
	case '0' <= c && c <= '9', c == '-':
		return !first
	}
	return false
}

// styleValue checks the value of a property of a <style> rule and returns it as it is written.
func styleValue(prop, value string) (string, bool) {
	switch {
	case slices.Contains(opacityProperties, prop):
		v, ok := parseNumber(value)
		if !ok || v < 0 || v > 1 || strings.ContainsAny(value, "eE+-") {
			return "", false
		}
		return formatNumber(v), true
	case slices.Contains(colourProperties, prop):
		switch l := asciiLower(value); {
		case l == "currentcolor":
			return "currentColor", true
		case l == "none":
			return "none", true
		case strings.HasPrefix(value, "#"):
			return value, (len(value) == 4 || len(value) == 7) && isHex(value[1:])
		default:
			return value, isLetters(value)
		}
	}
	return "", false
}

// isHex says whether s is made of hexadecimal digits only.
func isHex(s string) bool {
	return s != "" && strings.Trim(s, "0123456789abcdefABCDEF") == ""
}

// isLetters says whether s is made of ASCII letters only.
func isLetters(s string) bool {
	return s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") == ""
}

// applyRules applies the rules to n and to every element below it. A rule applies to the
// elements whose class attribute (n.class) has its class, as a presentation attribute that
// replaces the element's own attribute of that name: in the cascade a style rule beats a
// presentation attribute, and loses to the element's style attribute, which is why a property
// the style attribute sets is left alone. Rules are applied in order, so a later one wins.
func applyRules(n *node, rules []cssRule) {
	if n.name == "" {
		return
	}
	classes := strings.Fields(n.class)
	for _, rule := range rules {
		if !slices.Contains(classes, rule.class) {
			continue
		}
		for _, d := range rule.decls {
			if styleSets(n.attrs, d.name) {
				continue
			}
			if i := slices.IndexFunc(n.attrs, func(a attr) bool { return a.name == d.name }); i >= 0 {
				n.attrs[i].value = d.value
			} else {
				n.attrs = append(n.attrs, d)
			}
		}
	}
	for _, k := range n.kids {
		applyRules(k, rules)
	}
}

// styleSets says whether the style attribute among attrs sets the property prop.
func styleSets(attrs []attr, prop string) bool {
	for _, a := range attrs {
		if a.name != "style" {
			continue
		}
		for _, decl := range splitDeclarations(a.value) {
			name, _, _ := strings.Cut(decl, ":")
			if asciiLower(strings.TrimSpace(name)) == prop {
				return true
			}
		}
	}
	return false
}
