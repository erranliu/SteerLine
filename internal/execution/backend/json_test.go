package backend

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDecodeStrictJSON(t *testing.T) {
	const trailing = "codec-specific trailing data"
	for _, tc := range []struct {
		name, input, wantError string
		wantValue              int
	}{
		{"value", `{"value":3}`, "", 3},
		{"whitespace", " \n{\"value\":3}\t\n", "", 3},
		{"null", `null`, "", 0},
		{"duplicate_fields_keep_last_value", `{"value":2,"value":3}`, "", 3},
		{"unknown_field", `{"unknown":3}`, `json: unknown field "unknown"`, 0},
		{"second_value", `{"value":3} {}`, trailing, 3},
		{"second_null", `{"value":3} null`, trailing, 3},
		{"damaged_trailing_data", `{"value":3} !`, trailing, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got struct {
				Value int `json:"value"`
			}
			err := DecodeStrictJSON([]byte(tc.input), &got, trailing)
			if tc.wantError == "" && err != nil || tc.wantError != "" && (err == nil || err.Error() != tc.wantError) || got.Value != tc.wantValue {
				t.Fatalf("value=%d error=%v; want value=%d error=%q", got.Value, err, tc.wantValue, tc.wantError)
			}
		})
	}
	t.Run("first_decode_errors_keep_their_type", func(t *testing.T) {
		var value struct {
			Value int `json:"value"`
		}
		for _, input := range []string{"", " \n"} {
			if err := DecodeStrictJSON([]byte(input), &value, trailing); !errors.Is(err, io.EOF) {
				t.Fatalf("empty input: %v", err)
			}
		}
		for _, input := range []string{`{!}`, "\xef\xbb\xbf{\"value\":3}"} {
			var syntax *json.SyntaxError
			if err := DecodeStrictJSON([]byte(input), &value, trailing); !errors.As(err, &syntax) {
				t.Fatalf("syntax/BOM input: %v", err)
			}
		}
		var mismatch *json.UnmarshalTypeError
		if err := DecodeStrictJSON([]byte(`{"value":"three"}`), &value, trailing); !errors.As(err, &mismatch) {
			t.Fatalf("type mismatch: %v", err)
		}
		if err := DecodeStrictJSON([]byte(`{"value":`), &value, trailing); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("incomplete JSON: %v", err)
		}
		if err := DecodeStrictJSON([]byte(`{}`), nil, trailing); err == nil || !strings.Contains(err.Error(), "Unmarshal(nil)") {
			t.Fatalf("invalid destination: %v", err)
		}
	})
}
