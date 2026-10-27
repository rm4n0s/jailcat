package ui

import (
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	m "github.com/rm4n0s/glutter/material"
	gui "github.com/rm4n0s/glutter/ui"

	"github.com/rm4n0s/jailcat/internal/firejail"
)

// tabPage opens the editor of a new build of the app (the Default profile, or the app's
// own when it has one) on a tab, in a window tall enough to show the whole tab.
func tabPage(t *testing.T, app string, tab firejail.Tab) *page {
	t.Helper()
	backend := fixtureBackend(t)
	backend.set(func(f *fakeBackend) { f.builds = nil })
	p := newPage(t, backend)
	p.ts.SetViewSize(gui.Size{Width: 1100, Height: 2400})
	p.ready()
	p.openApp(app)
	p.selectTab(tab)
	return p
}

// reset presses Reset to profile on the mark of the key.
func (p *page) reset(markKey string) {
	p.t.Helper()
	mark := key(markKey)
	if !p.ts.Exists(mark) {
		p.t.Fatalf("no mark %s", markKey)
	}
	p.ts.Tap(gluttertest.Descendant(mark, gluttertest.ByText(resetText)))
	p.ts.Pump()
}

// pressable reports whether the button with the key does something when it is tapped.
func (p *page) pressable(k string) bool {
	p.t.Helper()
	w := p.ts.Widget(key(k))
	b, ok := w.(interface{ OnPressed() func() })
	if !ok {
		p.t.Fatalf("%s is a %T, not a button", k, w)
	}
	return b.OnPressed() != nil
}

// textOf is the text of the field with the key.
func (p *page) textOf(k string) string {
	p.t.Helper()
	return p.ts.Widget(key(k)).(*m.TextField).Controller().Text()
}

// checked reports whether the checkbox tile with the key is drawn checked.
func (p *page) checked(k string) bool {
	p.t.Helper()
	box := p.ts.Widget(gluttertest.Descendant(key(k), gluttertest.ByType[*m.Checkbox]())).(*m.Checkbox)
	return box.Value().OrElse(false)
}

// switchedOn reports whether the switch tile with the key is drawn on.
func (p *page) switchedOn(k string) bool {
	p.t.Helper()
	return p.ts.Widget(gluttertest.Descendant(key(k), gluttertest.ByType[*m.Switch]())).(*m.Switch).Value()
}
