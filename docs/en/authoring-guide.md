# Authoring Guide: Writing Markdown for lite-rag

This guide explains how lite-rag processes your documents and what Markdown
writing practices produce the best retrieval results.

---

## How lite-rag processes a document

Understanding the pipeline helps you write documents that index well.

```
Raw Markdown file
      │
      ▼
  Normalize          Unicode NFKC, whitespace cleanup, line-ending normalization
      │
      ▼
  Split by headings  Each heading creates a new section; HeadingPath is built
      │              up cumulatively (e.g. "# Guide > ## Install > ### Linux")
      ▼
  Atomize sections   Each section is split into paragraphs; if a paragraph
      │              still exceeds chunk_size it is split further at sentence
      │              boundaries (。for Japanese, ./?/! for English)
      ▼
  Pack into chunks   Paragraphs/sentences are packed up to chunk_size tokens;
      │              chunk_overlap tokens are carried into the next chunk
      ▼
  Embed & store      Each chunk gets a vector embedding and is stored in DuckDB
                     with its HeadingPath, content, and document metadata
```

Token estimation used by the chunker:

- CJK characters (Kanji, Hiragana, Katakana): **1 char ≈ 2 tokens**
- ASCII/Latin words: **1 word ≈ 1.3 tokens**

Default settings: `chunk_size = 512`, `chunk_overlap = 64`.

---

## 1. Use headings to create semantic sections

Headings are the primary way lite-rag understands document structure. Every
chunk carries the full heading hierarchy of its section as `HeadingPath`.
This path is shown in search results and used as context in the LLM prompt.

**Good — clear hierarchy:**

```markdown
# Installation Guide

## macOS

Install Homebrew, then run:

```sh
brew install lite-rag
```

## Linux

Download the binary from the releases page.
```

The `brew install` paragraph gets `HeadingPath = "# Installation Guide > ## macOS"`,
so a question about macOS installation will surface this chunk with high confidence.

**Avoid — flat structure:**

```markdown
# Everything

All setup steps, configuration, usage, and troubleshooting are described below.

Install with brew. Configure with config.toml. Run lite-rag ask "your question".
If something goes wrong, check the logs.
```

Without sub-headings, all content lands in one large section. Unrelated topics
compete for space in the same chunk, degrading retrieval precision.

---

## 2. Keep each section focused on one topic

The chunker cannot split a section by topic — only by size. If a section mixes
multiple concerns, a chunk may contain irrelevant material that lowers its
similarity score for any single query.

**Good:**

```markdown
## Configuration

Edit `config.toml` to set the embedding model and API endpoint.
The `chunk_size` field controls how many tokens each chunk contains (default: 512).
The `chunk_overlap` field adds context continuity across chunk boundaries (default: 64).
```

**Avoid:**

```markdown
## Misc

Edit config.toml. Also, the index command walks the directory recursively.
You can ask questions with `lite-rag ask`. The database is stored in DuckDB.
Run `make check` before committing.
```

---

## 3. Write declaratively — state facts explicitly

The embedding model compares query vectors to document vectors. Queries are
often questions, but documents contain answers. The match is strongest when the
document states the fact directly rather than implying it.

**Good:**

```markdown
The default embedding model is `nomic-embed-text-v1.5`.
lite-rag stores vector embeddings in a local DuckDB database file (`lite-rag.db`).
Re-indexing is skipped when the file hash and embedding model both match the stored values.
```

**Avoid:**

```markdown
As mentioned earlier, the model can be changed. The database thing is handled automatically.
If you already ran it before, it won't run again.
```

Vague pronouns, passive constructions, and forward/backward references all
weaken semantic matching.

---

## 4. Size sections to fit within chunk_size

With the default `chunk_size = 512` tokens, a section of roughly 200–400 Japanese
characters or 300–500 English words fits in a single chunk. When a section is
larger, the chunker will split it at paragraph or sentence boundaries.

Splitting is not harmful, but a self-contained fact split across two chunks
may appear in only one of them. Keep individual facts within a single paragraph.

**Rule of thumb:**

| Content type | Fits comfortably per chunk |
|--------------|---------------------------|
| Japanese prose | ~200–250 characters |
| English prose | ~300–400 words |
| Mixed JP/EN | ~150–200 Japanese chars + ~100 English words |
| Code blocks | included in token count; keep snippets concise |

---

## 5. Use blank lines to separate paragraphs

The chunker splits sections at **double newlines** (blank lines). A wall of
text with no blank lines is treated as one paragraph and packed as a unit.
Separate distinct ideas with blank lines so the chunker can handle them
independently.

**Good:**

```markdown
## Architecture

lite-rag uses DuckDB as its vector store. Embeddings are stored in a FLOAT[768] column
and similarity search is performed via `list_cosine_similarity`.

The chunker splits Markdown at heading and paragraph boundaries. Long paragraphs are
further split at sentence endings to stay within chunk_size.
```

**Avoid:**

```markdown
## Architecture
lite-rag uses DuckDB as its vector store. Embeddings are stored in a FLOAT[768] column and similarity search is performed via list_cosine_similarity. The chunker splits Markdown at heading and paragraph boundaries. Long paragraphs are further split at sentence endings to stay within chunk_size.
```

---

## 6. Japanese: end sentences with 。

When a paragraph exceeds `chunk_size`, the chunker splits at Japanese sentence
boundaries (。). Sentences that do not end with 。are treated as a tail fragment
and kept together, which may produce an oversized chunk.

**Good:**

```markdown
デフォルトのチャンクサイズは512トークンです。
オーバーラップは64トークンに設定されています。
これにより、チャンク境界をまたぐ文脈が保持されます。
```

**Avoid:**

```markdown
デフォルトのチャンクサイズは512トークンで、オーバーラップは64トークンに設定されており
これにより、チャンク境界をまたぐ文脈が保持されます
```

---

## 7. Use fenced code blocks for all code

Content inside fenced code blocks (` ``` ` or `~~~`) is treated as plain text —
`#` lines are not parsed as headings. The code is included in the chunk content
and contributes to the token count.

Keep code snippets short and directly relevant to the surrounding prose. If a
block is long, consider splitting it across multiple sections with explanatory
headings.

```markdown
## Starting the server

Run the following command to start the embedded server:

```sh
lite-rag serve --port 8080
```

The server listens on `localhost:8080` by default.
```

---

## 8. Avoid headings without body text

A heading immediately followed by another heading produces an empty section.
Empty sections are silently dropped by the chunker. The body text under the
second heading will carry only the second heading's `HeadingPath`, losing the
context of the first.

**Avoid:**

```markdown
# Guide
## Installation
## Usage

Usage instructions here.
```

The `## Installation` section is empty and dropped. Add at least a brief
description under each heading.

---

## 9. Use consistent terminology

The embedding model matches semantically similar text, but using different terms
for the same concept across documents weakens cross-document retrieval.

| Inconsistent | Consistent |
|---|---|
| "vector DB", "embedding store", "the database" | always "DuckDB vector store" |
| "chunk size", "block length", "token limit" | always "chunk_size" |
| "re-index", "update index", "refresh" | always "re-index" |

---

## 10. Summary checklist

- [ ] Each major topic has its own heading (H2 or H3)
- [ ] Sections are focused on one topic
- [ ] Facts are stated explicitly and declaratively
- [ ] Paragraphs are separated by blank lines
- [ ] Japanese sentences end with 。
- [ ] Code is in fenced blocks with a language identifier
- [ ] No headings immediately followed by another heading (no empty sections)
- [ ] Terminology is consistent across the document set
