// Copyright (c) the go-rtf authors.
// SPDX-License-Identifier: BSD-3-Clause

package rtf

import (
	"errors"
	"strconv"
	"strings"

	"github.com/go-richdoc/richdoc"
)

// Sentinel errors returned by [Parse]. They are wrapped-free so callers can
// compare with [errors.Is].
var (
	// ErrEmpty is returned for empty input.
	ErrEmpty = errors.New("rtf: empty input")
	// ErrNotRTF is returned when the stream does not begin with {\rtf.
	ErrNotRTF = errors.New("rtf: not an RTF document (missing {\\rtf header)")
	// ErrUnbalanced is returned for a group brace mismatch.
	ErrUnbalanced = errors.New("rtf: unbalanced group braces")
	// ErrTruncated is returned for a control sequence cut off by end of input.
	ErrTruncated = errors.New("rtf: truncated control sequence")
	// ErrBadHex is returned for a malformed \'hh escape.
	ErrBadHex = errors.New("rtf: invalid hex escape")
)

// fmtFlags captures the character-level formatting active for a text run.
type fmtFlags struct {
	bold, italic, strike, code bool
}

// state is one entry of the group stack. RTF groups scope both character and
// paragraph formatting, so a "{" duplicates the current state and a "}"
// discards it.
type state struct {
	bold, italic, strike, code bool
	fontIdx                    int // -1 when unset
	ucN                        int // \ucN Unicode fallback skip count

	// Paragraph properties, reset by \pard.
	styleIdx    int // -1 when unset
	outline     int // 0-based \outlinelvl, -1 when unset
	li          int // left indent in twips
	listItem    bool
	listOrdered bool
	hr          bool // a bottom border was requested (\brdrb)
}

func newState() state {
	return state{fontIdx: -1, ucN: 1, styleIdx: -1, outline: -1}
}

// segment is one piece of the paragraph currently being assembled.
type segment interface{ isSeg() }

type textSeg struct {
	flags fmtFlags
	text  string
}
type breakSeg struct{}
type inlineSeg struct{ inline richdoc.Inline }

// anchorStartSeg records an open {\*\bkmkstart name}. The matching
// {\*\bkmkend name} wraps the segments emitted in between into an
// [richdoc.Anchor]; an unmatched start becomes a point Anchor with no inlines.
type anchorStartSeg struct{ id string }

func (*textSeg) isSeg()       {}
func (breakSeg) isSeg()       {}
func (inlineSeg) isSeg()      {}
func (anchorStartSeg) isSeg() {}

// Parse converts a practical subset of RTF into a [richdoc.Document].
func Parse(src []byte) (*richdoc.Document, error) {
	if len(src) == 0 {
		return nil, ErrEmpty
	}
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{
		toks:         toks,
		src:          src,
		fontMono:     map[int]bool{},
		styleHeading: map[int]int{},
		states:       []state{newState()},
	}
	if len(toks) == 0 || toks[0].kind != tOpen {
		return nil, ErrNotRTF
	}
	if len(toks) < 2 || toks[1].kind != tWord || toks[1].sval != "rtf" {
		return nil, ErrNotRTF
	}
	p.i = 1
	if err := p.processGroup(); err != nil {
		return nil, err
	}
	// Trailing text with no closing \par still forms a paragraph.
	if len(p.segs) > 0 {
		p.endParagraph()
	}
	// Any leftover close brace means the document was over-closed.
	for ; p.i < len(p.toks); p.i++ {
		if p.toks[p.i].kind == tClose {
			return nil, ErrUnbalanced
		}
	}
	p.db.flushList()
	p.db.flushQuote()
	return &richdoc.Document{Blocks: p.db.blocks}, nil
}

type parser struct {
	toks []token
	src  []byte
	i    int

	states  []state
	segs    []segment
	uniSkip int

	fontMono     map[int]bool
	styleHeading map[int]int

	db docBuilder
}

func (p *parser) top() *state { return &p.states[len(p.states)-1] }

func (p *parser) pushState() { p.states = append(p.states, *p.top()) }

func (p *parser) popState() { p.states = p.states[:len(p.states)-1] }

// processGroup consumes tokens until the matching "}" (which it also consumes)
// or end of input, applying formatting and emitting content as it goes.
func (p *parser) processGroup() error {
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		switch t.kind {
		case tOpen:
			p.i++
			if err := p.dispatchGroup(p.toks[p.i-1].start); err != nil {
				return err
			}
		case tClose:
			p.i++
			return nil
		case tWord:
			p.i++
			p.applyWord(t)
		case tSymbol:
			p.i++
			p.applySymbol(t)
		case tText:
			p.i++
			p.appendText(decodeText(t.sval))
		default: // tHex
			p.i++
			p.appendText(string(decodeByte(byte(t.ival))))
		}
	}
	return ErrUnbalanced
}

// dispatchGroup is called just after a "{" has been consumed; openOff is the
// byte offset of that brace. It classifies the group by its leading token and
// routes it to a destination handler, or treats it as a nested formatting
// group.
func (p *parser) dispatchGroup(openOff int) error {
	if p.i >= len(p.toks) {
		return ErrUnbalanced
	}
	t := p.toks[p.i]
	if t.kind == tSymbol && t.sval == "*" {
		p.i++
		if p.i < len(p.toks) && p.toks[p.i].kind == tWord {
			switch p.toks[p.i].sval {
			case "pn":
				p.i++
				return p.parsePN()
			case "bkmkstart":
				p.i++
				return p.parseBookmark(true)
			case "bkmkend":
				p.i++
				return p.parseBookmark(false)
			}
		}
		return p.skipGroup()
	}
	if t.kind == tWord {
		switch t.sval {
		case "fonttbl":
			p.i++
			return p.parseFontTable()
		case "stylesheet":
			p.i++
			return p.parseStyleSheet()
		case "colortbl", "info", "header", "footer", "generator", "listtable", "listoverridetable", "revtbl":
			p.i++
			return p.skipGroup()
		case "pict":
			return p.captureRaw(openOff)
		case "footnote":
			p.i++
			return p.parseFootnote()
		case "field":
			p.i++
			return p.parseField(openOff)
		case "pntext":
			p.i++
			return p.parsePntext()
		case "listtext":
			p.i++
			return p.parseListtext()
		}
	}
	p.pushState()
	err := p.processGroup()
	p.popState()
	return err
}

// consumeToClose discards tokens until the "}" that closes the current group,
// returning that brace's end offset.
func (p *parser) consumeToClose() (int, error) {
	depth := 1
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		p.i++
		switch t.kind {
		case tOpen:
			depth++
		case tClose:
			depth--
			if depth == 0 {
				return t.end, nil
			}
		}
	}
	return 0, ErrUnbalanced
}

func (p *parser) skipGroup() error {
	_, err := p.consumeToClose()
	return err
}

// captureRaw preserves a whole group verbatim as a RawInline, used for
// pictures, which the model cannot represent losslessly.
func (p *parser) captureRaw(openOff int) error {
	end, err := p.consumeToClose()
	if err != nil {
		return err
	}
	p.segs = append(p.segs, inlineSeg{richdoc.RawInline{Format: "rtf", Text: string(p.src[openOff:end])}})
	return nil
}

// parseFootnote parses a {\footnote …} group (its leading \footnote control
// word already consumed) into an inline [richdoc.Footnote] holding the note's
// paragraphs. The note body runs on a fresh paragraph flow so it never leaks
// into the referencing paragraph; the surrounding formatting state is saved and
// restored around it.
func (p *parser) parseFootnote() error {
	savedSegs, savedDB, savedSkip := p.segs, p.db, p.uniSkip
	p.segs, p.db, p.uniSkip = nil, docBuilder{}, 0
	p.pushState()
	err := p.processGroup()
	if len(p.segs) > 0 {
		p.endParagraph()
	}
	p.popState()
	p.db.flushList()
	p.db.flushQuote()
	blocks := p.db.blocks
	p.segs, p.db, p.uniSkip = savedSegs, savedDB, savedSkip
	if err != nil {
		return err
	}
	p.segs = append(p.segs, inlineSeg{richdoc.Footnote{Blocks: blocks}})
	return nil
}

// parseBookmark handles a {\*\bkmkstart name} or {\*\bkmkend name} group (its
// leading control word already consumed). A start records an open anchor; an
// end wraps the intervening segments into an [richdoc.Anchor].
func (p *parser) parseBookmark(start bool) error {
	name, err := p.collectGroupText()
	if err != nil {
		return err
	}
	id := strings.TrimSpace(name)
	if start {
		p.segs = append(p.segs, anchorStartSeg{id: id})
		return nil
	}
	p.closeAnchor(id)
	return nil
}

// closeAnchor pairs a {\*\bkmkend id} with the nearest preceding open
// {\*\bkmkstart id}, replacing the run between them with a single Anchor. An end
// with no matching start is dropped.
func (p *parser) closeAnchor(id string) {
	for k := len(p.segs) - 1; k >= 0; k-- {
		as, ok := p.segs[k].(anchorStartSeg)
		if !ok || as.id != id {
			continue
		}
		inner := segsToInlines(p.segs[k+1:])
		p.segs = append(p.segs[:k], inlineSeg{richdoc.Anchor{ID: id, Inlines: inner}})
		return
	}
}

// parsePN reads a {\*\pn ...} paragraph-numbering definition, marking the
// current paragraph as a list item and recording whether it is ordered.
func (p *parser) parsePN() error {
	st := p.top()
	st.listItem = true
	depth := 1
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		p.i++
		switch t.kind {
		case tOpen:
			depth++
		case tClose:
			depth--
			if depth == 0 {
				return nil
			}
		case tWord:
			switch t.sval {
			case "pndec", "pncard", "pnord", "pnucltr", "pnlcltr", "pnucrm", "pnlcrm":
				st.listOrdered = true
			case "pnlvlblt":
				st.listOrdered = false
			}
		}
	}
	return ErrUnbalanced
}

// parsePntext / parseListtext handle the bullet-or-number marker groups. A
// digit in the marker text is taken as evidence of an ordered list.
func (p *parser) parsePntext() error   { return p.parseMarker() }
func (p *parser) parseListtext() error { return p.parseMarker() }

func (p *parser) parseMarker() error {
	st := p.top()
	st.listItem = true
	depth := 1
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		p.i++
		switch t.kind {
		case tOpen:
			depth++
		case tClose:
			depth--
			if depth == 0 {
				return nil
			}
		case tText:
			if strings.ContainsAny(t.sval, "0123456789") {
				st.listOrdered = true
			}
		}
	}
	return ErrUnbalanced
}

// parseFontTable records which font indices are monospace so that runs in
// those fonts become inline code.
func (p *parser) parseFontTable() error {
	depth := 1
	cur, mono := -1, false
	finish := func() {
		if cur >= 0 {
			p.fontMono[cur] = mono
		}
		cur, mono = -1, false
	}
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		p.i++
		switch t.kind {
		case tOpen:
			depth++
		case tClose:
			depth--
			if depth == 1 {
				finish()
			}
			if depth == 0 {
				finish()
				return nil
			}
		case tWord:
			switch t.sval {
			case "f":
				if t.hasParam {
					cur = t.ival
				}
			case "fmodern":
				mono = true
			}
		case tText:
			l := strings.ToLower(t.sval)
			if strings.Contains(l, "courier") || strings.Contains(l, "mono") || strings.Contains(l, "consol") {
				mono = true
			}
			if strings.Contains(t.sval, ";") {
				finish()
			}
		}
	}
	return ErrUnbalanced
}

// parseStyleSheet maps style indices to heading levels, resolving both
// \outlinelvl and "heading N" style names.
func (p *parser) parseStyleSheet() error {
	depth := 1
	idx, outline, isChar := -1, -1, false
	var name strings.Builder
	reset := func() { idx, outline, isChar = -1, -1, false; name.Reset() }
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		p.i++
		switch t.kind {
		case tOpen:
			depth++
			if depth == 2 {
				reset()
			}
		case tClose:
			depth--
			if depth == 1 {
				p.recordStyle(idx, outline, name.String(), isChar)
			}
			if depth == 0 {
				return nil
			}
		case tSymbol:
			if t.sval == "*" {
				isChar = true
			}
		case tWord:
			switch t.sval {
			case "s":
				if t.hasParam {
					idx = t.ival
				}
			case "cs", "ds":
				isChar = true
			case "outlinelvl":
				if t.hasParam {
					outline = t.ival
				}
			}
		case tText:
			name.WriteString(t.sval)
		}
	}
	return ErrUnbalanced
}

func (p *parser) recordStyle(idx, outline int, name string, isChar bool) {
	if idx < 0 || isChar {
		return
	}
	h := 0
	if outline >= 0 {
		h = outline + 1
	} else {
		n := strings.ToLower(strings.Trim(strings.TrimSpace(name), " ;"))
		if rest, ok := strings.CutPrefix(n, "heading "); ok {
			if d, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil {
				h = d
			}
		}
	}
	if h >= 1 {
		if h > 6 {
			h = 6
		}
		p.styleHeading[idx] = h
	}
}

// parseField handles {\field{\*\fldinst INSTRUCTION …}{\fldrslt text}}. A
// HYPERLINK instruction becomes a Link and a REF/PAGEREF instruction a
// RefLabel CrossRef pointing at the bookmark; any other field is preserved
// verbatim.
func (p *parser) parseField(openOff int) error {
	var url, refTarget string
	hasLink, hasRef := false, false
	var result []richdoc.Inline
	depth := 1
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		if t.kind == tOpen {
			p.i++
			depth++
			kind := p.fieldSubKind()
			switch kind {
			case "fldinst":
				inst, err := p.collectGroupText()
				if err != nil {
					return err
				}
				depth--
				if u, ok := parseHyperlink(inst); ok {
					url, hasLink = u, true
				} else if tgt, ok := parseRef(inst); ok {
					refTarget, hasRef = tgt, true
				}
			case "fldrslt":
				ins, err := p.collectInlines()
				if err != nil {
					return err
				}
				depth--
				result = ins
			default:
				if err := p.skipGroup(); err != nil {
					return err
				}
				depth--
			}
			continue
		}
		p.i++
		if t.kind == tClose {
			depth--
			if depth == 0 {
				break
			}
		}
	}
	if depth != 0 {
		return ErrUnbalanced
	}
	switch {
	case hasLink:
		if len(result) == 0 {
			result = []richdoc.Inline{richdoc.Text{Value: url}}
		}
		p.segs = append(p.segs, inlineSeg{richdoc.Link{URL: url, Inlines: result}})
	case hasRef:
		if len(result) == 0 {
			result = []richdoc.Inline{richdoc.Text{Value: refTarget}}
		}
		p.segs = append(p.segs, inlineSeg{richdoc.CrossRef{Target: refTarget, Kind: richdoc.RefLabel, Inlines: result}})
	default:
		p.segs = append(p.segs, inlineSeg{richdoc.RawInline{Format: "rtf", Text: string(p.src[openOff:p.toks[p.i-1].end])}})
	}
	return nil
}

// fieldSubKind identifies a field child group whose "{" has just been
// consumed, advancing past its leading \*, \fldinst or \fldrslt marker.
func (p *parser) fieldSubKind() string {
	if p.i < len(p.toks) && p.toks[p.i].kind == tSymbol && p.toks[p.i].sval == "*" {
		p.i++
	}
	if p.i < len(p.toks) && p.toks[p.i].kind == tWord {
		switch p.toks[p.i].sval {
		case "fldinst":
			p.i++
			return "fldinst"
		case "fldrslt":
			p.i++
			return "fldrslt"
		}
	}
	return ""
}

// collectGroupText returns the literal text of the current group, consuming it
// up to and including its "}".
func (p *parser) collectGroupText() (string, error) {
	var b strings.Builder
	depth := 1
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		p.i++
		switch t.kind {
		case tOpen:
			depth++
		case tClose:
			depth--
			if depth == 0 {
				return b.String(), nil
			}
		case tText:
			b.WriteString(t.sval)
		case tHex:
			b.WriteByte(byte(t.ival))
		}
	}
	return "", ErrUnbalanced
}

// collectInlines parses the current group's content as inline nodes, used for
// the display text of a hyperlink field.
func (p *parser) collectInlines() ([]richdoc.Inline, error) {
	saved := p.segs
	p.segs = nil
	p.pushState()
	err := p.processGroup()
	p.popState()
	ins := segsToInlines(p.segs)
	p.segs = saved
	if err != nil {
		return nil, err
	}
	return ins, nil
}

// parseRef extracts the bookmark name from a REF or PAGEREF field instruction,
// for example `REF _Ref42 \h` yields "_Ref42". Switches such as `\h` and
// `\* MERGEFORMAT` after the name are ignored.
func parseRef(inst string) (string, bool) {
	f := strings.Fields(inst)
	if len(f) < 2 {
		return "", false
	}
	switch f[0] {
	case "REF", "PAGEREF":
		return f[1], true
	}
	return "", false
}

func parseHyperlink(inst string) (string, bool) {
	i := strings.Index(inst, "HYPERLINK")
	if i < 0 {
		return "", false
	}
	rest := inst[i+len("HYPERLINK"):]
	q := strings.IndexByte(rest, '"')
	if q < 0 {
		if f := strings.Fields(rest); len(f) > 0 {
			return f[0], true
		}
		return "", false
	}
	rest = rest[q+1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

func (p *parser) applyWord(t token) {
	st := p.top()
	switch t.sval {
	case "uc":
		if t.hasParam {
			st.ucN = t.ival
		}
	case "b":
		st.bold = flag(t)
	case "i":
		st.italic = flag(t)
	case "strike":
		st.strike = flag(t)
	case "plain":
		st.bold, st.italic, st.strike, st.code, st.fontIdx = false, false, false, false, -1
	case "f":
		st.fontIdx = t.ival
		st.code = p.fontMono[t.ival]
	case "s":
		st.styleIdx = t.ival
	case "outlinelvl":
		st.outline = t.ival
	case "li":
		st.li = t.ival
	case "ilvl", "ls":
		st.listItem = true
	case "pard":
		st.styleIdx, st.outline, st.li = -1, -1, 0
		st.listItem, st.listOrdered, st.hr = false, false, false
	case "par":
		p.endParagraph()
	case "line", "softline":
		p.segs = append(p.segs, breakSeg{})
	case "tab":
		p.appendText("\t")
	case "brdrb":
		st.hr = true
	case "u":
		p.appendUnicode(t)
	default:
		// Unknown control words carry no meaning we model; ignore them.
	}
}

// flag reports the on/off value of a toggle control word: \b turns on, \b0 off.
func flag(t token) bool {
	if !t.hasParam {
		return true
	}
	return t.ival != 0
}

func (p *parser) applySymbol(t token) {
	switch t.sval {
	case "\\", "{", "}":
		p.appendText(t.sval)
	case "~":
		p.appendText(" ")
	case "_":
		p.appendText("-")
	case "-":
		// optional hyphen: contributes nothing to the text
	default:
		// other control symbols are not modelled
	}
}

func (p *parser) appendUnicode(t token) {
	if !t.hasParam {
		return
	}
	cp := t.ival
	if cp < 0 {
		cp += 65536
	}
	p.appendText(string(rune(cp)))
	p.uniSkip = p.top().ucN
}

func (p *parser) appendText(s string) {
	if p.uniSkip > 0 {
		rs := []rune(s)
		drop := p.uniSkip
		if drop > len(rs) {
			drop = len(rs)
		}
		p.uniSkip -= drop
		rs = rs[drop:]
		if len(rs) == 0 {
			return
		}
		s = string(rs)
	}
	f := p.currentFlags()
	if n := len(p.segs); n > 0 {
		if ts, ok := p.segs[n-1].(*textSeg); ok && ts.flags == f {
			ts.text += s
			return
		}
	}
	p.segs = append(p.segs, &textSeg{flags: f, text: s})
}

func (p *parser) currentFlags() fmtFlags {
	st := p.top()
	return fmtFlags{bold: st.bold, italic: st.italic, strike: st.strike, code: st.code}
}

func (p *parser) endParagraph() {
	st := p.top()
	inlines := segsToInlines(p.segs)
	p.segs = nil
	p.uniSkip = 0 // a Unicode fallback skip never spans a paragraph
	p.emitPara(st, inlines)
}

func (p *parser) emitPara(st *state, inlines []richdoc.Inline) {
	if st.hr && len(inlines) == 0 {
		p.db.flushList()
		p.db.flushQuote()
		p.db.add(richdoc.ThematicBreak{})
		return
	}
	if len(inlines) == 0 {
		return
	}
	if st.listItem {
		p.db.flushQuote()
		p.db.addListItem(st.listOrdered, richdoc.Paragraph{Inlines: inlines})
		return
	}
	if lvl := headingLevel(st, p.styleHeading); lvl >= 1 {
		p.db.flushList()
		p.db.flushQuote()
		p.db.add(richdoc.Heading{Level: lvl, Inlines: inlines})
		return
	}
	if st.li > 0 {
		p.db.flushList()
		p.db.addQuote(richdoc.Paragraph{Inlines: inlines})
		return
	}
	p.db.flushList()
	p.db.flushQuote()
	p.db.add(richdoc.Paragraph{Inlines: inlines})
}

func headingLevel(st *state, styleHeading map[int]int) int {
	if st.outline >= 0 {
		l := st.outline + 1
		if l > 6 {
			l = 6
		}
		return l
	}
	if st.styleIdx >= 0 {
		if h, ok := styleHeading[st.styleIdx]; ok && h > 0 {
			return h
		}
		if st.styleIdx >= 1 && st.styleIdx <= 6 {
			return st.styleIdx
		}
	}
	return 0
}

func segsToInlines(segs []segment) []richdoc.Inline {
	var out []richdoc.Inline
	for _, s := range segs {
		switch v := s.(type) {
		case *textSeg:
			if v.text != "" {
				out = append(out, wrapFlags(v.text, v.flags))
			}
		case breakSeg:
			out = append(out, richdoc.LineBreak{})
		case inlineSeg:
			out = append(out, v.inline)
		case anchorStartSeg:
			// A bookmark start with no matching end in this run is a point
			// anchor carrying no marked text.
			out = append(out, richdoc.Anchor{ID: v.id})
		}
	}
	return out
}

// wrapFlags builds the nested inline tree for a run of text. Code is a leaf and
// takes precedence; otherwise strikethrough, emphasis and strong nest from the
// inside out, giving a canonical Strong>Emph>Strikethrough>Text order.
func wrapFlags(text string, f fmtFlags) richdoc.Inline {
	if f.code {
		return richdoc.Code{Value: text}
	}
	var in richdoc.Inline = richdoc.Text{Value: text}
	if f.strike {
		in = richdoc.Strikethrough{Inlines: []richdoc.Inline{in}}
	}
	if f.italic {
		in = richdoc.Emph{Inlines: []richdoc.Inline{in}}
	}
	if f.bold {
		in = richdoc.Strong{Inlines: []richdoc.Inline{in}}
	}
	return in
}

// docBuilder accumulates blocks, coalescing consecutive list items into a
// single List and consecutive quoted paragraphs into a single BlockQuote.
type docBuilder struct {
	blocks []richdoc.Block
	list   *richdoc.List
	quote  []richdoc.Block
}

func (d *docBuilder) add(b richdoc.Block) { d.blocks = append(d.blocks, b) }

func (d *docBuilder) addListItem(ordered bool, b richdoc.Block) {
	if d.list == nil || d.list.Ordered != ordered {
		d.flushList()
		d.list = &richdoc.List{Ordered: ordered, Start: 1, Tight: true}
	}
	d.list.Items = append(d.list.Items, richdoc.ListItem{Blocks: []richdoc.Block{b}})
}

func (d *docBuilder) flushList() {
	if d.list != nil {
		d.blocks = append(d.blocks, *d.list)
		d.list = nil
	}
}

func (d *docBuilder) addQuote(b richdoc.Block) { d.quote = append(d.quote, b) }

func (d *docBuilder) flushQuote() {
	if len(d.quote) > 0 {
		d.blocks = append(d.blocks, richdoc.BlockQuote{Blocks: d.quote})
		d.quote = nil
	}
}
