// Command processor is a CLI demo for the stream-processor lab.
//
// Usage:
//
//	go run ./cmd/processor          # run the full demo
//	go run ./cmd/processor --stage v0   # tumbling window demo
//	go run ./cmd/processor --stage v1   # sliding window + watermark demo
//	go run ./cmd/processor --stage v2   # exactly-once 2PC demo
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/stream-processor/pkg/stream"
)

func main() {
	stage := flag.String("stage", "all", "which stage to demo: v0, v1, v2, or all")
	flag.Parse()

	switch *stage {
	case "v0", "all":
		demoV0()
		if *stage == "v0" {
			return
		}
	}

	switch *stage {
	case "v1", "all":
		demoV1()
		if *stage == "v1" {
			return
		}
	}

	switch *stage {
	case "v2", "all":
		demoV2()
	default:
		fmt.Fprintf(os.Stderr, "unknown stage %q\n", *stage)
		os.Exit(1)
	}
}

// demoV0 shows tumbling window aggregation over synthetic sensor events.
func demoV0() {
	fmt.Println("=== v0: Tumbling Window Aggregation ===")
	tw := stream.NewTumblingWindow(time.Minute)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	events := []stream.Event{
		{Key: "sensor-A", Value: 23.1, Timestamp: base.Add(10 * time.Second)},
		{Key: "sensor-A", Value: 24.5, Timestamp: base.Add(30 * time.Second)},
		{Key: "sensor-B", Value: 18.0, Timestamp: base.Add(45 * time.Second)},
		{Key: "sensor-A", Value: 22.8, Timestamp: base.Add(55 * time.Second)},
		// Next window
		{Key: "sensor-A", Value: 25.0, Timestamp: base.Add(90 * time.Second)},
		{Key: "sensor-B", Value: 19.2, Timestamp: base.Add(100 * time.Second)},
	}

	for _, e := range events {
		if results := tw.Process(e); len(results) > 0 {
			printResults("auto-flush on boundary", results)
		}
	}
	if results := tw.Flush(); len(results) > 0 {
		printResults("final flush", results)
	}
	fmt.Println()
}

// demoV1 shows sliding window + watermark with late event handling.
func demoV1() {
	fmt.Println("=== v1: Sliding Windows + Watermarks ===")

	cfg := stream.ProcessorConfig{
		WindowSize:      30 * time.Second,
		WindowStep:      10 * time.Second,
		AllowedLateness: 5 * time.Second,
	}
	sp := stream.NewStreamProcessor(cfg)
	sp.Start()

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Normal events.
	for i := 0; i < 40; i++ {
		sp.Source <- stream.Event{
			Key:       "temperature",
			Value:     float64(20 + i%10),
			Timestamp: base.Add(time.Duration(i) * time.Second),
		}
	}

	// Slightly late event within allowed lateness.
	sp.Source <- stream.Event{Key: "temperature", Value: 99, Timestamp: base.Add(3 * time.Second)}

	// Advance watermark far ahead to flush mature windows.
	sp.Source <- stream.Event{Key: "temperature", Value: 0, Timestamp: base.Add(5 * time.Minute)}

	// Too-late event.
	sp.Source <- stream.Event{Key: "temperature", Value: -999, Timestamp: base.Add(-10 * time.Second)}

	time.Sleep(100 * time.Millisecond)
	sp.Stop()

	var results []stream.WindowResult
	for r := range sp.Sink {
		results = append(results, r)
	}

	var late []stream.Event
	for {
		select {
		case e := <-sp.LateSink:
			late = append(late, e)
		default:
			goto done
		}
	}
done:

	fmt.Printf("  Sliding windows emitted: %d results\n", len(results))
	fmt.Printf("  Late events routed to side output: %d\n", len(late))
	fmt.Println()
}

// demoV2 shows exactly-once processing via 2PC with checkpoint.
func demoV2() {
	fmt.Println("=== v2: Exactly-Once via Two-Phase Commit ===")

	cpPath := "/tmp/stream-processor-demo-checkpoint.json"
	defer os.Remove(cpPath)
	defer os.Remove(cpPath + ".tmp")

	coord := stream.NewTxnCoordinator(cpPath)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	var totalWritten int
	sink := func(results []stream.WindowResult) error {
		totalWritten++
		for _, r := range results {
			fmt.Printf("  [sink] key=%s count=%d sum=%.1f avg=%.2f\n",
				r.Key, r.Count, r.Sum, r.Avg)
		}
		return nil
	}

	batches := [][]stream.Event{
		{
			{Key: "orders", Value: 150.0, Timestamp: base},
			{Key: "orders", Value: 220.0, Timestamp: base.Add(5 * time.Second)},
			{Key: "payments", Value: 370.0, Timestamp: base.Add(10 * time.Second)},
		},
		{
			{Key: "orders", Value: 80.0, Timestamp: base.Add(15 * time.Second)},
			{Key: "payments", Value: 210.0, Timestamp: base.Add(20 * time.Second)},
		},
	}

	for i, batch := range batches {
		if err := coord.ProcessBatch(batch, sink); err != nil {
			fmt.Fprintf(os.Stderr, "batch %d failed: %v\n", i, err)
			os.Exit(1)
		}
		off, _ := coord.CurrentOffset()
		fmt.Printf("  Batch %d committed. Source offset now: %d\n", i+1, off)
	}

	fmt.Printf("\n  Total sink calls: %d (no duplicates — 2PC guaranteed)\n\n", totalWritten)
}

func printResults(label string, results []stream.WindowResult) {
	for _, r := range results {
		fmt.Printf("  [%s] key=%s count=%d min=%.1f max=%.1f avg=%.2f [%s to %s]\n",
			label, r.Key, r.Count, r.Min, r.Max, r.Avg,
			r.WindowStart.Format("15:04:05"), r.WindowEnd.Format("15:04:05"))
	}
}
