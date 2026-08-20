// Copyright (c) the go-rtf authors.
// SPDX-License-Identifier: BSD-3-Clause

package rtf

import (
	"strconv"
	"strings"
)

// escapeText renders a run of text as RTF body content. The three RTF specials
// \ { } are backslash-escaped, tabs and newlines become the \tab and \line
// control words, and every non-ASCII rune is emitted as a \uN? Unicode escape
// (with a single ASCII fallback, matching the \uc1 in the header). Other
// control characters are dropped.
func escapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '{':
			b.WriteString(`\{`)
		case '}':
			b.WriteString(`\}`)
		case '\n':
			b.WriteString(`\line `)
		case '\t':
			b.WriteString(`\tab `)
		default:
			switch {
			case r == '\r':
				// dropped: carriage returns are insignificant
			case r < 0x20:
				// other control characters are dropped
			case r < 0x80:
				b.WriteRune(r)
			default:
				u := int(r)
				if u > 32767 {
					u -= 65536
				}
				b.WriteString(`\u`)
				b.WriteString(strconv.Itoa(u))
				b.WriteByte('?')
			}
		}
	}
	return b.String()
}

// escapeFieldURL escapes a URL for inclusion inside a field instruction, where
// only the RTF specials would break tokenisation.
func escapeFieldURL(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `{`, `\{`, `}`, `\}`)
	return r.Replace(s)
}
