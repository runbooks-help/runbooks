// SPDX-License-Identifier: FSL-1.1-MIT

package pagination

import (
	"net/http/httptest"
	"testing"
)

func parse(target string) Request {
	return Parse(httptest.NewRequest("GET", target, nil))
}

func TestParse(t *testing.T) {
	cases := []struct {
		target string
		want   Request
	}{
		{"/", Request{Page: 1, Limit: DefaultLimit}},
		{"/?page=3&limit=5", Request{Page: 3, Limit: 5}},
		{"/?page=0&limit=0", Request{Page: 1, Limit: DefaultLimit}}, // zero is invalid
		{"/?page=abc&limit=xyz", Request{Page: 1, Limit: DefaultLimit}},
		{"/?page=-2", Request{Page: 1, Limit: DefaultLimit}},
		{"/?limit=1000", Request{Page: 1, Limit: MaxLimit}}, // clamped, not defaulted
	}
	for _, c := range cases {
		if got := parse(c.target); got != c.want {
			t.Errorf("Parse(%q) = %+v, want %+v", c.target, got, c.want)
		}
	}
}

func TestOffsetAndBounds(t *testing.T) {
	r := Request{Page: 2, Limit: 2}
	if r.Offset() != 2 {
		t.Errorf("Offset = %d, want 2", r.Offset())
	}
	if start, end := r.Bounds(3); start != 2 || end != 3 {
		t.Errorf("Bounds(3) = %d,%d, want 2,3", start, end)
	}
	// A page past the end yields an empty slice, not an out-of-range one.
	if start, end := r.Bounds(1); start != 1 || end != 1 {
		t.Errorf("Bounds(1) = %d,%d, want 1,1", start, end)
	}
}

func TestBuildResponse(t *testing.T) {
	got := BuildResponse(Request{Page: 2, Limit: 20}, 45)
	want := Response{Page: 2, Limit: 20, Total: 45, LastPage: 3}
	if got != want {
		t.Errorf("BuildResponse = %+v, want %+v", got, want)
	}
	if empty := BuildResponse(Request{Page: 1, Limit: 20}, 0); empty.LastPage != 0 {
		t.Errorf("LastPage for an empty set = %d, want 0", empty.LastPage)
	}
}
