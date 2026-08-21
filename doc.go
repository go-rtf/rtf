// Copyright (c) the go-rtf authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package rtf converts between a practical subset of the Rich Text Format
// (RTF) and the neutral [github.com/go-richdoc/richdoc] document model.
//
// [Parse] tokenises RTF (groups, control words, control symbols, \'hh hex
// bytes and \uN Unicode escapes) and folds the group-scoped character state
// (bold, italic, strikethrough, monospace) into a [richdoc.Document]. [Write]
// emits a minimal, well-formed RTF document from a [richdoc.Document]. The two
// directions are designed as a faithful round-trip for the supported subset:
// Parse(Write(Parse(src))) is semantically equal to Parse(src).
//
// Footnote groups map to [richdoc.Footnote], bookmarks
// ({\*\bkmkstart}/{\*\bkmkend}) to [richdoc.Anchor], and REF/PAGEREF fields to
// a RefLabel [richdoc.CrossRef]; HYPERLINK fields stay a [richdoc.Link].
//
// RTF has no native concept for several richdoc nodes (code blocks, block and
// inline math, tables, images with embedded data). Those are emitted in a
// best-effort visual form by [Write] and are documented as one-way mappings.
// Conversely, constructs the model has no node for (embedded pictures and
// fields other than hyperlinks and references) are preserved verbatim through
// [richdoc.RawInline] with Format "rtf", so nothing in the source is silently
// lost.
//
// The package is pure Go and builds with CGO disabled, including for
// GOOS=js/GOARCH=wasm.
package rtf
