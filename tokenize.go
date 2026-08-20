// Copyright (c) the go-rtf authors.
// SPDX-License-Identifier: BSD-3-Clause

package rtf

// tokKind classifies a lexical token of an RTF stream.
type tokKind int

const (
	tOpen   tokKind = iota // "{"
	tClose                 // "}"
	tWord                  // control word \abc with an optional numeric parameter
	tSymbol                // control symbol \x for a single non-alphabetic x
	tText                  // a run of literal (code page 1252) bytes
	tHex                   // one \'hh byte, its value carried in ival
)

// token is a single lexical unit together with its byte span in the source,
// which lets destinations that must be preserved verbatim (pictures,
// footnotes, unrecognised fields) be recovered by slicing the original bytes.
type token struct {
	kind     tokKind
	sval     string // word/symbol/text payload
	ival     int    // numeric parameter (tWord) or byte value (tHex)
	hasParam bool   // whether a tWord carried an explicit parameter
	start    int    // inclusive byte offset of the token
	end      int    // exclusive byte offset of the token
}

func isAlpha(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isHex(b byte) bool {
	return isDigit(b) || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func hexVal(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	default:
		return int(b-'A') + 10
	}
}

// tokenize splits RTF source into a flat token slice. Raw carriage returns and
// line feeds are insignificant in RTF and are dropped. It reports [ErrTruncated]
// for a backslash or hex escape cut off by end of input and [ErrBadHex] for a
// malformed \'hh sequence.
func tokenize(src []byte) ([]token, error) {
	var toks []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '{':
			toks = append(toks, token{kind: tOpen, sval: "{", start: i, end: i + 1})
			i++
		case c == '}':
			toks = append(toks, token{kind: tClose, sval: "}", start: i, end: i + 1})
			i++
		case c == '\\':
			if i+1 >= len(src) {
				return nil, ErrTruncated
			}
			n := src[i+1]
			switch {
			case isAlpha(n):
				j := i + 1
				for j < len(src) && isAlpha(src[j]) {
					j++
				}
				word := string(src[i+1 : j])
				hasParam, val := false, 0
				k := j
				neg := false
				if k < len(src) && src[k] == '-' {
					neg = true
					k++
				}
				ds := k
				for k < len(src) && isDigit(src[k]) {
					val = val*10 + int(src[k]-'0')
					k++
				}
				if k > ds {
					hasParam = true
					if neg {
						val = -val
					}
				} else {
					// A lone '-' is not a parameter; leave it as text.
					k = j
				}
				if k < len(src) && src[k] == ' ' {
					k++ // consume the single optional delimiting space
				}
				toks = append(toks, token{kind: tWord, sval: word, ival: val, hasParam: hasParam, start: i, end: k})
				i = k
			case n == '\'':
				if i+3 >= len(src) {
					return nil, ErrTruncated
				}
				h1, h2 := src[i+2], src[i+3]
				if !isHex(h1) || !isHex(h2) {
					return nil, ErrBadHex
				}
				toks = append(toks, token{kind: tHex, ival: hexVal(h1)*16 + hexVal(h2), start: i, end: i + 4})
				i += 4
			default:
				toks = append(toks, token{kind: tSymbol, sval: string(n), start: i, end: i + 2})
				i += 2
			}
		case c == '\r' || c == '\n':
			i++
		default:
			j := i
			for j < len(src) {
				b := src[j]
				if b == '\\' || b == '{' || b == '}' || b == '\r' || b == '\n' {
					break
				}
				j++
			}
			toks = append(toks, token{kind: tText, sval: string(src[i:j]), start: i, end: j})
			i = j
		}
	}
	return toks, nil
}
