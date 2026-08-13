package database

import "testing"

/**
 * TestQuoteIdentifier 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestQuoteIdentifier(t *testing.T) {
	if got := quoteIdentifier("novro`db"); got != "`novro``db`" {
		t.Fatalf("quote identifier: got %q", got)
	}
}
