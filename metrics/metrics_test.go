package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aliraad79/Gun/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// /metrics endpoint serves the standard Prometheus text format plus the
// expected gun_* family names.
func TestHandler_ServesGunMetrics(t *testing.T) {
	// drive at least one observation so the histograms appear in output
	metrics.Order("BTC_USDT", "accepted")
	metrics.Matches("BTC_USDT", 3)
	metrics.OpDuration("BTC_USDT", "new", 250*time.Nanosecond)
	metrics.JournalAppendDuration(800 * time.Nanosecond)
	metrics.BookDepth("BTC_USDT", 12, 240, 11, 198)
	metrics.MarketCount(7)

	srv := httptest.NewServer(metrics.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	s := string(body)

	for _, name := range []string{
		"gun_orders_total",
		"gun_matches_total",
		"gun_op_duration_seconds",
		"gun_journal_append_duration_seconds",
		"gun_book_levels",
		"gun_book_orders",
		"gun_active_markets",
	} {
		assert.True(t, strings.Contains(s, name),
			"expected /metrics to expose %s, got:\n%s", name, s)
	}

	// label sanity
	assert.Contains(t, s, `symbol="BTC_USDT"`)
	assert.Contains(t, s, `result="accepted"`)
}
