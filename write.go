// Copyright (c) the go-rtf authors.
// SPDX-License-Identifier: BSD-3-Clause

package rtf

import (
	"strconv"
	"strings"

	"github.com/go-richdoc/richdoc"
)

// Write emits a well-formed RTF document from a [richdoc.Document]. It always
// succeeds; the error result is part of the symmetric API with [Parse]. A nil
// document produces an empty but valid RTF file.
//
// The header declares two fonts (proportional \f0, monospace \f1) and a small
// stylesheet mapping \s1..\s6 to heading levels, which [Parse] reads back to
// reconstruct headings. Font 1 also carries inline code, code blocks and math.
func Write(d *richdoc.Document) ([]byte, error) {
	var b strings.Builder
	b.WriteString("{\\rtf1\\ansi\\ansicpg1252\\deff0\\uc1\n")
	b.WriteString("{\\fonttbl{\\f0\\froman Times New Roman;}{\\f1\\fmodern Courier New;}}\n")
	b.WriteString("{\\stylesheet")
	for i := 1; i <= 6; i++ {
		b.WriteString("{\\s" + strconv.Itoa(i) + "\\outlinelvl" + strconv.Itoa(i-1) + " heading " + strconv.Itoa(i) + ";}")
	}
	b.WriteString("}\n")
	if d != nil {
		for _, blk := range d.Blocks {
			writeBlock(&b, blk)
		}
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

func writeBlock(b *strings.Builder, blk richdoc.Block) {
	switch n := blk.(type) {
	case richdoc.Heading:
		lvl := n.Level
		if lvl < 1 {
			lvl = 1
		}
		if lvl > 6 {
			lvl = 6
		}
		b.WriteString("\\pard\\s" + strconv.Itoa(lvl) + "\\outlinelvl" + strconv.Itoa(lvl-1) + " ")
		writeInlines(b, n.Inlines)
		b.WriteString("\\par\n")
	case richdoc.Paragraph:
		b.WriteString("\\pard ")
		writeInlines(b, n.Inlines)
		b.WriteString("\\par\n")
	case richdoc.List:
		writeList(b, n)
	case richdoc.BlockQuote:
		for _, cb := range n.Blocks {
			writeQuoteBlock(b, cb)
		}
	case richdoc.CodeBlock:
		b.WriteString("\\pard ")
		lines := strings.Split(n.Text, "\n")
		for i, ln := range lines {
			if i > 0 {
				b.WriteString("\\line ")
			}
			b.WriteString("{\\f1 " + escapeText(ln) + "}")
		}
		b.WriteString("\\par\n")
	case richdoc.ThematicBreak:
		b.WriteString("\\pard\\brdrb\\brdrs\\brdrw10 \\par\n")
	case richdoc.MathBlock:
		b.WriteString("\\pard{\\f1 " + escapeText(n.TeX) + "}\\par\n")
	case richdoc.Table:
		writeTable(b, n)
	case richdoc.RawBlock:
		if n.Format == "rtf" {
			b.WriteString(n.Text)
		}
	}
}

func writeInlines(b *strings.Builder, inlines []richdoc.Inline) {
	for _, in := range inlines {
		writeInline(b, in)
	}
}

func writeInline(b *strings.Builder, in richdoc.Inline) {
	switch n := in.(type) {
	case richdoc.Text:
		b.WriteString(escapeText(n.Value))
	case richdoc.Strong:
		b.WriteString("{\\b ")
		writeInlines(b, n.Inlines)
		b.WriteString("}")
	case richdoc.Emph:
		b.WriteString("{\\i ")
		writeInlines(b, n.Inlines)
		b.WriteString("}")
	case richdoc.Strikethrough:
		b.WriteString("{\\strike ")
		writeInlines(b, n.Inlines)
		b.WriteString("}")
	case richdoc.Code:
		b.WriteString("{\\f1 " + escapeText(n.Value) + "}")
	case richdoc.Link:
		b.WriteString(`{\field{\*\fldinst HYPERLINK "` + escapeFieldURL(n.URL) + `"}{\fldrslt `)
		writeInlines(b, n.Inlines)
		b.WriteString("}}")
	case richdoc.Image:
		// RTF has no URL-referenced image; preserve the alternative text.
		b.WriteString(escapeText(n.Alt))
	case richdoc.Math:
		b.WriteString("{\\f1 " + escapeText(n.TeX) + "}")
	case richdoc.LineBreak:
		b.WriteString("\\line ")
	case richdoc.RawInline:
		if n.Format == "rtf" {
			b.WriteString(n.Text)
		}
	}
}

func writeList(b *strings.Builder, l richdoc.List) {
	start := l.Start
	if start < 1 {
		start = 1
	}
	for i, it := range l.Items {
		b.WriteString("\\pard")
		if l.Ordered {
			b.WriteString("{\\pntext " + strconv.Itoa(start+i) + ".\\tab}{\\*\\pn\\pndec}")
		} else {
			b.WriteString("{\\pntext \\bullet\\tab}{\\*\\pn\\pnlvlblt}")
		}
		writeItemBlocks(b, it.Blocks)
		b.WriteString("\\par\n")
	}
}

func writeItemBlocks(b *strings.Builder, blocks []richdoc.Block) {
	for i, blk := range blocks {
		if i > 0 {
			b.WriteString("\\line ")
		}
		switch n := blk.(type) {
		case richdoc.Paragraph:
			writeInlines(b, n.Inlines)
		default:
			b.WriteString(escapeText(richdoc.PlainText(&richdoc.Document{Blocks: []richdoc.Block{blk}})))
		}
	}
}

func writeQuoteBlock(b *strings.Builder, blk richdoc.Block) {
	switch n := blk.(type) {
	case richdoc.Paragraph:
		b.WriteString("\\pard\\li720 ")
		writeInlines(b, n.Inlines)
		b.WriteString("\\par\n")
	default:
		b.WriteString("\\pard\\li720 ")
		b.WriteString(escapeText(richdoc.PlainText(&richdoc.Document{Blocks: []richdoc.Block{blk}})))
		b.WriteString("\\par\n")
	}
}

// writeTable flattens a table into tab-separated paragraphs. RTF's table model
// is not reconstructed by Parse, so this is a one-way, best-effort rendering.
func writeTable(b *strings.Builder, t richdoc.Table) {
	writeRow := func(cells []richdoc.Cell) {
		b.WriteString("\\pard ")
		for i, c := range cells {
			if i > 0 {
				b.WriteString("\\tab ")
			}
			writeInlines(b, c.Inlines)
		}
		b.WriteString("\\par\n")
	}
	if len(t.Header) > 0 {
		writeRow(t.Header)
	}
	for _, row := range t.Rows {
		writeRow(row)
	}
}
