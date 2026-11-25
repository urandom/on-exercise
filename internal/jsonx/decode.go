// Package jsonx holds the strict JSON decoding shared by every input loader.
package jsonx

import (
	"encoding/json"
	"errors"
	"io"
)

// ErrTrailingData is returned when the input holds anything other than
// whitespace after its single JSON value.
var ErrTrailingData = errors.New("unexpected data after the JSON value")

// DecodeStrict decodes exactly one JSON value from r into v. Unknown object
// keys are rejected when decoding into a struct; maps accept any key. Anything
// after the value other than whitespace is rejected with ErrTrailingData.
func DecodeStrict(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		return err
	}

	// Decode stops after the first value. Token returns io.EOF only when
	// nothing but whitespace remains; a second value, stray characters or an
	// unmatched bracket all return something else.
	if _, err := dec.Token(); err != io.EOF {
		return ErrTrailingData
	}

	return nil
}
