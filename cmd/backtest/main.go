package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/paul/nhl-prop-model/internal/backtest"
)

func main() {
	// Parse command line flags
	daysBack := flag.Int("days", 30, "Number of days to backtest")
	startDateStr := flag.String("start", "", "Start date (YYYY-MM-DD)")
	endDateStr := flag.String("end", "", "End date (YYYY-MM-DD)")
	version := flag.Int("v", 2, "Backtest version (1 or 2)")
	flag.Parse()

	var startDate, endDate time.Time
	var err error

	if *startDateStr != "" && *endDateStr != "" {
		startDate, err = time.Parse("2006-01-02", *startDateStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid start date: %v\n", err)
			os.Exit(1)
		}
		endDate, err = time.Parse("2006-01-02", *endDateStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid end date: %v\n", err)
			os.Exit(1)
		}
	} else {
		endDate = time.Now().AddDate(0, 0, -1) // Yesterday
		startDate = endDate.AddDate(0, 0, -*daysBack)
	}

	fmt.Printf("NHL Prop Model Backtest (V%d)\n", *version)
	fmt.Printf("============================\n")
	fmt.Printf("Running backtest from %s to %s...\n\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	if *version == 1 {
		backtester := backtest.NewBacktester()
		result, err := backtester.RunBacktest(startDate, endDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Backtest failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(result.FormatResults())
	} else {
		backtester := backtest.NewBacktesterV2()
		result, err := backtester.RunBacktestV2(startDate, endDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Backtest failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(result.FormatResultsV2())
	}
}
