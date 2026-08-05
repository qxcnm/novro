package database

import "testing"

func TestQuoteIdentifier(t *testing.T) {
	if got := quoteIdentifier("novro`db"); got != "`novro``db`" {
		t.Fatalf("quote identifier: got %q", got)
	}
}
