// Copyright (c) the go-rtf authors.
// SPDX-License-Identifier: BSD-3-Clause

package rtf

import "strings"

// cp1252High maps the bytes 0x80..0x9F of Windows code page 1252 to their
// Unicode code points. The five undefined positions (0x81, 0x8D, 0x8F, 0x90,
// 0x9D) hold 0 and fall back to the raw byte value.
//
// The parser decodes \'hh bytes and raw high bytes through this table as a
// best-effort default, matching the \ansicpg1252 declaration that [Write]
// emits. Other declared code pages are tracked but not transcoded.
var cp1252High = [32]rune{
	0x00: '€', // 0x80 EURO SIGN
	0x02: '‚', // 0x82 SINGLE LOW-9 QUOTATION MARK
	0x03: 'ƒ', // 0x83 LATIN SMALL LETTER F WITH HOOK
	0x04: '„', // 0x84 DOUBLE LOW-9 QUOTATION MARK
	0x05: '…', // 0x85 HORIZONTAL ELLIPSIS
	0x06: '†', // 0x86 DAGGER
	0x07: '‡', // 0x87 DOUBLE DAGGER
	0x08: 'ˆ', // 0x88 MODIFIER LETTER CIRCUMFLEX ACCENT
	0x09: '‰', // 0x89 PER MILLE SIGN
	0x0A: 'Š', // 0x8A LATIN CAPITAL LETTER S WITH CARON
	0x0B: '‹', // 0x8B SINGLE LEFT-POINTING ANGLE QUOTATION MARK
	0x0C: 'Œ', // 0x8C LATIN CAPITAL LIGATURE OE
	0x0E: 'Ž', // 0x8E LATIN CAPITAL LETTER Z WITH CARON
	0x11: '‘', // 0x91 LEFT SINGLE QUOTATION MARK
	0x12: '’', // 0x92 RIGHT SINGLE QUOTATION MARK
	0x13: '“', // 0x93 LEFT DOUBLE QUOTATION MARK
	0x14: '”', // 0x94 RIGHT DOUBLE QUOTATION MARK
	0x15: '•', // 0x95 BULLET
	0x16: '–', // 0x96 EN DASH
	0x17: '—', // 0x97 EM DASH
	0x18: '˜', // 0x98 SMALL TILDE
	0x19: '™', // 0x99 TRADE MARK SIGN
	0x1A: 'š', // 0x9A LATIN SMALL LETTER S WITH CARON
	0x1B: '›', // 0x9B SINGLE RIGHT-POINTING ANGLE QUOTATION MARK
	0x1C: 'œ', // 0x9C LATIN SMALL LIGATURE OE
	0x1E: 'ž', // 0x9E LATIN SMALL LETTER Z WITH CARON
	0x1F: 'Ÿ', // 0x9F LATIN CAPITAL LETTER Y WITH DIAERESIS
}

// decodeByte returns the Unicode rune for a single code page 1252 byte.
func decodeByte(b byte) rune {
	if b < 0x80 || b > 0x9F {
		return rune(b)
	}
	if r := cp1252High[b-0x80]; r != 0 {
		return r
	}
	return rune(b)
}

// decodeText decodes a run of code page 1252 bytes into a UTF-8 string.
func decodeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		b.WriteRune(decodeByte(s[i]))
	}
	return b.String()
}
