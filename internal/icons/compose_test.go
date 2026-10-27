package icons

import (
	"bytes"
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/jailcat/assets"
)

var updateGoldens = flag.Bool("update-goldens", false, "rewrite the golden files of testdata")

// composeDir is the folder of the golden SVGs and their sources.
var composeDir = filepath.Join("testdata", "compose")

// srcIcon is the source icon testdata/compose/src/file as [Lookup] would return it.
func srcIcon(t *testing.T, file string) Icon {
	t.Helper()
	path, err := filepath.Abs(filepath.Join(composeDir, "src", file))
	if err != nil {
		t.Fatal(err)
	}
	format := Format(strings.TrimPrefix(filepath.Ext(file), "."))
	return Icon{Path: path, Format: format}
}

// goldenSVG compares an icon with testdata/compose/name.svg byte for byte (or rewrites the
// file with -update-goldens), checks that it is well-formed XML that ends with a newline,
// and that librsvg renders it. It returns the golden text.
func goldenSVG(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(composeDir, name+".svg")
	if *updateGoldens {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update-goldens to make it)", err)
	}
	if !bytes.Equal(want, got) {
		t.Errorf("%s differs from the golden file.\n--- want\n%s--- got\n%s", name, want, got)
	}
	if !bytes.HasSuffix(got, []byte("</svg>\n")) {
		t.Errorf("%s does not end with a newline", name)
	}
	dec := xml.NewDecoder(bytes.NewReader(got))
	for {
		if _, err := dec.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("%s is not well-formed XML: %v", name, err)
		}
	}
}

// renderCheck renders an SVG to a PNG with rsvg-convert (librsvg, the renderer of GTK and GNOME)
// at the given width and returns the PNG. The test is skipped when rsvg-convert is missing.
func renderCheck(t *testing.T, svg []byte, width int) []byte {
	t.Helper()
	path, err := exec.LookPath("rsvg-convert")
	if err != nil {
		t.Skip("rsvg-convert is not installed")
	}
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), path, "-f", "png", "-w", strconv.Itoa(width))
	cmd.Stdin = bytes.NewReader(svg)
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("rsvg-convert failed: %v: %s\n%s", err, stderr.String(), svg)
	}
	if stderr.Len() > 0 {
		t.Errorf("rsvg-convert warned: %s", stderr.String())
	}
	if _, err := png.Decode(bytes.NewReader(out.Bytes())); err != nil {
		t.Fatalf("rsvg-convert wrote no PNG: %v", err)
	}
	return out.Bytes()
}

// badgeIcon calls BadgeIcon and fails the test on an error.
func badgeIcon(t *testing.T, src Icon) []byte {
	t.Helper()
	got, err := BadgeIcon(t.Context(), src)
	if err != nil {
		t.Fatalf("BadgeIcon: %v", err)
	}
	return got
}

// The three random icons of the goldens and the proof image.
var randomCases = []struct {
	name   string
	seed   int64
	letter string
}{
	{"random-1", 1, "Firefox"},
	{"random-2", 2, "code"},
	{"random-3", 3, "Éditeur"},
}

func TestBadgeIconGolden(t *testing.T) {
	tests := []struct {
		golden string
		file   string
		// has are texts the icon must contain, lacks texts it must not (in any letter case).
		has   []string
		lacks []string
	}{
		{golden: "simple", file: "simple.svg",
			has: []string{`viewBox="0 0 48 48"`, `<svg x="76.8" y="76.8" width="51.2" height="51.2" viewBox="0 0 64 64">`}},
		{golden: "ids", file: "ids.svg",
			has: []string{`xlink:href="#`, `url(#`, `preserveAspectRatio="xMidYMid meet"`, `fill="none"`, "Hi ", "&amp; bye"},
			lacks: []string{"sodipodi", "inkscape", "metadata", "ignored", `id="base"`, `href="#base"`, `url(#base)`,
				`url('#`, ` href=`}},
		{golden: "hostile", file: "hostile.svg",
			has: []string{`url(#`, `stroke:#e53935`, `data:image/png;base64,iVBOR`},
			lacks: []string{"script", "alert", "onload", "onclick", "onmouseover", "foreignobject", "<style", "evil", "passwd",
				"javascript", "text/html", "<animate", "<set", "<title", "stylesheet", "doctype", "file:", `href="http`, "\\"}},
		{golden: "png", file: "pixel.png",
			has:   []string{`viewBox="0 0 16 16"`, `<image width="16" height="16" xlink:href="data:image/png;base64,iVBOR`},
			lacks: []string{"<script"}},
		{golden: "code", file: "code.svg", has: []string{`url(#s`}},
		{golden: "nowh", file: "nowh.svg", has: []string{`viewBox="0 0 64 32"`}},
		{golden: "breeze", file: "breeze.svg",
			has:   []string{`fill="currentColor" color="#232629"`, `color="#3daee9" fill-opacity="0.5"`},
			lacks: []string{"<style", "class=", "ColorScheme"}},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			got := badgeIcon(t, srcIcon(t, tt.file))
			goldenSVG(t, tt.golden, got)
			text := string(got)
			for _, s := range tt.has {
				if !strings.Contains(text, s) {
					t.Errorf("the icon lacks %q:\n%s", s, text)
				}
			}
			for _, s := range tt.lacks {
				if strings.Contains(strings.ToLower(text), strings.ToLower(s)) {
					t.Errorf("the icon has %q:\n%s", s, text)
				}
			}
			renderCheck(t, got, 128)
		})
	}
}

// idRE finds the id attributes and the references to ids of an icon.
var (
	idRE  = regexp.MustCompile(`\sid="([^"]*)"`)
	refRE = regexp.MustCompile(`(?:href="#|url\(#)([^")]*)`)
)

// TestBadgeIconIDs checks that every id of an icon has a prefix, that no two are the same, and
// that every reference points to an id of the icon: the ids of the source and the badge never
// clash, whatever the source's ids are.
func TestBadgeIconIDs(t *testing.T) {
	for _, file := range []string{"ids.svg", "hostile.svg", "code.svg"} {
		got := string(badgeIcon(t, srcIcon(t, file)))
		ids := map[string]bool{}
		for _, m := range idRE.FindAllStringSubmatch(got, -1) {
			if ids[m[1]] {
				t.Errorf("%s: id %q is used twice", file, m[1])
			}
			ids[m[1]] = true
			if !strings.HasPrefix(m[1], "jcb-") && !regexp.MustCompile(`^s[0-9a-f]{8}-`).MatchString(m[1]) {
				t.Errorf("%s: id %q has no prefix", file, m[1])
			}
		}
		for _, m := range refRE.FindAllStringSubmatch(got, -1) {
			if !ids[m[1]] {
				t.Errorf("%s: reference to %q, which is no id of the icon", file, m[1])
			}
		}
		if len(ids) == 0 {
			t.Errorf("%s: no ids at all", file)
		}
	}

	// A source whose ids are the badge's own ids must not clash with the badge.
	dir := t.TempDir()
	src := filepath.Join(dir, "clash.svg")
	const clash = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><defs><linearGradient id="shield"><stop offset="0" stop-color="red"/></linearGradient></defs><rect width="8" height="8" fill="url(#shield)"/></svg>`
	if err := os.WriteFile(src, []byte(clash), 0o600); err != nil {
		t.Fatal(err)
	}
	got := string(badgeIcon(t, Icon{Path: src, Format: FormatSVG}))
	if strings.Count(got, `id="shield"`) != 0 || !strings.Contains(got, `id="jcb-shield"`) {
		t.Errorf("the badge's id is not separate from the source's:\n%s", got)
	}
	for _, m := range idRE.FindAllStringSubmatch(got, -1) {
		if strings.Count(got, `id="`+m[1]+`"`) != 1 {
			t.Errorf("id %q is used twice", m[1])
		}
	}
}

func TestBadgeIconSourceIDsFollowTheSource(t *testing.T) {
	// The prefix comes from the bytes of the source: the same source gives the same ids, another
	// source another prefix.
	dir := t.TempDir()
	write := func(name, text string) Icon {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return Icon{Path: p, Format: FormatSVG}
	}
	const a = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><rect id="r" width="8" height="8"/></svg>`
	const b = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><rect id="r" width="8" height="9"/></svg>`
	one := string(badgeIcon(t, write("a.svg", a)))
	same := string(badgeIcon(t, write("a2.svg", a)))
	other := string(badgeIcon(t, write("b.svg", b)))
	re := regexp.MustCompile(`id="(s[0-9a-f]{8}-r)"`)
	p1, p2, p3 := re.FindStringSubmatch(one), re.FindStringSubmatch(same), re.FindStringSubmatch(other)
	if p1 == nil || p2 == nil || p3 == nil {
		t.Fatalf("no prefixed id found:\n%s", one)
	}
	if p1[1] != p2[1] || p1[1] == p3[1] {
		t.Errorf("prefixes: %q, %q, %q", p1[1], p2[1], p3[1])
	}
}

func TestBadgeIconDeterministic(t *testing.T) {
	for _, file := range []string{"ids.svg", "hostile.svg", "pixel.png"} {
		first := badgeIcon(t, srcIcon(t, file))
		second := badgeIcon(t, srcIcon(t, file))
		if !bytes.Equal(first, second) {
			t.Errorf("%s: two calls gave different bytes", file)
		}
	}
}

func TestBadgeIconNoViewBox(t *testing.T) {
	// A source with only width and height (here in pt) gets a view box in pixels.
	got := string(badgeIcon(t, srcIcon(t, "nowh.svg")))
	if !strings.Contains(got, `<svg x="0" y="0" width="128" height="128" viewBox="0 0 64 32">`) {
		t.Errorf("no synthesized view box:\n%s", got)
	}
}

func TestBadgeIconRefused(t *testing.T) {
	dir := t.TempDir()
	// A file just over the limit, and a file that has a .png name but is not a PNG.
	large := filepath.Join(dir, "large.svg")
	if err := os.WriteFile(large, bytes.Repeat([]byte(" "), maxSourceBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	fakePNG := filepath.Join(dir, "fake.png")
	if err := os.WriteFile(fakePNG, []byte("not a png"), 0o600); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(dir, "deep.svg")
	nest := strings.Repeat("<g>", maxDepth+1) + strings.Repeat("</g>", maxDepth+1)
	if err := os.WriteFile(deep, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1">`+nest+`</svg>`), 0o600); err != nil {
		t.Fatal(err)
	}
	bigPNG := filepath.Join(dir, "big.png")
	img := image.NewGray(image.Rect(0, 0, maxPNGSide+1, 1))
	var enc bytes.Buffer
	if err := png.Encode(&enc, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bigPNG, enc.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		src   Icon
		route string
	}{
		{"xpm", srcIconAt(t, filepath.Join("testdata", "fake", "abs", "ICON.XPM"), FormatXPM),
			"BadgeIcon.IconSourceUnsupported"},
		{"unknown format", Icon{Path: "/x", Format: "bmp"}, "BadgeIcon.IconSourceUnsupported"},
		{"dtd with entities", srcIcon(t, "dtd.svg"), "BadgeIcon->parseSVG.IconSourceUnsupported"},
		{"not xml", srcIcon(t, "garbage.svg"), "BadgeIcon->parseSVG.IconSourceUnsupported"},
		{"not svg", srcIcon(t, "notsvg.svg"), "BadgeIcon->parseSVG.IconSourceUnsupported"},
		{"no size", srcIcon(t, "nosize.svg"), "BadgeIcon->parseSVG.IconSourceUnsupported"},
		{"too deep", Icon{Path: deep, Format: FormatSVG}, "BadgeIcon->parseSVG.IconSourceUnsupported"},
		{"too large", Icon{Path: large, Format: FormatSVG}, "BadgeIcon->readSource.IconSourceUnsupported"},
		{"png that is none", Icon{Path: fakePNG, Format: FormatPNG}, "BadgeIcon.IconSourceUnsupported"},
		{"png too wide", Icon{Path: bigPNG, Format: FormatPNG}, "BadgeIcon.IconSourceUnsupported"},
		{"missing file", Icon{Path: filepath.Join(dir, "nothing.svg"), Format: FormatSVG}, "BadgeIcon->readSource.IconReadFailed"},
		{"a folder", Icon{Path: dir, Format: FormatPNG}, "BadgeIcon->readSource.IconReadFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BadgeIcon(t.Context(), tt.src)
			if err == nil {
				t.Fatalf("no error, got\n%s", got)
			}
			appErr := requireRoute(t, err, tt.route)
			if got != nil {
				t.Errorf("a result with the error: %s", got)
			}
			t.Logf("%s: %s", appErr.Tag, appErr.Message)
		})
	}
}

// srcIconAt returns an Icon for a path relative to the package folder.
func srcIconAt(t *testing.T, rel string, format Format) Icon {
	t.Helper()
	path, err := filepath.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}
	return Icon{Path: path, Format: format}
}

func TestBadgeIconCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := BadgeIcon(ctx, srcIcon(t, "simple.svg"))
	appErr := requireRoute(t, err, "BadgeIcon.BadgeCanceled")
	if appErr.Tag != TagBadgeCanceled || !errors.Is(err, context.Canceled) {
		t.Errorf("tag %q, cause %v", appErr.Tag, appErr.OriginalErr)
	}
}

func TestRefusedIsTagged(t *testing.T) {
	_, err := BadgeIcon(t.Context(), srcIconAt(t, filepath.Join("testdata", "fake", "abs", "ICON.XPM"), FormatXPM))
	if !errors.Is(err, &errors.Error{Tag: TagIconSourceUnsupported}) {
		t.Errorf("not matched by the sentinel: %v", err)
	}
}

func TestRandomIconGolden(t *testing.T) {
	for _, tt := range randomCases {
		t.Run(tt.name, func(t *testing.T) {
			got := RandomIcon(tt.seed, tt.letter)
			goldenSVG(t, tt.name, got)
			if !strings.Contains(string(got), `id="jcb-shield"`) {
				t.Errorf("no badge:\n%s", got)
			}
			renderCheck(t, got, 128)
		})
	}
}

func TestRandomIconVaries(t *testing.T) {
	// Over many seeds every shape and every pair of colours shows up, every icon renders, and
	// the same seed and name give the same bytes.
	shapes := map[string]bool{}
	pairs := map[string]bool{}
	gradients := map[string]bool{}
	for seed := int64(-40); seed < 200; seed++ {
		got := RandomIcon(seed, "App")
		if !bytes.Equal(got, RandomIcon(seed, "App")) {
			t.Fatalf("seed %d: two calls gave different bytes", seed)
		}
		text := string(got)
		for _, shape := range []string{"<circle", "<rect", "<polygon"} {
			if strings.Contains(strings.SplitN(text, `<svg x=`, 2)[0], shape) {
				shapes[shape] = true
			}
		}
		pairs[regexp.MustCompile(`stop-color="(#\w+)"`).FindString(text)] = true
		gradients[regexp.MustCompile(`x1="\d" y1="\d" x2="\d" y2="\d"`).FindString(text)] = true
	}
	if len(shapes) != 3 || len(pairs) != len(palette) || len(gradients) != len(gradientDirections) {
		t.Errorf("shapes %v, colours %v, gradients %v", shapes, pairs, gradients)
	}
	if bytes.Equal(RandomIcon(7, "App"), RandomIcon(8, "App")) {
		t.Error("two seeds gave the same icon")
	}
}

func TestInitial(t *testing.T) {
	tests := []struct{ name, want string }{
		{"firefox", "F"},
		{"Code", "C"},
		{"éditeur", "É"},
		{"  spaced", "S"},
		{"日本語", "日"},
		{"", "?"},
		{"   ", "?"},
		{"\x00zero", "?"},
		{"\xffbad", "?"},
		{"<tag>", "<"},
		{"&amp", "&"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := initial(tt.name); got != tt.want {
				t.Errorf("initial(%q) = %q, want %q", tt.name, got, tt.want)
			}
			svg := RandomIcon(5, tt.name)
			var found bool
			dec := xml.NewDecoder(bytes.NewReader(svg))
			var inText bool
			for {
				tok, err := dec.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("not well-formed: %v\n%s", err, svg)
				}
				switch e := tok.(type) {
				case xml.StartElement:
					inText = e.Name.Local == "text"
				case xml.EndElement:
					inText = false
				case xml.CharData:
					if inText && string(e) == tt.want {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("the letter %q is not the text of the icon:\n%s", tt.want, svg)
			}
		})
	}
}

// luminance is the relative luminance of "#rrggbb" (WCAG 2.x).
func luminance(t *testing.T, hex string) float64 {
	t.Helper()
	var c [3]float64
	for i := range c {
		v, err := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		if err != nil {
			t.Fatal(err)
		}
		f := float64(v) / 255
		if f <= 0.03928 {
			c[i] = f / 12.92
		} else {
			c[i] = math.Pow((f+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}

func TestPaletteContrast(t *testing.T) {
	seen := map[[2]string]bool{}
	for _, pair := range palette {
		if seen[pair] || pair[0] == pair[1] {
			t.Errorf("pair %v is repeated or flat", pair)
		}
		seen[pair] = true
		for _, c := range pair {
			ratio := (1 + 0.05) / (luminance(t, c) + 0.05)
			if ratio < 4.5 {
				t.Errorf("%s has contrast %.2f with white, want at least 4.5", c, ratio)
			}
		}
	}
}

func TestAssetsRender(t *testing.T) {
	// The embedded files are the ones BadgeIcon and RandomIcon trust: they must parse, be
	// SVG 1.1 without anything the sanitizer removes, and render.
	for name, tt := range map[string]struct {
		data    []byte
		viewBox string
	}{
		"badge": {assets.Badge(), "0 0 64 64"},
		"icon":  {assets.Icon(), "0 0 128 128"},
	} {
		t.Run(name, func(t *testing.T) {
			src, err := parseSVG(tt.data, "x-")
			if err != nil {
				t.Fatal(err)
			}
			if src.viewBox != tt.viewBox {
				t.Errorf("viewBox = %q, want %q", src.viewBox, tt.viewBox)
			}
			for _, bad := range []string{"<script", "<style", "<foreignObject", "<filter", " on", "http://", "url(http"} {
				if strings.Contains(string(tt.data), bad) && bad != "http://" {
					t.Errorf("contains %q", bad)
				}
			}
			renderCheck(t, tt.data, 64)
			// Changing the returned copy does not change the asset.
			tt.data[0] = 'X'
			if name == "badge" && assets.Badge()[0] == 'X' || name == "icon" && assets.Icon()[0] == 'X' {
				t.Error("the asset can be changed through the returned slice")
			}
		})
	}
}

// TestProofImages writes the proof images of step 3.1 when JAILCAT_PROOF_DIR is set (a relative
// folder is relative to the module root): the fixture icon with the badge, and the three
// random icons side by side.
func TestProofImages(t *testing.T) {
	dir := os.Getenv("JAILCAT_PROOF_DIR")
	if dir == "" {
		t.Skip("JAILCAT_PROOF_DIR is not set")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("..", "..", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	golden := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(composeDir, name+".svg"))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if err := os.WriteFile(filepath.Join(dir, "3.1-badge-code.png"), renderCheck(t, golden("code"), 128), 0o644); err != nil {
		t.Fatal(err)
	}

	const gap = 16
	var tiles []image.Image
	for _, tt := range randomCases {
		img, err := png.Decode(bytes.NewReader(renderCheck(t, golden(tt.name), 128)))
		if err != nil {
			t.Fatal(err)
		}
		tiles = append(tiles, img)
	}
	sheet := image.NewNRGBA(image.Rect(0, 0, gap+len(tiles)*(128+gap), 128+2*gap))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.NRGBA{0xee, 0xee, 0xee, 0xff}), image.Point{}, draw.Src)
	for i, img := range tiles {
		at := image.Pt(gap+i*(128+gap), gap)
		draw.Draw(sheet, image.Rectangle{Min: at, Max: at.Add(img.Bounds().Size())}, img, img.Bounds().Min, draw.Over)
	}
	var out bytes.Buffer
	if err := png.Encode(&out, sheet); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "3.1-random.png"), out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// pixelAt returns the colour of the pixel x, y of a PNG as "#rrggbb".
func pixelAt(t *testing.T, data []byte, x, y int) string {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(x, y).RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// writeSVG writes an SVG source into a temporary folder and returns it as an Icon.
func writeSVG(t *testing.T, text string) Icon {
	t.Helper()
	p := filepath.Join(t.TempDir(), "source.svg")
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return Icon{Path: p, Format: FormatSVG}
}

func TestBadgeIconColourScheme(t *testing.T) {
	// The Breeze colour-scheme pattern draws in the scheme's colours, not in black.
	got := badgeIcon(t, srcIcon(t, "breeze.svg"))
	data := renderCheck(t, got, 128) // 22 units become 128 px
	if c := pixelAt(t, data, 20, 64); c != "#232629" {
		t.Errorf("the ColorScheme-Text rectangle is %s, want #232629", c)
	}
	if c := pixelAt(t, data, 64, 40); c != "#3daee9" {
		t.Errorf("the ColorScheme-Highlight circle is %s, want #3daee9", c)
	}
}

func TestBadgeIconStylePrecedence(t *testing.T) {
	// Three rects of 4x4 units on a 12x4 view box (10.67 px per unit), all with the class a, whose
	// rule sets the fill to green. In the cascade a style rule beats the presentation attribute
	// fill="red" and loses to the style attribute (blue). The pixel colours are what librsvg draws.
	src := writeSVG(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 12 4">
  <style>.a { fill: #00ff00 }</style>
  <rect class="a" x="0" width="4" height="4" fill="#ff0000"/>
  <rect class="a" x="4" width="4" height="4" fill="#ff0000" style="fill:#0000ff"/>
  <rect class="a" x="8" width="4" height="4"/>
</svg>`)
	got := badgeIcon(t, src)
	text := string(got)
	for _, want := range []string{`fill="#00ff00"`, `style="fill:#0000ff"`} {
		if !strings.Contains(text, want) {
			t.Errorf("the icon lacks %s:\n%s", want, text)
		}
	}
	if n := strings.Count(text, `fill="#ff0000"`); n != 1 {
		t.Errorf("%d rects keep fill=#ff0000, want 1 (the one with a style attribute):\n%s", n, text)
	}
	data := renderCheck(t, got, 128)
	for i, want := range []string{"#00ff00", "#0000ff", "#00ff00"} {
		if c := pixelAt(t, data, 21+43*i, 64); c != want {
			t.Errorf("rect %d is %s, want %s", i, c, want)
		}
	}
}

func TestBadgeIconStyleBlocks(t *testing.T) {
	// Every <style> that is not the plain colour pattern is dropped as a whole, and so are
	// the class attributes: the rect keeps only its own attributes.
	tests := []struct{ name, style string }{
		{"url", `.a { fill: url(http://evil.example/x.svg#a); }`},
		{"import", `@import url(http://evil.example/e.css); .a { fill: #abcdef }`},
		{"at-rule", `@media print { .a { fill: #abcdef } }`},
		{"element selector", `rect { fill: #abcdef }`},
		{"id selector", `#r { fill: #abcdef }`},
		{"descendant", `.b .a { fill: #abcdef }`},
		{"list", `.a, .b { fill: #abcdef }`},
		{"pseudo class", `.a:hover { fill: #abcdef }`},
		{"comment", `.a { fill: #abcdef } /* x */`},
		{"escape", `.a { fill: #abcdef; stroke: \72 ed }`},
		{"unknown property", `.a { fill: #abcdef; filter: none }`},
		{"behavior", `.a { fill: #abcdef; behavior: url(x.htc) }`},
		{"bad hex", `.a { fill: #abcde }`},
		{"hex with garbage", `.a { fill: #abcdeg }`},
		{"name with digit", `.a { fill: red2 }`},
		{"important", `.a { fill: #abcdef !important }`},
		{"function", `.a { fill: rgb(1,2,3) }`},
		{"opacity out of range", `.a { fill: #abcdef; opacity: 2 }`},
		{"opacity as colour", `.a { fill: #abcdef; opacity: red }`},
		{"colour as opacity", `.a { fill: #abcdef; fill-opacity: #fff }`},
		{"empty rule", `.a { }  .b { fill: #abcdef }`},
		{"no colon", `.a { fill #abcdef }`},
		{"bad class", `.1a { fill: #abcdef }`},
		{"nested rule", `.a { .b { fill: #abcdef } }`},
		{"unclosed", `.a { fill: #abcdef`},
		{"text before", `x .a { fill: #abcdef }`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := writeSVG(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"><style>`+tt.style+
				`</style><rect id="r" class="a b" width="4" height="4" fill="#123456"/></svg>`)
			text := string(badgeIcon(t, src))
			for _, bad := range []string{"<style", "class=", "abcdef", "evil", "@", `\`} {
				if strings.Contains(text, bad) {
					t.Errorf("the icon has %q:\n%s", bad, text)
				}
			}
			if !strings.Contains(text, `fill="#123456"`) {
				t.Errorf("the rect lost its fill:\n%s", text)
			}
		})
	}

	t.Run("type other than css", func(t *testing.T) {
		src := writeSVG(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"><style type="text/javascript">.a { fill: #abcdef }</style><rect class="a" width="4" height="4"/></svg>`)
		if text := string(badgeIcon(t, src)); strings.Contains(text, "abcdef") {
			t.Errorf("the rule was applied:\n%s", text)
		}
	})
	t.Run("element inside style", func(t *testing.T) {
		src := writeSVG(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"><style>.a { fill: #abcdef }<g/></style><rect class="a" width="4" height="4"/></svg>`)
		if text := string(badgeIcon(t, src)); strings.Contains(text, "abcdef") {
			t.Errorf("the rule was applied:\n%s", text)
		}
	})
	t.Run("one bad style does not cancel a good one", func(t *testing.T) {
		src := writeSVG(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"><style>.a { fill: #abcdef }</style><style>@import url(x);</style><rect class="a" width="4" height="4"/></svg>`)
		if text := string(badgeIcon(t, src)); !strings.Contains(text, `fill="#abcdef"`) || strings.Contains(text, "<style") {
			t.Errorf("unexpected icon:\n%s", text)
		}
	})
	t.Run("a style in a foreignObject is no style", func(t *testing.T) {
		src := writeSVG(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"><foreignObject><style>.a { fill: #abcdef }</style></foreignObject><rect class="a" width="4" height="4"/></svg>`)
		if text := string(badgeIcon(t, src)); strings.Contains(text, "abcdef") {
			t.Errorf("the rule was applied:\n%s", text)
		}
	})
	t.Run("later rule wins and the root gets rules too", func(t *testing.T) {
		src := writeSVG(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4" class="r"><style>.a { fill: #111111 } .b { fill: #222222; stroke: none } .r { color: blue }</style><rect class="b a" width="4" height="4"/></svg>`)
		text := string(badgeIcon(t, src))
		if !strings.Contains(text, `fill="#222222" stroke="none"`) || !strings.Contains(text, `color="blue"`) || strings.Contains(text, "111111") {
			t.Errorf("unexpected icon:\n%s", text)
		}
	})
}
