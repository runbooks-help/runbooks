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
		b2, err := json.Marshal(back)
		if err != nil {
			t.Fatalf("Marshal2: %v", err)
		}
		if string(b2) != string(b) {
			t.Fatalf("json not stable for invalid UTF-8: %s -> %s", b, b2)
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
