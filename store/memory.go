package store

import (
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
)

// MemoryStore is a SQLite-backed store over an in-memory database. It shares
// the exact SQLite implementation and invariants of the durable store, so the
// demo server and tests exercise the same code paths as production without a
// file on disk. It is not a separate implementation: it is the same
// SQLiteStore with a ":memory:" path.
type MemoryStore = SQLiteStore

// NewMemoryStore builds a store backed by the given catalog over an in-memory
// SQLite database.
func NewMemoryStore(cat catalog.Catalog) *MemoryStore {
	s, err := OpenSQLite(":memory:", cat)
	if err != nil {
		panic("store: cannot open in-memory store: " + err.Error())
	}
	return s
}
