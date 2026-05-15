// Package journal implements a per-symbol write-ahead log of accepted
// matching-engine operations. Each Market appends its inbound ops to the
// journal before applying them, so a crashed process can recover the
// exact same book state by replaying the journal in order.
//
// Format: one file per symbol named <symbol>.journal, one record per line
// JSON-encoded. Append-only; no compaction in this revision.
package journal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aliraad79/Gun/models"
)

// RecordKind discriminates between the three operations the engine
// accepts at its public surface.
type RecordKind string

const (
	RecNew    RecordKind = "new"
	RecCancel RecordKind = "cancel"
	RecModify RecordKind = "modify"
)

// Record is one journal entry. All fields use omitempty so the wire form
// stays compact and only carries the data each Kind actually needs.
type Record struct {
	Kind      RecordKind   `json:"kind"`
	Timestamp int64        `json:"ts,omitempty"` // unix nanos
	Order     models.Order `json:"order,omitempty"`
	OrderID   int64        `json:"order_id,omitempty"`   // cancel + modify
	NewPrice  models.Px    `json:"new_price,omitempty"`  // modify
	NewVolume models.Qty   `json:"new_volume,omitempty"` // modify
	Symbol    string       `json:"symbol,omitempty"`     // cancel + modify (new orders carry it on Order)
}

// Journal is the interface Market depends on. The two implementations
// shipped here are FileJournal (production: durable, fsync on append)
// and Discard (tests and benchmarks: no I/O).
type Journal interface {
	Append(symbol string, rec Record) error
	Replay(symbol string, fn func(Record) error) error
	Close() error
}

// ErrCorruptRecord is returned by Replay when a journal line cannot be
// parsed as a Record.
var ErrCorruptRecord = errors.New("journal: corrupt record")

// ----------------------- discard ---------------------------

// Discard is a no-op journal for tests, benchmarks, and ephemeral runs.
type Discard struct{}

func (Discard) Append(string, Record) error                  { return nil }
func (Discard) Replay(string, func(Record) error) error      { return nil }
func (Discard) Close() error                                 { return nil }

// ----------------------- file-backed -----------------------

// FileJournal is a directory of <symbol>.journal append-only files.
// One goroutine per Market writes its symbol's file; the FileJournal is
// safe for concurrent writes across symbols (one open file handle per
// symbol, lazily created on first Append).
//
// Fsync is forced on every Append by default. Set FsyncOnAppend = false
// at construction time to trade durability for throughput (matches the
// behavior of most production engines that batch fsyncs).
type FileJournal struct {
	dir            string
	fsyncOnAppend  bool

	mu    sync.Mutex
	files map[string]*bufio.Writer
	raw   map[string]*os.File
}

// NewFileJournal opens (or creates) the journal directory. The directory
// is created with mkdir -p semantics. Returns an error if dir is empty
// or cannot be created.
func NewFileJournal(dir string, fsyncOnAppend bool) (*FileJournal, error) {
	if dir == "" {
		return nil, errors.New("journal: directory path is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("journal: mkdir %s: %w", dir, err)
	}
	return &FileJournal{
		dir:           dir,
		fsyncOnAppend: fsyncOnAppend,
		files:         make(map[string]*bufio.Writer),
		raw:           make(map[string]*os.File),
	}, nil
}

// Append serializes rec as one JSON line and writes it to the symbol's
// journal file. Fsyncs after the write when FsyncOnAppend is true.
func (j *FileJournal) Append(symbol string, rec Record) error {
	if rec.Timestamp == 0 {
		rec.Timestamp = time.Now().UnixNano()
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("journal: marshal record: %w", err)
	}
	line = append(line, '\n')

	j.mu.Lock()
	defer j.mu.Unlock()

	w, f, err := j.openLocked(symbol)
	if err != nil {
		return err
	}
	if _, err := w.Write(line); err != nil {
		return fmt.Errorf("journal: write %s: %w", symbol, err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("journal: flush %s: %w", symbol, err)
	}
	if j.fsyncOnAppend {
		if err := f.Sync(); err != nil {
			return fmt.Errorf("journal: fsync %s: %w", symbol, err)
		}
	}
	return nil
}

// Replay reads the symbol's journal from the beginning, decoding each
// line into a Record and passing it to fn. Stops on the first error
// returned by fn, or on a corrupt record. Missing file is not an error
// (a fresh symbol has no journal yet).
func (j *FileJournal) Replay(symbol string, fn func(Record) error) error {
	path := j.pathFor(symbol)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("journal: open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Bump the buffer for orders with long string fields (decimal prices,
	// future TIF/STP additions).
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(raw, &rec); err != nil {
			return fmt.Errorf("%w at %s:%d: %v", ErrCorruptRecord, path, lineNum, err)
		}
		if err := fn(rec); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("journal: scan %s: %w", path, err)
	}
	return nil
}

// Close flushes and closes every per-symbol file handle.
func (j *FileJournal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	var firstErr error
	for symbol, w := range j.files {
		if err := w.Flush(); err != nil && firstErr == nil {
			firstErr = err
		}
		f := j.raw[symbol]
		if f != nil {
			if err := f.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		delete(j.files, symbol)
		delete(j.raw, symbol)
	}
	return firstErr
}

func (j *FileJournal) openLocked(symbol string) (*bufio.Writer, *os.File, error) {
	if w, ok := j.files[symbol]; ok {
		return w, j.raw[symbol], nil
	}
	f, err := os.OpenFile(j.pathFor(symbol), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("journal: open %s: %w", j.pathFor(symbol), err)
	}
	w := bufio.NewWriter(f)
	j.files[symbol] = w
	j.raw[symbol] = f
	return w, f, nil
}

func (j *FileJournal) pathFor(symbol string) string {
	// Sanitize so a malicious symbol can't escape the journal dir. Slashes
	// and dots are the obvious vectors; replace with underscore.
	safe := strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(symbol)
	return filepath.Join(j.dir, safe+".journal")
}
