package admindivision

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestValidationErrors(t *testing.T) {
	good := Record{Code: "1", Name: "北京", Pinyin: "bei jing"}
	tests := []struct {
		kind         error
		field, value string
		modify       func(*Record)
	}{
		{ErrInvalidCode, "code", "x", func(r *Record) { r.Code = "x" }},
		{ErrInvalidName, "name", "", func(r *Record) { r.Name = "" }},
		{ErrInvalidPinyin, "pinyin", "123", func(r *Record) { r.Pinyin = "123" }},
		{ErrInvalidPinyin, "pinyin", "", func(r *Record) { r.Pinyin = "" }},
		{ErrInvalidDeprecation, "deprecated", "5", func(r *Record) { r.Deprecated = 5 }},
		{ErrDuplicateCode, "code", "1", func(r *Record) { r.Code = "1" }},
	}
	for _, tt := range tests {
		t.Run(tt.kind.Error()+tt.value, func(t *testing.T) {
			bad := good
			bad.Code = "2"
			tt.modify(&bad)
			_, err := New([]Record{good, bad})
			err = fmt.Errorf("caller: %w", err)
			var detail *ValidationError
			if !errors.Is(err, tt.kind) || !errors.As(err, &detail) {
				t.Fatalf("unexpected error: %v", err)
			}
			if detail.Field != tt.field || detail.Value != tt.value || detail.RecordIndex != 1 || detail.Code != bad.Code {
				t.Fatalf("wrong details: %+v", detail)
			}
			if errors.Is(err, ErrEmbeddedData) {
				t.Fatal("New is not embedded loading")
			}
		})
	}
	var idx Index
	for _, tt := range []struct {
		options      Options
		kind         error
		field, value string
	}{
		{Options{Limit: -1}, ErrInvalidLimit, "limit", "-1"},
		{Options{CodePrefix: "x"}, ErrInvalidCodePrefix, "codePrefix", "x"},
	} {
		_, err := idx.Search("", tt.options)
		var detail *ValidationError
		if !errors.Is(err, tt.kind) || !errors.As(err, &detail) {
			t.Fatal(err)
		}
		if detail.Field != tt.field || detail.Value != tt.value || detail.RecordIndex != -1 || detail.Code != "" {
			t.Fatalf("wrong option details: %+v", detail)
		}
	}
}

func TestWrappedErrors(t *testing.T) {
	var d Deprecation
	err := json.Unmarshal([]byte(`"unknown"`), &d)
	var validation *ValidationError
	if !errors.Is(err, ErrInvalidDeprecation) || !errors.As(err, &validation) || validation.RecordIndex != -1 {
		t.Fatal(err)
	}
	_, err = json.Marshal(Deprecation(99))
	var marshal *json.MarshalerError
	if !errors.Is(err, ErrInvalidDeprecation) || !errors.As(err, &marshal) || !errors.As(err, &validation) {
		t.Fatal(err)
	}

	_, err = loadEmbedded([]byte(`[`))
	var loading *EmbeddedDataError
	var syntax *json.SyntaxError
	if !errors.Is(err, ErrEmbeddedData) || !errors.As(err, &loading) || !errors.As(err, &syntax) {
		t.Fatal(err)
	}
	_, err = loadEmbedded([]byte(`[{"code":"x","name":"北京","pinyin":"bei jing","deprecated":false}]`))
	if !errors.Is(err, ErrEmbeddedData) || !errors.Is(err, ErrInvalidCode) || !errors.As(err, &validation) {
		t.Fatal(err)
	}
	if validation.RecordIndex != 0 {
		t.Fatal(validation)
	}
	_, err = loadEmbedded([]byte(`123`))
	var typeError *json.UnmarshalTypeError
	if !errors.Is(err, ErrEmbeddedData) || !errors.As(err, &typeError) {
		t.Fatal(err)
	}
}
