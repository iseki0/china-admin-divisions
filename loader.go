package admindivision

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

// Snapshot is immutable. Only the compressed wire representation is retained.
type Snapshot struct {
	index    *Index
	gzipData []byte
	etag     string
}

func (s *Snapshot) Index() *Index      { return s.index }
func (s *Snapshot) GzipBinary() []byte { return bytes.Clone(s.gzipData) }
func (s *Snapshot) ETag() string       { return s.etag }

// Loader atomically replaces snapshots; its zero value uses embedded binary.
type Loader struct {
	once       sync.Once
	current    atomic.Pointer[Snapshot]
	initialErr error
}

var DefaultLoader = &Loader{}

// Current panics if the embedded dataset cannot be decoded or validated.
func (l *Loader) Current() *Snapshot {
	l.once.Do(func() {
		s, err := snapshotBinary(embedded)
		l.initialErr = err
		if err == nil {
			l.current.Store(s)
		}
	})
	if s := l.current.Load(); s != nil {
		return s
	}
	panic(&EmbeddedDataError{Err: l.initialErr})
}
func snapshotBinary(data []byte) (*Snapshot, error) {
	nodes, err := decodeGzip(data)
	if err != nil {
		return nil, err
	}
	records := make([]Record, len(nodes))
	for i, n := range nodes {
		records[i] = n.Record
	}
	idx, err := New(records)
	if err != nil {
		return nil, err
	}
	return &Snapshot{idx, bytes.Clone(data), fmt.Sprintf(`"%x"`, sha256.Sum256(data))}, nil
}

// NewLoader validates JSON and builds an independent binary snapshot.
func NewLoader(data []byte) (*Loader, error) {
	compressed, err := CompileJSON(data)
	if err != nil {
		return nil, err
	}
	s, err := snapshotBinary(compressed)
	if err != nil {
		return nil, err
	}
	l := &Loader{}
	l.once.Do(func() {})
	l.current.Store(s)
	return l, nil
}

// LoadFile reads maintained JSON; failure leaves the previous snapshot intact.
func (l *Loader) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	replacement, err := NewLoader(data)
	if err != nil {
		return err
	}
	// A successful explicit reload may initialize a loader without using embed.
	l.once.Do(func() {})
	l.current.Store(replacement.Current())
	return nil
}
