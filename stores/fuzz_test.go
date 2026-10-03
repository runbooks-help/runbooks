// SPDX-License-Identifier: FSL-1.1-MIT

package stores

import "testing"

// FuzzTransportsRoundTrip checks the comma-separated encoding survives a full
// Scan → Value → Scan round trip, from both the string and []byte a driver may
// hand back.
func FuzzTransportsRoundTrip(f *testing.F) {
	f.Add([]byte("usb,nfc"))
	f.Add([]byte("a,b,c"))
	f.Add([]byte(""))
	f.Add([]byte(","))
	f.Add([]byte("a,"))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, src := range []any{string(data), data} {
			var tr Transports
			if err := tr.Scan(src); err != nil {
				t.Fatalf("Scan(%T): %v", src, err)
			}
			v, err := tr.Value()
			if err != nil {
				t.Fatalf("Value: %v", err)
			}
			if len(tr) == 0 {
				if v != nil {
					t.Fatalf("empty transports Value = %v, want nil", v)
				}
				continue
			}
			joined, ok := v.(string)
			if !ok {
				t.Fatalf("Value = %T, want string", v)
			}
			var back Transports
			if err := back.Scan(joined); err != nil {
				t.Fatalf("rescan: %v", err)
			}
			if len(back) != len(tr) {
				t.Fatalf("round trip len %d -> %d", len(tr), len(back))
			}
			for i := range tr {
				if back[i] != tr[i] {
					t.Fatalf("round trip elem %d: %q -> %q", i, tr[i], back[i])
				}
			}
		}
	})
}
