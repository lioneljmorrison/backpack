// Package board holds the board's operations and enforces its rules. Every
// board action is one FalkorDB query, its rule checks included.
package board

import "strings"

// digits is in ASCII order, so keys compare the same in Go and in FalkorDB.
const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Between returns a rank key strictly between a and b, where a < b. An empty
// a means "no story above" and an empty b "no story below". Keys never end in
// '0', so there is always room for another key above any of them.
func Between(a, b string) string {
	return midpoint(a, b, b != "")
}

func midpoint(a, b string, bounded bool) string {
	if bounded {
		// Keep the common prefix; a missing digit in a counts as '0'.
		n := 0
		for n < len(b) && digitAt(a, n) == b[n] {
			n++
		}
		if n > 0 {
			return b[:n] + midpoint(tail(a, n), b[n:], true)
		}
	}
	lo := strings.IndexByte(digits, digitAt(a, 0))
	hi := len(digits)
	if bounded {
		hi = strings.IndexByte(digits, b[0])
	}
	if hi-lo > 1 {
		return string(digits[(lo+hi+1)/2])
	}
	if bounded && len(b) > 1 {
		return b[:1]
	}
	return string(digits[lo]) + midpoint(tail(a, 1), "", false)
}

func digitAt(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return '0'
}

func tail(s string, n int) string {
	if n < len(s) {
		return s[n:]
	}
	return ""
}
