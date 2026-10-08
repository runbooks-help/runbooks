// SPDX-License-Identifier: FSL-1.1-MIT

// Package pagination parses page/limit query parameters and builds the response
// envelope, so every paginated endpoint pages, defaults and clamps the same way.
package pagination

import (
	"math"
	"net/http"
	"strconv"
)

const (
	// DefaultLimit is the page size when the client sends no limit.
	DefaultLimit = 20
	// MaxLimit caps the page size, so a client cannot ask for everything.
	MaxLimit = 100
)

// Request is one parsed pagination request.
type Request struct {
	Page  int
	Limit int
}

// Parse reads page and limit from the query string. A missing or invalid value
// falls back to the default, and limit is clamped to MaxLimit.
func Parse(r *http.Request) Request {
	q := r.URL.Query()
	return Request{
		Page:  atoi(q.Get("page"), 1),
		Limit: clamp(atoi(q.Get("limit"), DefaultLimit), 1, MaxLimit),
	}
}

// Offset is the number of items to skip.
func (r Request) Offset() int { return (r.Page - 1) * r.Limit }

// Bounds returns the [start, end) slice of a total-length list for this page.
func (r Request) Bounds(total int) (start, end int) {
	start = clamp(r.Offset(), 0, total)
	return start, clamp(start+r.Limit, start, total)
}

// Response is the pagination envelope for a list response. Embed it in the list
// type so the fields flatten into the JSON body.
type Response struct {
	Page     int `json:"page"`
	Limit    int `json:"limit"`
	Total    int `json:"total"`
	LastPage int `json:"last_page"`
}

// BuildResponse builds the envelope for a total item count.
func BuildResponse(r Request, total int) Response {
	last := 0
	if r.Limit > 0 {
		last = int(math.Ceil(float64(total) / float64(r.Limit)))
	}
	return Response{Page: r.Page, Limit: r.Limit, Total: total, LastPage: last}
}

func atoi(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return def
	}
	return v
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
