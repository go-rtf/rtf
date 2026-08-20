// Copyright (c) the go-rtf authors.
// SPDX-License-Identifier: BSD-3-Clause

package rtf

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// mustParse parses src or fails the test.
func mustParse(t *testing.T, src string) *richdoc.Document {
	t.Helper()
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", src, err)
	}
	return d
}

// assertBalanced verifies that RTF output starts with {\rtf1, has balanced
// braces, and re-parses without error.
func assertBalanced(t *testing.T, out []byte) {
	t.Helper()
	s := string(out)
	if !strings.HasPrefix(s, `{\rtf1`) {
		t.Fatalf("output does not start with {\\rtf1: %q", s[:min(20, len(s))])
	}
	depth := 0
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		switch c {
		case '\\':
			esc = true
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				t.Fatalf("brace underflow at %d", i)
			}
		}
	}
	if depth != 0 {
		t.Fatalf("unbalanced braces: final depth %d", depth)
	}
	if _, err := Parse(out); err != nil {
		t.Fatalf("Write output does not re-parse: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestParseInlineFormatting(t *testing.T) {
	d := mustParse(t, `{\rtf1\ansi Plain {\b bold} {\i italic} {\strike struck}.\par}`)
	want := richdoc.New().P(
		richdoc.Txt("Plain "),
		richdoc.Bold(richdoc.Txt("bold")),
		richdoc.Txt(" "),
		richdoc.Italic(richdoc.Txt("italic")),
		richdoc.Txt(" "),
		richdoc.Strike(richdoc.Txt("struck")),
		richdoc.Txt("."),
	).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseNestedFormatting(t *testing.T) {
	d := mustParse(t, `{\rtf1 {\b bold {\i both}}\par}`)
	want := richdoc.New().P(
		richdoc.Bold(richdoc.Txt("bold ")),
		richdoc.Bold(richdoc.Italic(richdoc.Txt("both"))),
	).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseMonospaceIsCode(t *testing.T) {
	src := `{\rtf1{\fonttbl{\f0\froman Times;}{\f1\fmodern Courier New;}}` +
		`Say {\f1 go test} now.\par}`
	d := mustParse(t, src)
	want := richdoc.New().P(
		richdoc.Txt("Say "),
		richdoc.Mono("go test"),
		richdoc.Txt(" now."),
	).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseHeadingsFromStylesheet(t *testing.T) {
	src := `{\rtf1{\stylesheet{\s1\outlinelvl0 heading 1;}{\s2 heading 2;}}` +
		`\pard\s1 Title\par\pard\s2 Sub\par\pard Body\par}`
	d := mustParse(t, src)
	want := richdoc.New().
		H(1, richdoc.Txt("Title")).
		H(2, richdoc.Txt("Sub")).
		P(richdoc.Txt("Body")).
		Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseHeadingFromOutline(t *testing.T) {
	d := mustParse(t, `{\rtf1\pard\outlinelvl2 Deep\par}`)
	want := richdoc.New().H(3, richdoc.Txt("Deep")).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseBulletList(t *testing.T) {
	src := `{\rtf1\pard{\pntext \bullet\tab}{\*\pn\pnlvlblt}One\par` +
		`\pard{\pntext \bullet\tab}{\*\pn\pnlvlblt}Two\par}`
	d := mustParse(t, src)
	want := richdoc.New().UList(true,
		richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("One")}}),
		richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("Two")}}),
	).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseOrderedList(t *testing.T) {
	src := `{\rtf1\pard{\pntext 1.\tab}{\*\pn\pndec}First\par` +
		`\pard{\pntext 2.\tab}{\*\pn\pndec}Second\par}`
	d := mustParse(t, src)
	want := richdoc.New().OList(1, true,
		richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("First")}}),
		richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("Second")}}),
	).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseHyperlink(t *testing.T) {
	src := `{\rtf1 See {\field{\*\fldinst HYPERLINK "https://example.com"}{\fldrslt the site}} now.\par}`
	d := mustParse(t, src)
	want := richdoc.New().P(
		richdoc.Txt("See "),
		richdoc.Href("https://example.com", "", richdoc.Txt("the site")),
		richdoc.Txt(" now."),
	).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseLineBreak(t *testing.T) {
	d := mustParse(t, `{\rtf1 a\line b\par}`)
	want := richdoc.New().P(richdoc.Txt("a"), richdoc.Br(), richdoc.Txt("b")).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseUnicodeAndHex(t *testing.T) {
	// \u233? is é (the ? is the skipped fallback char); \'e9 is é in cp1252;
	// the raw byte 0x80 is the euro sign in cp1252.
	d := mustParse(t, "{\\rtf1 caf\\u233? and \\'e9 and \x80\\par}")
	want := richdoc.New().P(richdoc.Txt("café and é and €")).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseUnicodeSkipMultiple(t *testing.T) {
	// \uc2 makes each \uN skip two fallback bytes.
	d := mustParse(t, `{\rtf1\uc2 \u233??x\par}`)
	want := richdoc.New().P(richdoc.Txt("éx")).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseBlockQuote(t *testing.T) {
	d := mustParse(t, `{\rtf1\pard\li720 Quoted line\par\pard Normal\par}`)
	want := richdoc.New().
		Quote(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("Quoted line")}}).
		P(richdoc.Txt("Normal")).
		Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseThematicBreak(t *testing.T) {
	d := mustParse(t, `{\rtf1 Above\par\pard\brdrb\brdrs \par Below\par}`)
	want := richdoc.New().
		P(richdoc.Txt("Above")).
		HR().
		P(richdoc.Txt("Below")).
		Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParsePictAndFootnoteAndUnknownFieldAsRaw(t *testing.T) {
	src := `{\rtf1 img{\pict\wmetafile8 0102}fn{\footnote note}` +
		`fld{\field{\*\fldinst TIME}{\fldrslt 12:00}}\par}`
	d := mustParse(t, src)
	p, ok := d.Blocks[0].(richdoc.Paragraph)
	if !ok {
		t.Fatalf("expected paragraph, got %T", d.Blocks[0])
	}
	var raws []richdoc.RawInline
	for _, in := range p.Inlines {
		if r, ok := in.(richdoc.RawInline); ok {
			raws = append(raws, r)
		}
	}
	if len(raws) != 3 {
		t.Fatalf("expected 3 RawInline (pict, footnote, field), got %d: %#v", len(raws), p.Inlines)
	}
	for _, r := range raws {
		if r.Format != "rtf" {
			t.Fatalf("raw format = %q, want rtf", r.Format)
		}
	}
	if !strings.Contains(raws[0].Text, `\pict`) {
		t.Fatalf("pict raw = %q", raws[0].Text)
	}
	if !strings.Contains(raws[1].Text, `\footnote`) {
		t.Fatalf("footnote raw = %q", raws[1].Text)
	}
	if !strings.Contains(raws[2].Text, "TIME") {
		t.Fatalf("field raw = %q", raws[2].Text)
	}
}

func TestParseIgnoresDestinations(t *testing.T) {
	src := `{\rtf1{\colortbl;\red0\green0\blue0;}{\info{\author Me}}` +
		`{\*\themedata 00ff}Visible\par}`
	d := mustParse(t, src)
	want := richdoc.New().P(richdoc.Txt("Visible")).Doc()
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("got %#v\nwant %#v", d, want)
	}
}

func TestParseEmptyDocument(t *testing.T) {
	d := mustParse(t, `{\rtf1\ansi}`)
	if len(d.Blocks) != 0 {
		t.Fatalf("expected no blocks, got %#v", d.Blocks)
	}
}

// TestRoundTripCorpus asserts the fixed-point property
// Parse(Write(Parse(src))) == Parse(src) over a representative corpus.
func TestRoundTripCorpus(t *testing.T) {
	corpus := []string{
		`{\rtf1 Plain {\b bold} {\i italic} {\strike struck} and {\b {\i both}}.\par}`,
		`{\rtf1{\fonttbl{\f1\fmodern Courier;}}Use {\f1 code} here.\par}`,
		`{\rtf1{\stylesheet{\s1\outlinelvl0 heading 1;}}\pard\s1 Title\par\pard Body\par}`,
		`{\rtf1\pard{\pntext \bullet\tab}{\*\pn\pnlvlblt}A\par\pard{\pntext \bullet\tab}{\*\pn\pnlvlblt}B\par}`,
		`{\rtf1\pard{\pntext 1.\tab}{\*\pn\pndec}A\par\pard{\pntext 2.\tab}{\*\pn\pndec}B\par}`,
		`{\rtf1 See {\field{\*\fldinst HYPERLINK "https://go.dev"}{\fldrslt Go}}.\par}`,
		"{\\rtf1 caf\\u233? \x80 line1\\line line2\\par}",
		`{\rtf1\pard\li720 A quote\par\pard After\par}`,
		`{\rtf1 Above\par\pard\brdrb\brdrs \par Below\par}`,
	}
	for _, src := range corpus {
		first := mustParse(t, src)
		out, err := Write(first)
		if err != nil {
			t.Fatalf("Write error: %v", err)
		}
		assertBalanced(t, out)
		second, err := Parse(out)
		if err != nil {
			t.Fatalf("re-parse error for %q: %v\nRTF:\n%s", src, err, out)
		}
		if !reflect.DeepEqual(richdoc.Clone(first), richdoc.Clone(second)) {
			t.Fatalf("round-trip mismatch for %q\nfirst:  %#v\nsecond: %#v\nRTF:\n%s", src, first, second, out)
		}
	}
}

// TestRoundTripExcerpt shows one concrete Write output for the report.
func TestRoundTripExcerpt(t *testing.T) {
	d := richdoc.New().
		H(1, richdoc.Txt("Title")).
		P(richdoc.Bold(richdoc.Txt("Bold")), richdoc.Txt(" and "), richdoc.Italic(richdoc.Txt("italic"))).
		UList(true,
			richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("one")}}),
			richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("two")}}),
		).
		Doc()
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, out)
	got := mustParse(t, string(out))
	if !reflect.DeepEqual(richdoc.Clone(d), richdoc.Clone(got)) {
		t.Fatalf("excerpt round-trip mismatch\nwant %#v\ngot  %#v", d, got)
	}
	t.Logf("Write output:\n%s", out)
}
