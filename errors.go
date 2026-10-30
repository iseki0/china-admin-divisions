package admindivision

import (
	"errors"
	"fmt"
)

// Stable error categories for errors.Is. Error message text is diagnostic only.
var (
	ErrInvalidLimit       = errors.New("invalid search limit")
	ErrInvalidCodePrefix  = errors.New("invalid code prefix")
	ErrInvalidCode        = errors.New("invalid division code")
	ErrInvalidName        = errors.New("invalid division name")
	ErrInvalidPinyin      = errors.New("invalid pinyin")
	ErrInvalidDeprecation = errors.New("invalid deprecation")
	ErrDuplicateCode      = errors.New("duplicate division code")
	ErrEmbeddedData       = errors.New("cannot load embedded dataset")
)

// ValidationError describes a rejected input. Field uses JSON field names
// (code, name, pinyin, deprecated) or option names (limit, codePrefix).
// Value is its textual representation, not a message to parse.
// RecordIndex is zero-based for New and -1 for options or standalone JSON values.
// Code is the supplied record code for New, otherwise empty.
// Kind is one of the validation sentinel errors above.
type ValidationError struct {
	Kind        error
	Field       string
	Value       string
	RecordIndex int
	Code        string
}

func (e *ValidationError) Error() string {
	if e.RecordIndex >= 0 {
		return fmt.Sprintf("record[%d] code=%q: %s: %s=%q", e.RecordIndex, e.Code, e.Kind, e.Field, e.Value)
	}
	return fmt.Sprintf("%s: %s=%q", e.Kind, e.Field, e.Value)
}

func (e *ValidationError) Unwrap() error { return e.Kind }

// EmbeddedDataError wraps a JSON decoding or record validation failure.
// errors.Is(err, ErrEmbeddedData) identifies loading failures; errors.As and
// errors.Is also traverse Err to inspect the original cause.
type EmbeddedDataError struct{ Err error }

func (e *EmbeddedDataError) Error() string        { return fmt.Sprintf("%s: %v", ErrEmbeddedData, e.Err) }
func (e *EmbeddedDataError) Unwrap() error        { return e.Err }
func (e *EmbeddedDataError) Is(target error) bool { return target == ErrEmbeddedData }
