# rtf

An **RTF ⇄ [richdoc](https://github.com/go-richdoc/richdoc)** converter, written
in pure Go (CGO-free, including `GOOS=js`).

`rtf` parses a practical subset of the Rich Text Format into the neutral
`richdoc` document model, and emits a minimal, well-formed RTF document from a
`richdoc.Document`. The two directions are designed as a faithful round-trip.

```go
d, err := rtf.Parse(src)   // RTF subset -> *richdoc.Document
out, err := rtf.Write(d)   // *richdoc.Document -> RTF document
```

## API

```go
func Parse(src []byte) (*richdoc.Document, error)
func Write(d *richdoc.Document) ([]byte, error)
```

`Parse` tokenises RTF properly — groups `{}`, control words `\word` with an
optional numeric parameter, control symbols (`\\ \{ \} \~ \- \_`), `\'hh` hex
bytes (decoded through the `\ansicpgN` / default code page, best-effort
cp1252) and `\uN` Unicode escapes (skipping `\ucN` fallback characters). RTF
groups scope formatting, so character state is tracked on a group stack.
Anything the model has no node for is preserved verbatim through
`RawInline` with `Format: "rtf"`, so nothing in the source is silently lost.
`Write` escapes the RTF specials `\ { }`, emits non-ASCII as `\uN?`, and
declares a two-font table (proportional `\f0`, monospace `\f1`) plus a small
stylesheet so headings survive the round-trip.

## Construct mapping

The supported subset maps to `richdoc` as follows.

### RTF → richdoc (`Parse`)

| RTF | richdoc |
| --- | --- |
| `\b` / `\b0` | `Strong` |
| `\i` / `\i0` | `Emph` |
| `\strike` / `\strike0` | `Strikethrough` |
| a run in a monospace font (`\fN` → `\fmodern` / Courier / Consolas / *mono*) | `Code` (inline) |
| plain text, `\'hh`, `\uN`, `\tab`, `\~`, `\_` | `Text` |
| `\line` / `\softline` | `LineBreak` |
| `\par` | ends a `Paragraph` |
| `\pard` | resets paragraph properties |
| `\sN` (resolved against `{\stylesheet}`) or `\outlinelvlN` | `Heading` (level 1–6) |
| `{\pntext…}` / `{\*\pn…}` / `\listtext` / `\ilvl` / `\ls` | `List` / `ListItem` (`\pndec` or a digit marker → ordered, `\pnlvlblt` → unordered) |
| `\liN` left indent (not a list/heading) | `BlockQuote` |
| `\brdrb` on an empty paragraph | `ThematicBreak` |
| `{\field{\*\fldinst HYPERLINK "url"}{\fldrslt text}}` | `Link` |
| `{\fonttbl…}` | read to classify monospace fonts, then dropped |
| `{\stylesheet…}` | read to resolve heading styles, then dropped |
| `{\colortbl…}`, `{\info…}`, `{\*\…}` and other unrecognised destinations | consumed and dropped |

### richdoc → RTF (`Write`)

| richdoc | RTF |
| --- | --- |
| `Heading` | `\pard\sN\outlinelvl(N-1) … \par` (+ a stylesheet entry) |
| `Paragraph` | `\pard … \par` |
| `Strong` | `{\b …}` |
| `Emph` | `{\i …}` |
| `Strikethrough` | `{\strike …}` |
| `Code`, `Math` | `{\f1 …}` (monospace) |
| `List` | one `\pard{\pntext…}{\*\pn…}…\par` per item |
| `BlockQuote` | `\pard\li720 … \par` per block |
| `CodeBlock` | `\pard` monospace lines separated by `\line` |
| `LineBreak` | `\line` |
| `Link` | `{\field{\*\fldinst HYPERLINK "url"}{\fldrslt text}}` |
| `ThematicBreak` | `\pard\brdrb\brdrs\brdrw10 \par` |
| `MathBlock` | `\pard{\f1 tex}\par` |
| `Table` | tab-separated `\par` rows (flattened) |
| `Image` | its alternative text |
| `RawBlock` / `RawInline` (`Format: "rtf"`) | emitted verbatim; other formats dropped |

## Model gaps (routed through `Raw`)

RTF has no node for some richdoc constructs and vice-versa. Constructs the
model cannot represent are preserved verbatim by `Parse` as `RawInline`
(`Format: "rtf"`):

- **Embedded pictures** `{\pict…}` — the model's `Image` only references a URL,
  so pixel data has no home; the whole group (including its hex data) is kept
  raw.
- **Footnotes** `{\footnote…}` — no footnote node.
- **Non-hyperlink fields** `{\field…}` (dates, page numbers, references, …) —
  only `HYPERLINK` maps to `Link`; every other field is kept raw.

Conversely, a few richdoc nodes have no RTF concept and are written one-way
(visually faithful, but not reconstructed by `Parse`): `CodeBlock` and
`MathBlock` become monospace paragraphs, inline `Math` becomes monospace text,
`Table` is flattened to tab-separated rows, and `Image` is reduced to its alt
text.

## Reference library

RTF has no dominant Go library. The maintained options are one-directional or
lossy: `j45k4/rtf` and `lu4p/cat` strip RTF to plain text, while `therox/rtf-doc`
and `max-legrand/rtf-doc` only *create* documents; `docconv` shells out to
external tools. None parse RTF into a structured, writable model or target
`richdoc`. RTF is a well-specified control-word format the Go standard library
handles cleanly, so this package ships a focused, in-org parser and writer with
no non-Go dependency.

## License

BSD-3-Clause. Copyright (c) the go-rtf authors.
