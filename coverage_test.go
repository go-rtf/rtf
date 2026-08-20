// Copyright (c) the go-rtf authors.
// SPDX-License-Identifier: BSD-3-Clause

package rtf

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// paraText returns the plain text of the first block of a parsed document.
func paraText(t *testing.T, src string) string {
	t.Helper()
	return richdoc.PlainText(mustParse(t, src))
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want error
	}{
		{"empty", "", ErrEmpty},
		{"whitespace only", "\n\n", ErrNotRTF},
		{"no open brace", "hello", ErrNotRTF},
		{"only open brace", "{", ErrNotRTF},
		{"missing rtf header", `{\ansi}`, ErrNotRTF},
		{"missing close", `{\rtf1`, ErrUnbalanced},
		{"over closed", `{\rtf1}}`, ErrUnbalanced},
		{"brace at eof", `{\rtf1 {`, ErrUnbalanced},
		{"truncated backslash", `{\rtf1 x\`, ErrTruncated},
		{"truncated hex", `{\rtf1 \'e`, ErrTruncated},
		{"bad hex", `{\rtf1 \'gg\par}`, ErrBadHex},
		{"truncated fonttbl", `{\rtf1{\fonttbl`, ErrUnbalanced},
		{"truncated stylesheet", `{\rtf1{\stylesheet`, ErrUnbalanced},
		{"truncated pn", `{\rtf1{\*\pn`, ErrUnbalanced},
		{"truncated pntext", `{\rtf1{\pntext`, ErrUnbalanced},
		{"truncated listtext", `{\rtf1{\listtext`, ErrUnbalanced},
		{"truncated pict", `{\rtf1{\pict`, ErrUnbalanced},
		{"truncated unknown dest", `{\rtf1{\*\weird`, ErrUnbalanced},
		{"truncated field", `{\rtf1{\field`, ErrUnbalanced},
		{"truncated fldinst", `{\rtf1{\field{\*\fldinst HYPERLINK`, ErrUnbalanced},
		{"truncated fldrslt", `{\rtf1{\field{\*\fldinst HYPERLINK "u"}{\fldrslt R`, ErrUnbalanced},
		{"truncated field subgroup", `{\rtf1{\field{\xyz`, ErrUnbalanced},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.src))
			if !errors.Is(err, c.want) {
				t.Fatalf("Parse(%q) = %v, want %v", c.src, err, c.want)
			}
		})
	}
}

func TestControlSymbols(t *testing.T) {
	// \~ -> space, \_ -> hyphen, \- -> nothing, \\ \{ \} literal, \| ignored.
	got := paraText(t, `{\rtf1 a\~b\_c\-d\\e\{f\}g\|h\par}`)
	if want := `a b-cd\e{f}gh`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTabAndUnknownWordAndSoftline(t *testing.T) {
	if got := paraText(t, `{\rtf1 a\tab b\par}`); got != "a\tb" {
		t.Fatalf("tab: got %q", got)
	}
	// \deff0, \viewkind4 and friends are unknown-but-harmless control words.
	if got := paraText(t, `{\rtf1\deff0\viewkind4 text\par}`); got != "text" {
		t.Fatalf("unknown word: got %q", got)
	}
	d := mustParse(t, `{\rtf1 a\softline b\par}`)
	p := d.Blocks[0].(richdoc.Paragraph)
	if len(p.Inlines) != 3 {
		t.Fatalf("softline inlines = %d, want 3", len(p.Inlines))
	}
	if _, ok := p.Inlines[1].(richdoc.LineBreak); !ok {
		t.Fatalf("expected LineBreak, got %T", p.Inlines[1])
	}
}

func TestPlainResets(t *testing.T) {
	d := mustParse(t, `{\rtf1 {\b bold\plain plain}\par}`)
	want := richdoc.New().P(
		richdoc.Bold(richdoc.Txt("bold")),
		richdoc.Txt("plain"),
	).Doc()
	if richdoc.PlainText(d) != "boldplain" {
		t.Fatalf("plaintext = %q", richdoc.PlainText(d))
	}
	p := d.Blocks[0].(richdoc.Paragraph)
	wp := want.Blocks[0].(richdoc.Paragraph)
	if len(p.Inlines) != len(wp.Inlines) {
		t.Fatalf("inlines = %d, want %d", len(p.Inlines), len(wp.Inlines))
	}
}

func TestBoldOffToggle(t *testing.T) {
	// \b0 turns bold off; the run stays plain text.
	if got := paraText(t, `{\rtf1 {\b0 plain}\par}`); got != "plain" {
		t.Fatalf("got %q", got)
	}
	d := mustParse(t, `{\rtf1 {\b0 plain}\par}`)
	p := d.Blocks[0].(richdoc.Paragraph)
	if _, ok := p.Inlines[0].(richdoc.Text); !ok {
		t.Fatalf("expected Text, got %T", p.Inlines[0])
	}
}

func TestListViaIlvl(t *testing.T) {
	d := mustParse(t, `{\rtf1\pard\ls1\ilvl0 Item\par}`)
	if _, ok := d.Blocks[0].(richdoc.List); !ok {
		t.Fatalf("expected List, got %T", d.Blocks[0])
	}
}

func TestListViaListtext(t *testing.T) {
	d := mustParse(t, `{\rtf1\pard{\listtext 1.\tab}First\par}`)
	l, ok := d.Blocks[0].(richdoc.List)
	if !ok {
		t.Fatalf("expected List, got %T", d.Blocks[0])
	}
	if !l.Ordered {
		t.Fatalf("digit marker should mark ordered list")
	}
}

func TestEmptyParagraphSkipped(t *testing.T) {
	d := mustParse(t, `{\rtf1\par x\par}`)
	if len(d.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1 (empty paragraph skipped)", len(d.Blocks))
	}
}

func TestHeadingClampAndStyleFallback(t *testing.T) {
	d := mustParse(t, `{\rtf1\pard\outlinelvl9 Big\par\pard\s7 Plain\par}`)
	h, ok := d.Blocks[0].(richdoc.Heading)
	if !ok || h.Level != 6 {
		t.Fatalf("expected Heading level 6, got %#v", d.Blocks[0])
	}
	if _, ok := d.Blocks[1].(richdoc.Paragraph); !ok {
		t.Fatalf("\\s7 should not be a heading, got %T", d.Blocks[1])
	}
}

func TestStyleSheetVariants(t *testing.T) {
	src := `{\rtf1{\stylesheet` +
		`{\s1\outlinelvl8 heading;}` + // outline clamps to 6
		`{\s2 heading 2;}` + // name based
		`{\s3 heading X;}` + // unparseable number
		`{\*\cs10 charstyle;}` + // char style skipped
		`{\ql no index;}` + // no \s index
		`}` +
		`\pard\s1 A\par\pard\s2 B\par}`
	d := mustParse(t, src)
	if h := d.Blocks[0].(richdoc.Heading); h.Level != 6 {
		t.Fatalf("s1 level = %d, want 6", h.Level)
	}
	if h := d.Blocks[1].(richdoc.Heading); h.Level != 2 {
		t.Fatalf("s2 level = %d, want 2", h.Level)
	}
}

func TestHexCaseAndUndefinedByte(t *testing.T) {
	// \'FF (uppercase hex) is ÿ; raw 0x81 is undefined in cp1252 and maps to U+0081.
	if got := paraText(t, `{\rtf1 \'FF\par}`); got != "ÿ" {
		t.Fatalf("uppercase hex: got %q", got)
	}
	if got := paraText(t, "{\\rtf1 \x81\\par}"); got != "" {
		t.Fatalf("undefined byte: got %q", got)
	}
}

func TestUnicodeNegativeAndBare(t *testing.T) {
	// A bare \u with no parameter contributes nothing.
	if got := paraText(t, `{\rtf1 a\u b\par}`); got != "ab" {
		t.Fatalf("bare u: got %q", got)
	}
	// Negative \u is a code point above 32767: -3913 -> 61623 (U+F0B7).
	if got := paraText(t, `{\rtf1 \u-3913?\par}`); got != "" {
		t.Fatalf("negative u: got %q", got)
	}
}

func TestUnicodeSkipClampAcrossToken(t *testing.T) {
	// \uc3 asks to skip 3 fallback runes but only 2 follow before \par; the
	// skip is clamped and does not leak into the next paragraph.
	d := mustParse(t, `{\rtf1\uc3 \u233?x\par y\par}`)
	if len(d.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(d.Blocks))
	}
	if got := richdoc.PlainText(&richdoc.Document{Blocks: d.Blocks[:1]}); got != "é" {
		t.Fatalf("first paragraph = %q, want é", got)
	}
	if got := richdoc.PlainText(&richdoc.Document{Blocks: d.Blocks[1:]}); got != "y" {
		t.Fatalf("second paragraph = %q, want y", got)
	}
}

func TestLoneDashAndNegativeParam(t *testing.T) {
	// \b followed by a bare '-' is not a parameter: the dash is literal text.
	if got := paraText(t, `{\rtf1 \b-x\par}`); got != "-x" {
		t.Fatalf("lone dash: got %q", got)
	}
	// \li-360 carries a negative parameter and is a (small) left indent.
	d := mustParse(t, `{\rtf1\pard\li-360 x\par}`)
	if _, ok := d.Blocks[0].(richdoc.Paragraph); !ok {
		t.Fatalf("negative li: got %T", d.Blocks[0])
	}
}

func TestRawNewlinesIgnored(t *testing.T) {
	if got := paraText(t, "{\\rtf1 line1\nline2\r\npart\\par}"); got != "line1line2part" {
		t.Fatalf("got %q", got)
	}
}

func TestFieldEdgeCases(t *testing.T) {
	// Hyperlink with an empty result uses the URL as its text.
	d := mustParse(t, `{\rtf1{\field{\*\fldinst HYPERLINK "u"}{\fldrslt }}\par}`)
	link := d.Blocks[0].(richdoc.Paragraph).Inlines[0].(richdoc.Link)
	if link.URL != "u" || richdoc.PlainText(&richdoc.Document{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: link.Inlines}}}) != "u" {
		t.Fatalf("empty-result link = %#v", link)
	}

	// Unquoted URL after HYPERLINK.
	d = mustParse(t, `{\rtf1{\field{\*\fldinst HYPERLINK http://x}{\fldrslt X}}\par}`)
	if link := d.Blocks[0].(richdoc.Paragraph).Inlines[0].(richdoc.Link); link.URL != "http://x" {
		t.Fatalf("unquoted url = %q", link.URL)
	}

	// Unterminated quote: not a link, preserved as raw.
	d = mustParse(t, `{\rtf1{\field{\*\fldinst HYPERLINK "oops}{\fldrslt X}}\par}`)
	if _, ok := d.Blocks[0].(richdoc.Paragraph).Inlines[0].(richdoc.RawInline); !ok {
		t.Fatalf("unterminated quote should be raw, got %T", d.Blocks[0].(richdoc.Paragraph).Inlines[0])
	}

	// Unknown inner groups and a nested group + hex inside fldinst.
	d = mustParse(t, `{\rtf1{\field{\*\fldinst SET {\i a}\'41}{\xyz junk}{\*\other z}{\fldrslt R}}\par}`)
	if _, ok := d.Blocks[0].(richdoc.Paragraph).Inlines[0].(richdoc.RawInline); !ok {
		t.Fatalf("non-hyperlink field should be raw, got %T", d.Blocks[0].(richdoc.Paragraph).Inlines[0])
	}
}

func TestStarDestinationWithoutWord(t *testing.T) {
	if got := paraText(t, `{\rtf1{\*}visible\par}`); got != "visible" {
		t.Fatalf("got %q", got)
	}
}

func TestSegMarkers(t *testing.T) {
	// Exercise the closed-set marker methods.
	(&textSeg{}).isSeg()
	breakSeg{}.isSeg()
	inlineSeg{}.isSeg()
}

func TestTrailingTextWithoutPar(t *testing.T) {
	d := mustParse(t, `{\rtf1 hello}`)
	if richdoc.PlainText(d) != "hello" || len(d.Blocks) != 1 {
		t.Fatalf("got %#v", d)
	}
}

func TestNestedGroupsInMarkers(t *testing.T) {
	// Nested groups inside {\pntext ...} and {\*\pn ...} must be consumed.
	d := mustParse(t, `{\rtf1\pard{\pntext{\f1 1}.\tab}{\*\pn\pndec{\pntxta )}}Item\par}`)
	l, ok := d.Blocks[0].(richdoc.List)
	if !ok || !l.Ordered {
		t.Fatalf("expected ordered list, got %#v", d.Blocks[0])
	}
	if richdoc.PlainText(d) != "Item" {
		t.Fatalf("plaintext = %q", richdoc.PlainText(d))
	}
}

func TestFieldStrayToken(t *testing.T) {
	// A stray control word between \field and its subgroups is skipped.
	d := mustParse(t, `{\rtf1{\field\flddirty{\*\fldinst HYPERLINK "u"}{\fldrslt X}}\par}`)
	if _, ok := d.Blocks[0].(richdoc.Paragraph).Inlines[0].(richdoc.Link); !ok {
		t.Fatalf("expected Link, got %T", d.Blocks[0].(richdoc.Paragraph).Inlines[0])
	}
}

func TestHyperlinkNoTarget(t *testing.T) {
	// HYPERLINK with nothing after it is not a usable link -> preserved as raw.
	d := mustParse(t, `{\rtf1{\field{\*\fldinst HYPERLINK}{\fldrslt X}}\par}`)
	if _, ok := d.Blocks[0].(richdoc.Paragraph).Inlines[0].(richdoc.RawInline); !ok {
		t.Fatalf("expected RawInline, got %T", d.Blocks[0].(richdoc.Paragraph).Inlines[0])
	}
}

func TestHeadingStyleNumberFallback(t *testing.T) {
	// \s3 with no stylesheet falls back to heading level 3; \s0 is body text.
	d := mustParse(t, `{\rtf1\pard\s3 A\par\pard\s0 B\par}`)
	if h, ok := d.Blocks[0].(richdoc.Heading); !ok || h.Level != 3 {
		t.Fatalf("s3 -> %#v, want Heading level 3", d.Blocks[0])
	}
	if _, ok := d.Blocks[1].(richdoc.Paragraph); !ok {
		t.Fatalf("s0 -> %T, want Paragraph", d.Blocks[1])
	}
}

func TestWriteRawListStartClamp(t *testing.T) {
	// A hand-built List with Start 0 exercises writeList's own clamp.
	d := richdoc.New().Add(richdoc.List{
		Ordered: true,
		Start:   0,
		Items:   []richdoc.ListItem{richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("x")}})},
	}).Doc()
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, out)
	if !strings.Contains(string(out), `{\pntext 1.`) {
		t.Fatalf("start not clamped to 1: %s", out)
	}
}

// --- Write-direction coverage ---

func TestWriteAllBlockTypes(t *testing.T) {
	d := richdoc.New().
		CodeBlock("go", "line1\nline2").
		MathBlock(`E=mc^2`).
		HR().
		RawBlock("rtf", `{\pard raw\par}`).
		RawBlock("latex", `\dropped`).
		Table(
			[]richdoc.Alignment{richdoc.AlignLeft},
			[]richdoc.Cell{richdoc.Td(richdoc.Txt("H1")), richdoc.Td(richdoc.Txt("H2"))},
			[][]richdoc.Cell{{richdoc.Td(richdoc.Txt("a")), richdoc.Td(richdoc.Txt("b"))}},
		).
		Doc()
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, out)
	if !strings.Contains(string(out), `raw`) {
		t.Fatalf("rtf RawBlock not emitted: %s", out)
	}
	if strings.Contains(string(out), `dropped`) {
		t.Fatalf("non-rtf RawBlock should be dropped: %s", out)
	}
}

func TestWriteAllInlineTypes(t *testing.T) {
	d := richdoc.New().P(
		richdoc.Img("http://x/i.png", "alt text", "title"),
		richdoc.InlineMath("x^2"),
		richdoc.RawI("rtf", `{\i raw}`),
		richdoc.RawI("html", "<b>dropped</b>"),
		richdoc.Href("http://x", "", richdoc.Txt("k")),
	).Doc()
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, out)
	s := string(out)
	if !strings.Contains(s, "alt text") {
		t.Fatalf("image alt not emitted: %s", s)
	}
	if strings.Contains(s, "dropped") {
		t.Fatalf("non-rtf RawInline should be dropped: %s", s)
	}
}

func TestWriteEscapesSpecials(t *testing.T) {
	d := richdoc.New().P(
		richdoc.Txt("a\\b{c}d\ttab\nnl\rcr\x01ctrl"),
		richdoc.Txt("emoji 😀 and é"),
	).Doc()
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, out)
	s := string(out)
	// 😀 is U+1F600 (128512); RTF emits it as a signed 16-bit value 128512-65536.
	for _, sub := range []string{`\\`, `\{`, `\}`, `\tab `, `\line `, `\u62976?`, `\u233?`} {
		if !strings.Contains(s, sub) {
			t.Fatalf("expected %q in output: %s", sub, s)
		}
	}
	// The control character and carriage return are dropped.
	if strings.Contains(s, "ctrl\x01") || strings.Contains(s, "\r") {
		t.Fatalf("control characters should be dropped: %q", s)
	}
}

func TestWriteListStartClampAndItemBlocks(t *testing.T) {
	d := richdoc.New().
		OList(0, true, // Start 0 clamps to 1
			richdoc.Item(
				richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("first para")}},
				richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("second para")}},
			),
			richdoc.Item(richdoc.CodeBlock{Text: "code item"}),
		).
		Doc()
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, out)
	s := string(out)
	if !strings.Contains(s, `{\pntext 1.`) {
		t.Fatalf("ordered start not clamped to 1: %s", s)
	}
	if !strings.Contains(s, "code item") {
		t.Fatalf("non-paragraph item block not flattened: %s", s)
	}
}

func TestWriteQuoteNonParagraph(t *testing.T) {
	d := richdoc.New().
		Quote(richdoc.CodeBlock{Text: "quoted code"}).
		Doc()
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, out)
	if !strings.Contains(string(out), "quoted code") {
		t.Fatalf("non-paragraph quote block not flattened: %s", out)
	}
}

func TestWriteHeadingLevelClamp(t *testing.T) {
	d := richdoc.New().
		Add(richdoc.Heading{Level: 0, Inlines: []richdoc.Inline{richdoc.Txt("lo")}}).
		Add(richdoc.Heading{Level: 9, Inlines: []richdoc.Inline{richdoc.Txt("hi")}}).
		Doc()
	out, err := Write(d)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, out)
	s := string(out)
	if !strings.Contains(s, `\s1\outlinelvl0`) || !strings.Contains(s, `\s6\outlinelvl5`) {
		t.Fatalf("heading levels not clamped to 1..6: %s", s)
	}
}

func TestWriteNilDocument(t *testing.T) {
	out, err := Write(nil)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, out)
}
