package icons

import (
	"bytes"
	"encoding/xml"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/rm4n0s/errors"
)

// The namespaces of the attributes and elements of an SVG 1.1 document.
const (
	nsSVG   = "http://www.w3.org/2000/svg"
	nsXLink = "http://www.w3.org/1999/xlink"
	nsXML   = "http://www.w3.org/XML/1998/namespace"
)

// maxDepth is the deepest nesting of elements a source SVG may have.
const maxDepth = 128

// allowedElements are the elements of SVG 1.1 that draw, group, reference or paint, which
// both librsvg and Qt SVG render. Everything else is dropped with what is inside it:
// <script>, <foreignObject>, <style>, the animation elements, <title>, <desc>,
// <metadata> and every element of another namespace (Inkscape's, RDF's, ...).
//
// The <style> element is dropped too, but not always without a trace: reading CSS needs a CSS
// parser, so only the one pattern of KDE's colour-scheme icons is understood (about a quarter of
// the Breeze application icons; the hicolor theme has none): rules ".class { property: value }"
// of colours and opacities, see parseStyleRules. They are applied to the elements with that
// class as presentation attributes, and the class attributes are dropped. A <style> that is
// anything else (other selectors, @rules, url(), other properties) is lost as a whole, and an
// icon that depends on it may draw differently.
var allowedElements = map[string]bool{
	"a": true, "circle": true, "clipPath": true, "defs": true, "ellipse": true, "g": true,
	"image": true, "line": true, "linearGradient": true, "marker": true, "mask": true,
	"path": true, "pattern": true, "polygon": true, "polyline": true, "radialGradient": true,
	"rect": true, "stop": true, "svg": true, "switch": true, "symbol": true, "text": true,
	"textPath": true, "tspan": true, "use": true,

	"filter": true, "feBlend": true, "feColorMatrix": true, "feComponentTransfer": true,
	"feComposite": true, "feConvolveMatrix": true, "feDiffuseLighting": true,
	"feDisplacementMap": true, "feDistantLight": true, "feFlood": true, "feFuncA": true,
	"feFuncB": true, "feFuncG": true, "feFuncR": true, "feGaussianBlur": true, "feImage": true,
	"feMerge": true, "feMergeNode": true, "feMorphology": true, "feOffset": true,
	"fePointLight": true, "feSpecularLighting": true, "feSpotLight": true, "feTile": true,
	"feTurbulence": true,
}

// rootGeometry are the attributes of the source's <svg> element that compose replaces with
// its own when the source becomes a nested <svg>.
var rootGeometry = map[string]bool{
	"x": true, "y": true, "width": true, "height": true, "viewBox": true,
	"preserveAspectRatio": true, "version": true, "baseProfile": true,
}

// aspectRatioRE is a valid value of preserveAspectRatio.
var aspectRatioRE = regexp.MustCompile(`^(defer )?(none|x(Min|Mid|Max)Y(Min|Mid|Max))( (meet|slice))?$`)

// svgSource is a source SVG that has been read and sanitized, ready to become a nested <svg>.
type svgSource struct {
	// viewBox is the source's view box ("x y width height"), taken from its viewBox
	// attribute or made from its width and height.
	viewBox string
	// aspectRatio is the source's preserveAspectRatio, or "" for the default.
	aspectRatio string
	// attrs are the other attributes of the source's <svg> element (fill, stroke, ...).
	attrs []attr
	// kids are the sanitized children of the source's <svg> element.
	kids []*node
}

// nested returns the source as a nested <svg> that fills the box x, y, width, height.
func (s *svgSource) nested(x, y, width, height string) *node {
	n := element("svg", "x", x, "y", y, "width", width, "height", height, "viewBox", s.viewBox)
	if s.aspectRatio != "" {
		n.attrs = append(n.attrs, attr{"preserveAspectRatio", s.aspectRatio})
	}
	n.attrs = append(n.attrs, s.attrs...)
	n.kids = s.kids
	return n
}

// parseSVG reads the SVG document data with encoding/xml and keeps only what is safe to
// inline (SPEC §10.2):
//   - elements that are not in allowedElements, and everything inside them, are dropped
//   - the rules of a <style> element that parseStyleRules accepts are applied to the elements with
//     the class (class rules beat presentation attributes and lose to style attributes); the
//     element itself and the class attributes are dropped
//   - comments, processing instructions and a DOCTYPE without an internal subset are dropped;
//     a DOCTYPE with an internal subset (entity declarations) is refused: entity expansion
//   - attributes that are in no namespace (or xlink:href, xml:space, xml:lang) are kept, except
//     the on* event attributes and any attribute that holds a backslash
//   - every id gets prefix, and every #id reference (href, xlink:href, url(#id) in an attribute
//     or in a style attribute) is rewritten to match
//   - a reference to anything but a #id or a data: URI of a PNG, JPEG or SVG image is removed
//     (the attribute, or the declaration of a style attribute)
//
// A document that cannot be used has the tag [TagIconSourceUnsupported].
func parseSVG(data []byte, prefix string) (*svgSource, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	// dec.Entity stays nil and dec.CharsetReader stays nil: only the five predefined entities
	// and character references are known, and only UTF-8 is read.

	var (
		root     *node
		rootAttr []xml.Attr
		stack    []*node // the kept elements that are open, innermost last
		dropped  int     // how many elements are open inside a dropped element
		done     bool
		rules    []cssRule // the rules of the <style> elements read so far
		style    *styleText
	)
	for !done {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.NewErr(TagIconSourceUnsupported, err).SetMetadata("reason", "not well-formed XML")
		}
		switch t := tok.(type) {
		case xml.Directive:
			if bytes.IndexByte(t, '[') >= 0 {
				return nil, errors.New(TagIconSourceUnsupported, "the document has an internal DTD subset", "reason", "internal DTD subset")
			}
		case xml.StartElement:
			if root == nil && (t.Name.Space != nsSVG || t.Name.Local != "svg") {
				return nil, errors.New(TagIconSourceUnsupported, "the root element is not an SVG svg element", "reason", "root element is not svg", "element", t.Name.Local)
			}
			if style != nil {
				style.ok = false // an element inside <style>
			}
			if dropped == 0 && t.Name.Space == nsSVG && t.Name.Local == "style" {
				style = &styleText{ok: styleAttrsOK(t.Attr)}
			}
			if dropped > 0 || (t.Name.Space != nsSVG || !allowedElements[t.Name.Local]) {
				dropped++
				continue
			}
			if len(stack) >= maxDepth {
				return nil, errors.New(TagIconSourceUnsupported, "the document is nested too deeply", "reason", "nested too deeply", "max", maxDepth)
			}
			n := &node{name: t.Name.Local, class: classOf(t.Attr)}
			if root == nil {
				root = n
				rootAttr = slices.Clone(t.Attr)
				n.attrs = cleanAttrs(t.Attr, prefix, true)
			} else {
				n.attrs = cleanAttrs(t.Attr, prefix, false)
				parent := stack[len(stack)-1]
				parent.kids = append(parent.kids, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if dropped > 0 {
				dropped--
				if dropped == 0 && style != nil {
					if more, ok := parseStyleRules(style.text.String()); ok && style.ok {
						rules = append(rules, more...)
					}
					style = nil
				}
				continue
			}
			stack = stack[:len(stack)-1]
			done = len(stack) == 0
		case xml.CharData:
			if style != nil {
				style.text.Write(t)
				style.ok = style.ok && style.text.Len() <= maxStyleBytes
				continue
			}
			// Text matters only inside the elements that draw it; elsewhere it is formatting.
			if dropped == 0 && len(stack) > 0 && isTextElement(stack[len(stack)-1].name) {
				parent := stack[len(stack)-1]
				parent.kids = append(parent.kids, &node{text: string(t)})
			}
		}
	}
	if root == nil {
		return nil, errors.New(TagIconSourceUnsupported, "the document has no SVG element", "reason", "no svg element")
	}

	applyRules(root, rules)

	viewBox, ok := sourceViewBox(rootAttr)
	if !ok {
		return nil, errors.New(TagIconSourceUnsupported, "the document has neither a usable viewBox nor a width and height in pixels", "reason", "no size")
	}
	src := &svgSource{viewBox: viewBox, attrs: root.attrs, kids: root.kids}
	for _, a := range rootAttr {
		if a.Name.Space == "" && a.Name.Local == "preserveAspectRatio" {
			if v := strings.TrimSpace(a.Value); aspectRatioRE.MatchString(v) {
				src.aspectRatio = v
			}
		}
	}
	return src, nil
}

// sourceViewBox returns the view box of a source <svg> element: its viewBox attribute when that
// is four numbers with a positive width and height, else "0 0 width height" from its width and
// height attributes (a pixel size, or a size in pt, pc, mm, cm or in converted to pixels).
func sourceViewBox(attrs []xml.Attr) (string, bool) {
	var viewBox, width, height string
	for _, a := range attrs {
		if a.Name.Space != "" {
			continue
		}
		switch a.Name.Local {
		case "viewBox":
			viewBox = a.Value
		case "width":
			width = a.Value
		case "height":
			height = a.Value
		}
	}
	fields := strings.FieldsFunc(viewBox, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	if len(fields) == 4 {
		var v [4]float64
		ok := true
		for i, f := range fields {
			var fine bool
			v[i], fine = parseNumber(f)
			ok = ok && fine
		}
		if ok && v[2] > 0 && v[3] > 0 {
			return formatNumber(v[0]) + " " + formatNumber(v[1]) + " " + formatNumber(v[2]) + " " + formatNumber(v[3]), true
		}
	}
	w, wok := parseLength(width)
	h, hok := parseLength(height)
	if wok && hok && w > 0 && h > 0 {
		return "0 0 " + formatNumber(w) + " " + formatNumber(h), true
	}
	return "", false
}

// parseNumber parses a plain decimal number (digits, a point, an exponent and a sign).
func parseNumber(s string) (float64, bool) {
	if s == "" || strings.Trim(s, "0123456789.eE+-") != "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) || math.Abs(v) > 1e6 {
		return 0, false
	}
	return v, true
}

// pixelsPerUnit is how many pixels (1 user unit) each absolute CSS unit of a length is.
var pixelsPerUnit = map[string]float64{
	"px": 1, "pt": 4.0 / 3, "pc": 16, "mm": 96 / 25.4, "cm": 96 / 2.54, "in": 96,
}

// parseLength parses an SVG length in an absolute unit (or with none) and returns pixels.
// A relative length (em, ex, %) has no pixel size and is refused.
func parseLength(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	factor := 1.0
	if len(s) > 2 {
		if f, ok := pixelsPerUnit[asciiLower(s[len(s)-2:])]; ok {
			factor = f
			s = s[:len(s)-2]
		}
	}
	v, ok := parseNumber(strings.TrimSpace(s))
	return v * factor, ok
}

// formatNumber writes v with at most four decimals and no exponent.
func formatNumber(v float64) string {
	return strconv.FormatFloat(math.Round(v*1e4)/1e4, 'f', -1, 64)
}

// cleanAttrs returns the attributes of an element that are safe to write (see parseSVG). With
// root, the attributes compose replaces on a nested <svg> are left out.
func cleanAttrs(attrs []xml.Attr, prefix string, root bool) []attr {
	var out []attr
	for _, a := range attrs {
		var name string
		switch a.Name.Space {
		case "":
			name = a.Name.Local
			if name == "xmlns" || name == "class" {
				continue // class is read by classOf
			}
		case nsXLink:
			if a.Name.Local != "href" {
				continue
			}
			name = "xlink:href"
		case nsXML:
			if a.Name.Local != "space" && a.Name.Local != "lang" {
				continue
			}
			name = "xml:" + a.Name.Local
		default:
			continue
		}
		if root && rootGeometry[name] {
			continue
		}
		if strings.HasPrefix(asciiLower(name), "on") {
			continue
		}
		value := a.Value
		ok := true
		switch {
		case name == "id":
			ok = safeID(value)
			value = prefix + value
		case name == "href" || name == "xlink:href":
			name = "xlink:href"
			value, ok = cleanHref(value, prefix)
		case name == "style":
			value, ok = cleanStyle(value, prefix)
		case strings.Contains(value, `\`):
			ok = false
		case strings.Contains(asciiLower(value), "url("):
			value, ok = rewriteURLs(value, prefix)
		}
		if !ok || slices.ContainsFunc(out, func(o attr) bool { return o.name == name }) {
			continue
		}
		out = append(out, attr{name, value})
	}
	return out
}

// safeID says whether s can be an id that is referenced by "#s" and "url(#s)" without quoting.
func safeID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_-.:", r) {
			return false
		}
	}
	return true
}

// dataURIKinds are the data: URIs a source may keep: the images.
var dataURIKinds = []string{"data:image/png", "data:image/jpeg", "data:image/svg+xml"}

// isImageDataURI says whether s is a data: URI of a PNG, JPEG or SVG image.
func isImageDataURI(s string) bool {
	l := asciiLower(s)
	for _, k := range dataURIKinds {
		if strings.HasPrefix(l, k) && len(l) > len(k) && (l[len(k)] == ';' || l[len(k)] == ',') {
			return true
		}
	}
	return false
}

// cleanHref returns the value of an href: "#id" with the prefix, or an image data: URI without
// whitespace. Anything else (a file, a URL) is refused.
func cleanHref(v, prefix string) (string, bool) {
	t := strings.TrimSpace(v)
	switch {
	case strings.HasPrefix(t, "#") && safeID(t[1:]):
		return "#" + prefix + t[1:], true
	case isImageDataURI(t):
		return strings.Join(strings.Fields(t), ""), true
	}
	return "", false
}

// cleanStyle returns the declarations of a style attribute that are safe: a declaration with a
// url() that is not allowed is removed, and so is a whole style attribute that has CSS escapes
// or comments. ok is false when nothing is left.
func cleanStyle(v, prefix string) (string, bool) {
	if strings.Contains(v, `\`) || strings.Contains(v, "/*") {
		return "", false
	}
	var kept []string
	for _, decl := range splitDeclarations(v) {
		if decl = strings.TrimSpace(decl); decl == "" {
			continue
		}
		if r, ok := rewriteURLs(decl, prefix); ok {
			kept = append(kept, r)
		}
	}
	return strings.Join(kept, "; "), len(kept) > 0
}

// splitDeclarations splits a style attribute at the semicolons that are not inside
// parentheses or quotes (a data: URI has semicolons).
func splitDeclarations(v string) []string {
	var out []string
	depth := 0
	var quote rune
	start := 0
	for i, r := range v {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '(':
			depth++
		case r == ')':
			depth = max(depth-1, 0)
		case r == ';' && depth == 0:
			out = append(out, v[start:i])
			start = i + 1
		}
	}
	return append(out, v[start:])
}

// rewriteURLs rewrites every url(...) of v: url(#id) gets the prefix; a url() of an image data:
// URI stays. ok is false when v has any other url().
func rewriteURLs(v, prefix string) (string, bool) {
	var b strings.Builder
	rest := v
	for {
		i := indexURL(rest)
		if i < 0 {
			b.WriteString(rest)
			return b.String(), true
		}
		b.WriteString(rest[:i])
		arg, tail, ok := urlArgument(rest[i+len("url("):])
		if !ok {
			return "", false
		}
		switch {
		case strings.HasPrefix(arg, "#") && safeID(arg[1:]):
			b.WriteString("url(#" + prefix + arg[1:] + ")")
		case isImageDataURI(arg) && !strings.ContainsAny(arg, " \t\r\n\"'()\\"):
			b.WriteString("url(" + arg + ")")
		default:
			return "", false
		}
		rest = tail
	}
}

// indexURL returns the index of the first "url(" of s in any letter case, or -1.
func indexURL(s string) int {
	for i := 0; i+len("url(") <= len(s); i++ {
		if asciiLower(s[i:i+len("url(")]) == "url(" {
			return i
		}
	}
	return -1
}

// urlArgument reads the argument of a url( whose "url(" was already read from s: optional
// white space, the argument (quoted or not), optional white space and ")". It returns the
// argument and what follows the ")".
func urlArgument(s string) (arg, tail string, ok bool) {
	s = strings.TrimLeft(s, " \t\r\n")
	if s != "" && (s[0] == '"' || s[0] == '\'') {
		end := strings.IndexByte(s[1:], s[0])
		if end < 0 {
			return "", "", false
		}
		arg = s[1 : 1+end]
		s = strings.TrimLeft(s[2+end:], " \t\r\n")
		if !strings.HasPrefix(s, ")") {
			return "", "", false
		}
		return arg, s[1:], true
	}
	end := strings.IndexByte(s, ')')
	if end < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:end]), s[end+1:], true
}

// asciiLower lower-cases the ASCII letters of s and leaves every other byte alone (unlike
// strings.ToLower it never changes the length of s).
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// styleText is a <style> element of a source that is being read.
type styleText struct {
	text strings.Builder
	// ok is false when the element cannot be a plain CSS text: it has elements inside, a
	// type other than text/css, a media other than all, or too much text.
	ok bool
}

// styleAttrsOK says whether the attributes of a <style> element allow its text to be used.
func styleAttrsOK(attrs []xml.Attr) bool {
	for _, a := range attrs {
		if a.Name.Space != "" {
			continue
		}
		switch a.Name.Local {
		case "type":
			if v := asciiLower(strings.TrimSpace(a.Value)); v != "" && v != "text/css" {
				return false
			}
		case "media":
			if v := asciiLower(strings.TrimSpace(a.Value)); v != "" && v != "all" {
				return false
			}
		}
	}
	return true
}

// classOf returns the class attribute of an element.
func classOf(attrs []xml.Attr) string {
	for _, a := range attrs {
		if a.Name.Space == "" && a.Name.Local == "class" {
			return a.Value
		}
	}
	return ""
}
