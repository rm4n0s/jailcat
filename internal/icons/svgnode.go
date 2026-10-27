package icons

import "strings"

// attr is one attribute of a node. name is written as it is (for example "xlink:href").
type attr struct {
	name  string
	value string
}

// node is an element or a piece of text of the SVG Jailcat writes. A node whose name is
// empty is text and holds text; any other node is an element with attrs and kids.
type node struct {
	name  string
	attrs []attr
	kids  []*node
	text  string
	// class is the class attribute of a source element. It is not written: parseSVG applies
	// the rules of the source's <style> to it and drops it.
	class string
}

// isTextElement says whether name is an element whose children are written on the same
// line, without indentation, because whitespace is part of their content.
func isTextElement(name string) bool {
	return name == "text" || name == "tspan" || name == "textPath"
}

// element makes an element node from name and the alternating names and values of attrs.
func element(name string, attrs ...string) *node {
	n := &node{name: name}
	for i := 0; i+1 < len(attrs); i += 2 {
		n.attrs = append(n.attrs, attr{attrs[i], attrs[i+1]})
	}
	return n
}

// add appends kids to n and returns n.
func (n *node) add(kids ...*node) *node {
	n.kids = append(n.kids, kids...)
	return n
}

// write appends n to b as text, two spaces of indentation per depth. With inline the node
// is written without indentation or line breaks (inside a text element).
func (n *node) write(b *strings.Builder, depth int, inline bool) {
	if n.name == "" {
		escapeText(b, n.text)
		return
	}
	if !inline {
		b.WriteString(strings.Repeat("  ", depth))
	}
	b.WriteByte('<')
	b.WriteString(n.name)
	for _, a := range n.attrs {
		b.WriteByte(' ')
		b.WriteString(a.name)
		b.WriteString(`="`)
		escapeAttr(b, a.value)
		b.WriteByte('"')
	}
	if len(n.kids) == 0 {
		b.WriteString("/>")
	} else {
		b.WriteByte('>')
		kidsInline := inline || isTextElement(n.name)
		if !kidsInline {
			b.WriteByte('\n')
		}
		for _, k := range n.kids {
			k.write(b, depth+1, kidsInline)
		}
		if !kidsInline {
			b.WriteString(strings.Repeat("  ", depth))
		}
		b.WriteString("</")
		b.WriteString(n.name)
		b.WriteByte('>')
	}
	if !inline {
		b.WriteByte('\n')
	}
}

// escapeAttr writes v as the content of a double-quoted attribute. Tabs and line breaks
// become spaces, which is what an XML parser makes of them anyway.
func escapeAttr(b *strings.Builder, v string) {
	for _, r := range v {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\t', '\n', '\r':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
}

// escapeText writes v as character data.
func escapeText(b *strings.Builder, v string) {
	for _, r := range v {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\r':
			b.WriteString("&#xD;")
		default:
			b.WriteRune(r)
		}
	}
}
