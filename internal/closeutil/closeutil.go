// Package closeutil provides helpers for closing resources whose error is
// not actionable on the cleanup path (response bodies, deferred file handles).
package closeutil

import "io"

// Quiet closes c and discards any error. Intended use: defer closeutil.Quiet(c).
func Quiet(c io.Closer) {
	_ = c.Close()
}
