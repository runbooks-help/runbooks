// SPDX-License-Identifier: FSL-1.1-MIT

package nullable

import (
	"encoding/json"
	"testing"
	"unicode/utf8"
)

// FuzzNullStringRoundTrip checks a scanned value survives Value and a JSON
// round trip unchanged. encoding/json replaces invalid UTF-8 with U+FFFD, so
// exact preservation only holds for valid UTF-8; otherwise marshalling must at
// least be stable.
func FuzzNullStringRoundTrip(f *testing.F) {
	f.Add("")
	f.Add("hello")
	f.Fuzz(func(t *testing.T, s string) {
		var n Null[string]
		if err := n.Scan(s); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		v, err := n.Value()
		if err != nil {
			t.Fatalf("Value: %v", err)
		}
		if v != s {
			t.Fatalf("Value = %v, want %q", v, s)
		}

		b, err := json.Marshal(n)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var back Null[string]
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if !back.Valid {
			t.Fatalf("json round trip lost validity: %+v", back)
		}
		if utf8.ValidString(s) {
			if back.V != s {
				t.Fatalf("json round trip: %q -> %q", s, back.V)
			}
			return
		}
		// encoding/json replaces invalid UTF-8 with U+FFFD, and how that
		// replacement is escaped varies by Go version; require stability only
		// once the value is valid.
		if !utf8.ValidString(back.V) {
			t.Fatalf("json round trip did not yield valid UTF-8: %q", back.V)
		}
		b2, err := json.Marshal(back)
		if err != nil {
			t.Fatalf("Marshal2: %v", err)
		}
		var again Null[string]
		if err := json.Unmarshal(b2, &again); err != nil {
			t.Fatalf("Unmarshal2: %v", err)
		}
		b3, err := json.Marshal(again)
		if err != nil {
			t.Fatalf("Marshal3: %v", err)
		}
		if string(b3) != string(b2) {
			t.Fatalf("json not stable for invalid UTF-8: %s -> %s", b2, b3)
		}
	})
}

// FuzzNullJSONUnmarshal checks arbitrary JSON never panics.
func FuzzNullJSONUnmarshal(f *testing.F) {
	f.Add([]byte("null"))
	f.Add([]byte(`"x"`))
	f.Add([]byte("["))
	f.Fuzz(func(t *testing.T, data []byte) {
		var n Null[string]
		_ = json.Unmarshal(data, &n)
	})
}
