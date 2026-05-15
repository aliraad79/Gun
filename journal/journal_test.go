package journal_test

import (
	"path/filepath"
	"testing"

	"github.com/aliraad79/Gun/journal"
	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileJournal_AppendReplayRoundTrip(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.NewFileJournal(dir, false)
	require.NoError(t, err)
	defer j.Close()

	rec := journal.Record{
		Kind: journal.RecNew,
		Order: models.Order{
			ID: 42, Symbol: "BTC_USDT", Side: models.BUY, Type: models.LIMIT,
			Price: models.Px(10000_0000_0000), Volume: models.Qty(1_0000_0000),
		},
	}
	require.NoError(t, j.Append("BTC_USDT", rec))

	var got []journal.Record
	require.NoError(t, j.Replay("BTC_USDT", func(r journal.Record) error {
		got = append(got, r)
		return nil
	}))

	require.Len(t, got, 1)
	assert.Equal(t, rec.Kind, got[0].Kind)
	assert.Equal(t, rec.Order.ID, got[0].Order.ID)
	assert.Equal(t, rec.Order.Symbol, got[0].Order.Symbol)
	assert.Equal(t, rec.Order.Price, got[0].Order.Price)
	assert.Equal(t, rec.Order.Volume, got[0].Order.Volume)
	assert.NotZero(t, got[0].Timestamp, "Timestamp should be auto-populated when zero")
}

func TestFileJournal_MissingSymbolIsNotError(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.NewFileJournal(dir, false)
	require.NoError(t, err)
	defer j.Close()

	count := 0
	err = j.Replay("UNKNOWN_USDT", func(journal.Record) error {
		count++
		return nil
	})
	assert.NoError(t, err)
	assert.Zero(t, count)
}

func TestFileJournal_ManySymbolsIsolated(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.NewFileJournal(dir, false)
	require.NoError(t, err)
	defer j.Close()

	require.NoError(t, j.Append("BTC_USDT", journal.Record{Kind: journal.RecNew, Order: models.Order{ID: 1}}))
	require.NoError(t, j.Append("ETH_USDT", journal.Record{Kind: journal.RecNew, Order: models.Order{ID: 100}}))
	require.NoError(t, j.Append("BTC_USDT", journal.Record{Kind: journal.RecCancel, OrderID: 1}))

	var btc, eth []journal.Record
	require.NoError(t, j.Replay("BTC_USDT", func(r journal.Record) error { btc = append(btc, r); return nil }))
	require.NoError(t, j.Replay("ETH_USDT", func(r journal.Record) error { eth = append(eth, r); return nil }))

	assert.Len(t, btc, 2)
	assert.Len(t, eth, 1)
	assert.Equal(t, journal.RecCancel, btc[1].Kind)
}

// Sanitization: a symbol containing a path separator must not escape the
// journal directory.
func TestFileJournal_SymbolSanitization(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.NewFileJournal(dir, false)
	require.NoError(t, err)
	defer j.Close()

	// /etc/passwd would otherwise write outside dir
	require.NoError(t, j.Append("/etc/passwd", journal.Record{Kind: journal.RecNew}))

	// directory above journal dir should contain no extra files
	parent := filepath.Dir(dir)
	entries, err := filepath.Glob(filepath.Join(parent, "passwd*"))
	require.NoError(t, err)
	assert.Empty(t, entries, "symbol must not escape its journal directory")
}

func TestDiscardJournal_NoOp(t *testing.T) {
	var d journal.Discard
	assert.NoError(t, d.Append("X", journal.Record{Kind: journal.RecNew}))
	assert.NoError(t, d.Replay("X", func(journal.Record) error {
		t.Fatal("Discard.Replay should not invoke fn")
		return nil
	}))
	assert.NoError(t, d.Close())
}
