# Dependency Register

All third-party dependencies are listed here as required by RULES.md §18.

---

## Direct Dependencies

### `github.com/marcboeker/go-duckdb/v2` @ v2.4.3

- **Purpose:** CGo-based Go driver for DuckDB v2. Provides `database/sql`
  compatibility and supports passing `[]float32` directly as SQL parameters to
  `FLOAT[]` columns, enabling DB-side cosine similarity via `list_cosine_similarity()`.
- **Why not in-house:** Implementing a DuckDB wire protocol driver from scratch would
  be prohibitive. DuckDB was chosen for its embedded nature (no separate server process)
  and built-in array arithmetic functions.
- **Why v2 (not v1):** go-duckdb v1 cannot pass `[]float32` as a SQL parameter to a
  `FLOAT[]` column; embeddings had to be stored as `BLOB` and similarity computed in Go.
  v2 resolves this, allowing the similarity search to run entirely inside DuckDB using
  `list_cosine_similarity`, which is both simpler and more efficient.
- **License:** MIT
- **Compliance:** No restrictions for internal or commercial use.

### `github.com/spf13/cobra` @ v1.10.2

- **Purpose:** CLI framework providing structured subcommands, flag parsing, and
  help text generation.
- **Why not in-house:** The standard `flag` package lacks subcommand support. Cobra
  is the de-facto standard for Go CLIs and reduces boilerplate significantly.
- **License:** Apache 2.0
- **Compliance:** No restrictions for internal or commercial use.

### `github.com/BurntSushi/toml` @ v1.6.0

- **Purpose:** TOML configuration file parsing.
- **Why not in-house:** TOML parsing is non-trivial. This library is minimal (no
  transitive dependencies), widely used, and actively maintained.
- **License:** MIT
- **Compliance:** No restrictions.

### `golang.org/x/text` @ v0.35.0

- **Purpose:** Unicode normalization (NFKC) for Japanese/English mixed-text handling.
  Used in `internal/normalizer` to normalize full-width characters, half-width Katakana,
  and compatibility characters before chunking and embedding.
- **Why not in-house:** Unicode normalization algorithms are complex and error-prone to
  implement correctly. This is the standard library extension maintained by the Go team.
- **License:** BSD 3-Clause
- **Compliance:** No restrictions.

---

## Indirect Dependencies

Indirect dependencies are pulled in by the packages above. They are not listed
individually here; see `go.mod` for the full list. All are permissive licenses
(MIT, Apache 2.0, BSD).
