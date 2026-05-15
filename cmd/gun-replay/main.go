// gun-replay reads a Gun journal directory and replays each symbol's
// write-ahead log into a fresh, in-memory Orderbook. It prints the
// resulting top-of-book depth, total resting orders, and the final
// sequence number per symbol.
//
// This is the incident-response tool: "what does the book look like at
// the moment of the crash?" without touching the running engine or
// dragging Redis into the picture.
//
// Usage:
//
//	gun-replay -dir ./data/journal
//	gun-replay -dir ./data/journal -symbol BTC_USDT -depth 5
//	gun-replay -dir ./data/journal -json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aliraad79/Gun/journal"
	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
)

func main() {
	dir := flag.String("dir", "./data/journal", "journal directory")
	symbolFlag := flag.String("symbol", "", "single symbol to replay (default: all symbols in dir)")
	depth := flag.Int("depth", 10, "top N price levels to print per side")
	asJSON := flag.Bool("json", false, "emit a JSON report instead of human-readable output")
	flag.Parse()

	symbols, err := listSymbols(*dir, *symbolFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gun-replay:", err)
		os.Exit(1)
	}
	if len(symbols) == 0 {
		fmt.Fprintln(os.Stderr, "gun-replay: no journals found in", *dir)
		os.Exit(1)
	}

	j, err := journal.NewFileJournal(*dir, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gun-replay: open journal:", err)
		os.Exit(1)
	}
	defer j.Close()

	reports := make([]symbolReport, 0, len(symbols))
	for _, symbol := range symbols {
		rep, err := replayOne(j, symbol, *depth)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gun-replay:", symbol, ":", err)
			os.Exit(1)
		}
		reports = append(reports, rep)
	}

	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(reports)
		return
	}
	for _, r := range reports {
		printHuman(r)
	}
}

type levelRow struct {
	Price string `json:"price"`
	Qty   string `json:"qty"`
}

type symbolReport struct {
	Symbol      string     `json:"symbol"`
	OpsReplayed int        `json:"ops_replayed"`
	Seq         uint64     `json:"seq"`
	BuyLevels   int        `json:"buy_levels"`
	SellLevels  int        `json:"sell_levels"`
	BuyOrders   int        `json:"buy_orders"`
	SellOrders  int        `json:"sell_orders"`
	BestBid     string     `json:"best_bid,omitempty"`
	BestAsk     string     `json:"best_ask,omitempty"`
	Bids        []levelRow `json:"bids,omitempty"`
	Asks        []levelRow `json:"asks,omitempty"`
}

func replayOne(j *journal.FileJournal, symbol string, depth int) (symbolReport, error) {
	book := models.NewOrderbook(symbol)
	ops := 0

	err := j.Replay(symbol, func(rec journal.Record) error {
		ops++
		switch rec.Kind {
		case journal.RecNew:
			_ = matchEngine.MatchAndAddNewOrder(book, rec.Order)
		case journal.RecCancel:
			_ = matchEngine.CancelOrder(book, rec.OrderID)
		case journal.RecModify:
			_ = matchEngine.ModifyOrder(book, rec.OrderID, rec.NewPrice, rec.NewVolume)
		}
		return nil
	})
	if err != nil {
		return symbolReport{}, err
	}

	rep := symbolReport{
		Symbol:      symbol,
		OpsReplayed: ops,
		Seq:         book.Seq(),
		BuyLevels:   book.LevelCount(models.BUY),
		SellLevels:  book.LevelCount(models.SELL),
		BuyOrders:   book.OrderCount(models.BUY),
		SellOrders:  book.OrderCount(models.SELL),
	}
	if len(book.Buy) > 0 {
		rep.BestBid = book.Buy[0].Price.String()
	}
	if len(book.Sell) > 0 {
		rep.BestAsk = book.Sell[0].Price.String()
	}
	for i, l := range book.Buy {
		if i >= depth {
			break
		}
		rep.Bids = append(rep.Bids, levelRow{Price: l.Price.String(), Qty: l.TotalQty().String()})
	}
	for i, l := range book.Sell {
		if i >= depth {
			break
		}
		rep.Asks = append(rep.Asks, levelRow{Price: l.Price.String(), Qty: l.TotalQty().String()})
	}
	return rep, nil
}

func listSymbols(dir, only string) ([]string, error) {
	if only != "" {
		return []string{only}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".journal") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".journal"))
	}
	sort.Strings(out)
	return out, nil
}

func printHuman(r symbolReport) {
	fmt.Printf("== %s ==\n", r.Symbol)
	fmt.Printf("  ops replayed : %d\n", r.OpsReplayed)
	fmt.Printf("  next seq     : %d\n", r.Seq+1)
	fmt.Printf("  resting      : buy %d orders @ %d levels   sell %d orders @ %d levels\n",
		r.BuyOrders, r.BuyLevels, r.SellOrders, r.SellLevels)
	if r.BestBid != "" && r.BestAsk != "" {
		fmt.Printf("  spread       : %s / %s\n", r.BestBid, r.BestAsk)
	} else if r.BestBid != "" {
		fmt.Printf("  best bid     : %s  (no asks)\n", r.BestBid)
	} else if r.BestAsk != "" {
		fmt.Printf("  best ask     : %s  (no bids)\n", r.BestAsk)
	} else {
		fmt.Printf("  book empty\n")
	}

	width := 0
	for _, side := range [][]levelRow{r.Bids, r.Asks} {
		for _, l := range side {
			if len(l.Price) > width {
				width = len(l.Price)
			}
		}
	}
	if len(r.Bids) > 0 || len(r.Asks) > 0 {
		fmt.Printf("\n  %-*s | %-*s   %-*s | %-*s\n",
			width, "bid price", 18, "qty",
			width, "ask price", 18, "qty")
		fmt.Printf("  %s-+-%s-+-%s-+-%s\n",
			strings.Repeat("-", width), strings.Repeat("-", 18),
			strings.Repeat("-", width), strings.Repeat("-", 18))
		rows := len(r.Bids)
		if len(r.Asks) > rows {
			rows = len(r.Asks)
		}
		for i := 0; i < rows; i++ {
			var bp, bq, ap, aq string
			if i < len(r.Bids) {
				bp, bq = r.Bids[i].Price, r.Bids[i].Qty
			}
			if i < len(r.Asks) {
				ap, aq = r.Asks[i].Price, r.Asks[i].Qty
			}
			fmt.Printf("  %-*s | %-*s   %-*s | %-*s\n",
				width, bp, 18, bq, width, ap, 18, aq)
		}
	}
	_ = filepath.Separator // (keep filepath import in case future expansion needs it)
	fmt.Println()
}
