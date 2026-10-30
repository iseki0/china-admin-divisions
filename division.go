// Package admindivision queries atomically replaceable administrative division snapshots.
package admindivision

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

//go:generate go run ./internal/generate

//go:embed data.bin.gz
var embedded []byte

// Deprecation preserves the source's three distinct states.
type Deprecation int

const (
	Unmarked Deprecation = iota // Not marked deprecated; current validity is unverified.
	Suspected
	Deprecated
)

func (d *Deprecation) UnmarshalJSON(b []byte) error {
	switch string(bytes.TrimSpace(b)) {
	case "false":
		*d = Unmarked
	case `"suspected"`:
		*d = Suspected
	case "true":
		*d = Deprecated
	default:
		return &ValidationError{Kind: ErrInvalidDeprecation, Field: "deprecated", Value: string(b), RecordIndex: -1}
	}
	return nil
}

func (d Deprecation) MarshalJSON() ([]byte, error) {
	switch d {
	case Unmarked:
		return []byte("false"), nil
	case Suspected:
		return []byte(`"suspected"`), nil
	case Deprecated:
		return []byte("true"), nil
	default:
		return nil, &ValidationError{Kind: ErrInvalidDeprecation, Field: "deprecated", Value: strconv.Itoa(int(d)), RecordIndex: -1}
	}
}

type Record struct {
	Code       string      `json:"code"`
	Name       string      `json:"name"`
	Pinyin     string      `json:"pinyin"`
	Deprecated Deprecation `json:"deprecated"`
}

// Options controls search. Zero Limit means 20; positive limits have no fixed cap.
// CodePrefix filters codes only and does not infer parent-child relationships.
type Options struct {
	Limit      int
	CodePrefix string
}

type entry struct {
	record Record
	keys   [4]string
}

// Index is immutable after construction and safe for concurrent queries.
// Its zero value is an empty index. Returned records are independent values.
type Index struct {
	entries []entry
	codes   map[string]Record
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalize(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "u:", "v")
	s = strings.ReplaceAll(s, "u\u0308", "v")
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.Is(unicode.M, r) {
			return -1
		}
		for _, group := range []struct {
			letters string
			base    rune
		}{
			{"āáǎà", 'a'}, {"ēéěèê", 'e'}, {"īíǐì", 'i'},
			{"ōóǒò", 'o'}, {"ūúǔù", 'u'}, {"üǖǘǚǜ", 'v'}, {"ńňǹ", 'n'}, {"ḿ", 'm'},
		} {
			if strings.ContainsRune(group.letters, r) {
				return group.base
			}
		}
		return r
	}, s)
}

// New validates and copies records into a reusable index.
func New(records []Record) (*Index, error) {
	idx := &Index{codes: make(map[string]Record, len(records))}
	for i, r := range records {
		invalid := func(kind error, field, value string) error {
			return &ValidationError{Kind: kind, Field: field, Value: value, RecordIndex: i, Code: r.Code}
		}
		if r.Code == "" || !digits(r.Code) {
			return nil, invalid(ErrInvalidCode, "code", r.Code)
		}
		if strings.TrimSpace(r.Name) == "" {
			return nil, invalid(ErrInvalidName, "name", r.Name)
		}
		if r.Deprecated < Unmarked || r.Deprecated > Deprecated {
			return nil, invalid(ErrInvalidDeprecation, "deprecated", strconv.Itoa(int(r.Deprecated)))
		}
		if _, ok := idx.codes[r.Code]; ok {
			return nil, invalid(ErrDuplicateCode, "code", r.Code)
		}
		parts := strings.Fields(r.Pinyin)
		if len(parts) == 0 {
			return nil, invalid(ErrInvalidPinyin, "pinyin", r.Pinyin)
		}
		var initials strings.Builder
		for _, part := range parts {
			p := normalize(part)
			if p == "" || strings.IndexFunc(p, func(c rune) bool { return c < 'a' || c > 'z' }) >= 0 {
				return nil, invalid(ErrInvalidPinyin, "pinyin", r.Pinyin)
			}
			initials.WriteByte(p[0])
		}
		idx.codes[r.Code] = r
		idx.entries = append(idx.entries, entry{r, [4]string{r.Code, normalize(r.Name), normalize(r.Pinyin), initials.String()}})
	}
	sort.Slice(idx.entries, func(i, j int) bool { return idx.entries[i].record.Code < idx.entries[j].record.Code })
	return idx, nil
}

func loadEmbedded(data []byte) (*Index, error) {
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, &EmbeddedDataError{Err: err}
	}
	idx, err := New(records)
	if err != nil {
		return nil, &EmbeddedDataError{Err: err}
	}
	return idx, nil
}

// Default returns the current embedded index; invalid embedded data panics.
func Default() *Index { return DefaultLoader.Current().Index() }

// Lookup returns the record with exactly this code, without normalization.
func (idx *Index) Lookup(code string) (Record, bool) { r, ok := idx.codes[code]; return r, ok }

func (idx *Index) Len() int { return len(idx.entries) }

// Search ranks exact, prefix, then substring matches; within each rank it orders
// by deprecation state then code. Empty queries return an empty slice.
func (idx *Index) Search(query string, options Options) ([]Record, error) {
	limit := options.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 0 {
		return nil, &ValidationError{Kind: ErrInvalidLimit, Field: "limit", Value: strconv.Itoa(options.Limit), RecordIndex: -1}
	}
	if !digits(options.CodePrefix) {
		return nil, &ValidationError{Kind: ErrInvalidCodePrefix, Field: "codePrefix", Value: options.CodePrefix, RecordIndex: -1}
	}
	q := normalize(query)
	result := make([]Record, 0)
	if q == "" {
		return result, nil
	}
	var buckets [9][]Record
	for _, e := range idx.entries {
		if !strings.HasPrefix(e.record.Code, options.CodePrefix) {
			continue
		}
		rank := 3
		for _, k := range e.keys {
			if k == q {
				rank = 0
				break
			}
			if strings.HasPrefix(k, q) {
				if rank > 1 {
					rank = 1
				}
			} else if strings.Contains(k, q) && rank > 2 {
				rank = 2
			}
		}
		if rank == 3 {
			continue
		}
		b := rank*3 + int(e.record.Deprecated)
		if len(buckets[b]) < limit {
			buckets[b] = append(buckets[b], e.record)
		}
	}
	for _, bucket := range buckets {
		n := limit - len(result)
		if len(bucket) > n {
			bucket = bucket[:n]
		}
		result = append(result, bucket...)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}
