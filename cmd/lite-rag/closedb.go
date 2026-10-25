package main

import (
	"errors"
	"fmt"
	"io"
)

// closeDB closes db and adds a failure to *err. DuckDB checkpoints on
// Close, so after index, reindex or docs delete a close failure can mean
// the change did not reach the file — the command must not report success.
func closeDB(db io.Closer, err *error) {
	if cerr := db.Close(); cerr != nil {
		*err = errors.Join(*err, fmt.Errorf("close database: %w", cerr))
	}
}
