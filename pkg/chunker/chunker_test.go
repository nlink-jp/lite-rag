package chunker_test

import (
	"strings"
	"testing"

	"lite-rag/pkg/chunker"
)

// ── helpers ────────────────────────────────────────────────────────────────

func contents(chunks []chunker.Chunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Content
	}
	return out
}

// ── Heading splitting ──────────────────────────────────────────────────────

func TestHeadingPath(t *testing.T) {
	text := `# Guide

intro text

## Installation

install text

### Linux

linux text

## Usage

usage text
`
	c := chunker.New(512, 0)
	chunks := c.Chunk(text)

	if len(chunks) == 0 {
		t.Fatal("expected chunks, got none")
	}

	want := map[string]string{
		"intro text":   "# Guide",
		"install text": "# Guide > ## Installation",
		"linux text":   "# Guide > ## Installation > ### Linux",
		"usage text":   "# Guide > ## Usage",
	}

	for _, ch := range chunks {
		exp, ok := want[ch.Content]
		if !ok {
			continue
		}
		if ch.HeadingPath != exp {
			t.Errorf("content %q: HeadingPath = %q, want %q", ch.Content, ch.HeadingPath, exp)
		}
	}
}

func TestHeadingSiblingResetsDeeper(t *testing.T) {
	// When ## appears after ### within the same # section, the ### must be cleared.
	text := `# Root

## A

### A1

a1 text

## B

b text
`
	c := chunker.New(512, 0)
	chunks := c.Chunk(text)

	for _, ch := range chunks {
		if ch.Content == "b text" && ch.HeadingPath != "# Root > ## B" {
			t.Errorf("b text HeadingPath = %q, want '# Root > ## B'", ch.HeadingPath)
		}
	}
}

// ── Index ordering ─────────────────────────────────────────────────────────

func TestIndexIsSequential(t *testing.T) {
	text := `# A

para one

para two

# B

para three
`
	c := chunker.New(20, 0) // small chunk size to force splits
	chunks := c.Chunk(text)
	for i, ch := range chunks {
		if ch.Index != i {
			t.Errorf("chunk[%d].Index = %d", i, ch.Index)
		}
	}
}

// ── Chunk size enforcement ─────────────────────────────────────────────────

func TestChunkSizeRespected(t *testing.T) {
	// Build a section whose paragraphs are each ~10 tokens.
	// ChunkSize=15 should split them into multiple chunks.
	para := strings.Repeat("word ", 10) // ~13 tokens each
	text := "# Section\n\n" + para + "\n\n" + para + "\n\n" + para

	c := chunker.New(15, 0)
	chunks := c.Chunk(text)

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for large content, got %d", len(chunks))
	}
}

// ── Overlap ────────────────────────────────────────────────────────────────

func TestOverlapCarriesContent(t *testing.T) {
	// Two paragraphs; overlap should carry the second para into the start of chunk 2.
	p1 := strings.Repeat("alpha ", 20) // ~26 tokens
	p2 := strings.Repeat("beta ", 5)   // ~7 tokens  ← should appear as overlap in chunk 2
	p3 := strings.Repeat("gamma ", 20) // ~26 tokens

	text := "# Sec\n\n" + p1 + "\n\n" + p2 + "\n\n" + p3

	c := chunker.New(30, 10)
	chunks := c.Chunk(text)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// chunk[1] should contain the overlap text ("beta")
	if !strings.Contains(chunks[1].Content, "beta") {
		t.Errorf("chunk[1] does not contain overlap text 'beta':\n%s", chunks[1].Content)
	}
}

// ── Japanese content ───────────────────────────────────────────────────────

func TestJapaneseSentenceBoundary(t *testing.T) {
	// A single paragraph with Japanese sentences. With a small chunk size the
	// chunker must split at 。 boundaries.
	text := `# 概要

これは最初の文章です。次の文章が続きます。三番目の文章もあります。四番目の文章で終わります。
`
	c := chunker.New(10, 0) // force splitting at sentence level
	chunks := c.Chunk(text)

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks from Japanese sentences, got %d: %v",
			len(chunks), contents(chunks))
	}
	// Every chunk should end with 。 or be the tail fragment
	for _, ch := range chunks {
		if !strings.HasSuffix(ch.Content, "。") && ch.Index != chunks[len(chunks)-1].Index {
			t.Logf("non-terminal chunk does not end with 。: %q", ch.Content)
		}
	}
}

func TestMixedJapaneseEnglish(t *testing.T) {
	text := `# Mixed

GoでRAGシステムを構築します。This is an English sentence. また日本語に戻ります。
`
	c := chunker.New(512, 0)
	chunks := c.Chunk(text)
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
}

// ── English-only content ───────────────────────────────────────────────────

func TestEnglishOnlyContent(t *testing.T) {
	text := `# Introduction

This is the first paragraph. It has two sentences.

This is the second paragraph. It also has two sentences.
`
	c := chunker.New(512, 0)
	chunks := c.Chunk(text)
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
}

// ── Edge cases ─────────────────────────────────────────────────────────────

func TestEmptyInput(t *testing.T) {
	c := chunker.New(512, 64)
	chunks := c.Chunk("")
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

func TestNoHeadings(t *testing.T) {
	text := "Just some plain text without any headings.\n"
	c := chunker.New(512, 0)
	chunks := c.Chunk(text)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].HeadingPath != "" {
		t.Errorf("HeadingPath = %q, want empty", chunks[0].HeadingPath)
	}
}

func TestHeadingWithNoBody(t *testing.T) {
	// A heading immediately followed by another heading (no body text).
	text := "# Title\n## Empty Section\n## Has Content\n\nbody\n"
	c := chunker.New(512, 0)
	chunks := c.Chunk(text)
	// Should produce at least one chunk for "body"
	found := false
	for _, ch := range chunks {
		if ch.Content == "body" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected chunk with 'body', got: %v", contents(chunks))
	}
}

func TestSplitSentences_TailWithoutPunctuation(t *testing.T) {
	// A paragraph that ends without sentence-final punctuation must produce a
	// tail chunk — this covers the `last < len(text)` branch in splitSentences.
	// chunk_size=5 forces atomize() to call splitSentences on the paragraph.
	text := "# 節\n\n最初の文。これは末尾なし\n"
	c := chunker.New(5, 0)
	chunks := c.Chunk(text)

	found := false
	for _, ch := range chunks {
		if strings.Contains(ch.Content, "末尾なし") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a chunk containing the unpunctuated tail, got: %v", contents(chunks))
	}
}

// ── Code fence handling ────────────────────────────────────────────────────

func TestCodeFenceHashNotHeading(t *testing.T) {
	// Shell comments inside a fenced code block must not be parsed as headings.
	text := "# Section\n\n```sh\n# this is a shell comment\nmake build\n```\n\nbody text\n"
	c := chunker.New(512, 0)
	chunks := c.Chunk(text)

	for _, ch := range chunks {
		if ch.HeadingPath == "# this is a shell comment" {
			t.Errorf("shell comment inside code block was misidentified as a heading")
		}
	}

	// body text must be under "# Section", not under the shell comment heading.
	for _, ch := range chunks {
		if strings.Contains(ch.Content, "body text") && ch.HeadingPath != "# Section" {
			t.Errorf("body text HeadingPath = %q, want '# Section'", ch.HeadingPath)
		}
	}
}

func TestCodeFenceContentPreserved(t *testing.T) {
	// Code inside a fenced block must be included in the chunk content.
	text := "# Sec\n\n```go\nfunc main() {}\n```\n\nafter\n"
	c := chunker.New(512, 0)
	chunks := c.Chunk(text)

	found := false
	for _, ch := range chunks {
		if strings.Contains(ch.Content, "func main()") {
			found = true
		}
	}
	if !found {
		t.Errorf("code fence content was lost: %v", contents(chunks))
	}
}

func TestTildeFenceHashNotHeading(t *testing.T) {
	// Same as TestCodeFenceHashNotHeading but using ~~~ delimiters.
	text := "# Title\n\n~~~sh\n# shell comment\necho hi\n~~~\n\nfooter\n"
	c := chunker.New(512, 0)
	chunks := c.Chunk(text)

	for _, ch := range chunks {
		if ch.HeadingPath == "# shell comment" {
			t.Errorf("~~~ fence: shell comment treated as heading")
		}
	}
}

func TestSplitSentences_AllEndWithPunctuation(t *testing.T) {
	// When every sentence ends with punctuation, every chunk ends with 。
	text := "# 節\n\n最初の文。二番目の文。三番目の文。\n"
	c := chunker.New(5, 0)
	chunks := c.Chunk(text)
	for _, ch := range chunks {
		if !strings.HasSuffix(ch.Content, "。") {
			t.Errorf("chunk does not end with 。: %q", ch.Content)
		}
	}
}
