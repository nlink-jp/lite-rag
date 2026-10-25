// Package chunker splits normalized Markdown text into overlapping chunks
// suitable for embedding. It handles both Japanese and English content.
package chunker

import (
	"regexp"
	"strings"

	"lite-rag/internal/normalizer"
)

// Chunk is a single text unit produced by the chunker.
type Chunk struct {
	// Content is the normalized, prefix-free text of this chunk.
	// Callers (Indexer / Retriever) are responsible for adding
	// nomic-embed task prefixes before sending to the embedding API.
	Content string

	// HeadingPath is the hierarchical heading context for this chunk,
	// e.g. "# Guide > ## Installation > ### Linux".
	HeadingPath string

	// Index is the 0-based position of this chunk within the document.
	Index int
}

// Chunker splits Markdown text into chunks of at most ChunkSize tokens,
// with ChunkOverlap tokens of context carried over between adjacent chunks.
type Chunker struct {
	ChunkSize    int
	ChunkOverlap int
}

// New returns a Chunker with the given token limits.
func New(chunkSize, chunkOverlap int) *Chunker {
	return &Chunker{ChunkSize: chunkSize, ChunkOverlap: chunkOverlap}
}

var headingRe = regexp.MustCompile(`(?m)^(#{1,6})[ \t]+(.+)$`)

// section holds one heading-delimited region of a Markdown document.
type section struct {
	headingPath string
	content     string
}

// Chunk splits normalized Markdown text and returns all chunks in document order.
// The caller should pass text that has already been processed by normalizer.Normalize.
func (c *Chunker) Chunk(text string) []Chunk {
	sections := splitByHeadings(text)
	var result []Chunk
	idx := 0
	for _, sec := range sections {
		for _, ch := range c.packSection(sec) {
			ch.Index = idx
			idx++
			result = append(result, ch)
		}
	}
	return result
}

// splitByHeadings splits text into sections at Markdown heading boundaries,
// building a cumulative heading path for each section.
// Lines inside fenced code blocks (``` or ~~~) are never treated as headings.
func splitByHeadings(text string) []section {
	lines := strings.Split(text, "\n")

	// headings[level] holds the most recent heading at that depth (1-6).
	var headings [7]string
	var contentBuf strings.Builder
	var currentPath string
	var sections []section
	inFence := false // true while inside a ``` or ~~~ code block

	flush := func() {
		content := strings.TrimSpace(contentBuf.String())
		contentBuf.Reset()
		if content == "" && currentPath == "" {
			return
		}
		sections = append(sections, section{headingPath: currentPath, content: content})
	}

	for _, line := range lines {
		// Toggle fence state on opening/closing markers.
		// A fence marker is a line that starts with ``` or ~~~.
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			contentBuf.WriteString(line)
			contentBuf.WriteByte('\n')
			continue
		}

		// Inside a code block: treat every line as plain content.
		if inFence {
			contentBuf.WriteString(line)
			contentBuf.WriteByte('\n')
			continue
		}

		m := headingRe.FindStringSubmatch(line)
		if m == nil {
			contentBuf.WriteString(line)
			contentBuf.WriteByte('\n')
			continue
		}
		flush()
		level := len(m[1])
		headings[level] = m[1] + " " + strings.TrimSpace(m[2])
		// Clear all deeper levels when a shallower heading appears.
		for i := level + 1; i <= 6; i++ {
			headings[i] = ""
		}
		var parts []string
		for i := 1; i <= 6; i++ {
			if headings[i] != "" {
				parts = append(parts, headings[i])
			}
		}
		currentPath = strings.Join(parts, " > ")
	}
	flush()
	return sections
}

// packSection breaks a single section into one or more chunks, respecting
// ChunkSize and carrying ChunkOverlap tokens across chunk boundaries.
func (c *Chunker) packSection(sec section) []Chunk {
	// Split content into atomic blocks: paragraphs first, then sentences if
	// a paragraph still exceeds ChunkSize.
	blocks := atomize(sec.content, c.ChunkSize)
	if len(blocks) == 0 {
		return nil
	}

	var chunks []Chunk
	var window []string // blocks accumulated for the current chunk
	windowTokens := 0

	emit := func() {
		// Defensive guard: the loop below guarantees window is non-empty
		// before every emit() call, but protect against future refactoring.
		if len(window) == 0 {
			return
		}
		chunks = append(chunks, Chunk{
			Content:     strings.Join(window, "\n"),
			HeadingPath: sec.headingPath,
		})
		// Carry overlap into the next window.
		window, windowTokens = trimToOverlap(window, c.ChunkOverlap)
	}

	for _, b := range blocks {
		bt := normalizer.EstimateTokens(b)
		if windowTokens+bt > c.ChunkSize && len(window) > 0 {
			emit()
		}
		window = append(window, b)
		windowTokens += bt
	}
	emit()
	return chunks
}

// atomize splits text into the smallest blocks that fit within maxTokens.
// Priority: paragraph breaks → Japanese sentence endings → Western sentence endings.
func atomize(text string, maxTokens int) []string {
	paragraphs := splitParagraphs(text)
	var result []string
	for _, p := range paragraphs {
		if normalizer.EstimateTokens(p) <= maxTokens {
			result = append(result, p)
			continue
		}
		// Paragraph too large: try sentence splitting.
		result = append(result, splitSentences(p)...)
	}
	return result
}

var paragraphRe = regexp.MustCompile(`\n{2,}`)

// splitParagraphs splits text on blank lines.
func splitParagraphs(text string) []string {
	parts := paragraphRe.Split(text, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// sentenceEndRe matches a run of sentence-ending punctuation.
// Handles Japanese full-stop characters and Western punctuation followed by
// whitespace or end-of-string.
var sentenceEndRe = regexp.MustCompile(`[。．！？]+|[.!?]+(?:\s+|$)`)

// splitSentences splits text at sentence boundaries, keeping each sentence's
// trailing punctuation attached to the left segment.
func splitSentences(text string) []string {
	var result []string
	last := 0
	for _, loc := range sentenceEndRe.FindAllStringIndex(text, -1) {
		end := loc[1]
		sent := strings.TrimSpace(text[last:end])
		if sent != "" {
			result = append(result, sent)
		}
		last = end
	}
	if last < len(text) {
		if tail := strings.TrimSpace(text[last:]); tail != "" {
			result = append(result, tail)
		}
	}
	return result
}

// trimToOverlap returns the suffix of blocks whose total token count does not
// exceed overlapTokens. Used to seed the next window after emitting a chunk.
func trimToOverlap(blocks []string, overlapTokens int) ([]string, int) {
	if overlapTokens <= 0 {
		return nil, 0
	}
	total := 0
	start := len(blocks)
	for i := len(blocks) - 1; i >= 0; i-- {
		t := normalizer.EstimateTokens(blocks[i])
		if total+t > overlapTokens {
			break
		}
		total += t
		start = i
	}
	kept := blocks[start:]
	if len(kept) == 0 {
		return nil, 0
	}
	return kept, total
}
