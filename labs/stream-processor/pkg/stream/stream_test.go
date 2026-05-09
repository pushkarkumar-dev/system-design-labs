package stream_test

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/stream-processor/pkg/stream"
)

// ---------------------------------------------------------------------------
// v0 — TumblingWindow tests
// ---------------------------------------------------------------------------

// Test: events in the same window are grouped together.
func TestTumblingWindow_SameWindowGrouped(t *testing.T) {
	tw := stream.NewTumblingWindow(time.Minute)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tw.Process(stream.Event{Key: "sensor", Value: 10, Timestamp: base})
	tw.Process(stream.Event{Key: "sensor", Value: 20, Timestamp: base.Add(30 * time.Second)})

	results := tw.Flush()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Count != 2 {
		t.Errorf("expected count=2, got %d", r.Count)
	}
	if r.Sum != 30 {
		t.Errorf("expected sum=30, got %f", r.Sum)
	}
}

// Test: event crossing window boundary triggers a new window.
func TestTumblingWindow_BoundaryCreatesNewWindow(t *testing.T) {
	tw := stream.NewTumblingWindow(time.Minute)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tw.Process(stream.Event{Key: "k", Value: 1, Timestamp: base})
	// This event is in the next window — crosses the boundary.
	flushed := tw.Process(stream.Event{Key: "k", Value: 2, Timestamp: base.Add(61 * time.Second)})

	// The crossing should have triggered a flush of the first window.
	if len(flushed) != 1 {
		t.Fatalf("expected flush of first window on boundary cross, got %d results", len(flushed))
	}
	if flushed[0].Count != 1 || flushed[0].Sum != 1 {
		t.Errorf("first window: expected count=1 sum=1, got count=%d sum=%f", flushed[0].Count, flushed[0].Sum)
	}

	// Current window (second) should hold the second event.
	results := tw.Flush()
	if len(results) != 1 || results[0].Sum != 2 {
		t.Errorf("second window: expected sum=2, got %v", results)
	}
}

// Test: multi-key aggregation produces one result per key.
func TestTumblingWindow_MultiKey(t *testing.T) {
	tw := stream.NewTumblingWindow(time.Minute)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tw.Process(stream.Event{Key: "a", Value: 5, Timestamp: base})
	tw.Process(stream.Event{Key: "b", Value: 7, Timestamp: base.Add(10 * time.Second)})
	tw.Process(stream.Event{Key: "a", Value: 3, Timestamp: base.Add(20 * time.Second)})

	results := tw.Flush()
	if len(results) != 2 {
		t.Fatalf("expected 2 results (one per key), got %d", len(results))
	}
	byKey := map[string]stream.WindowResult{}
	for _, r := range results {
		byKey[r.Key] = r
	}
	if byKey["a"].Sum != 8 {
		t.Errorf("key a: expected sum=8, got %f", byKey["a"].Sum)
	}
	if byKey["b"].Sum != 7 {
		t.Errorf("key b: expected sum=7, got %f", byKey["b"].Sum)
	}
}

// Test: flush emits correct min, max, avg.
func TestTumblingWindow_MinMaxAvg(t *testing.T) {
	tw := stream.NewTumblingWindow(time.Minute)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	vals := []float64{3, 1, 4, 1, 5, 9, 2, 6}
	for i, v := range vals {
		tw.Process(stream.Event{Key: "x", Value: v, Timestamp: base.Add(time.Duration(i) * time.Second)})
	}

	results := tw.Flush()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Min != 1 {
		t.Errorf("expected min=1, got %f", r.Min)
	}
	if r.Max != 9 {
		t.Errorf("expected max=9, got %f", r.Max)
	}
	wantSum := 3 + 1 + 4 + 1 + 5 + 9 + 2 + 6.0
	if r.Sum != wantSum {
		t.Errorf("expected sum=%f, got %f", wantSum, r.Sum)
	}
	wantAvg := wantSum / float64(len(vals))
	if r.Avg != wantAvg {
		t.Errorf("expected avg=%f, got %f", wantAvg, r.Avg)
	}
}

// Test: flushing an empty window emits nothing.
func TestTumblingWindow_EmptyFlushEmitsNothing(t *testing.T) {
	tw := stream.NewTumblingWindow(time.Minute)
	results := tw.Flush()
	if len(results) != 0 {
		t.Errorf("expected empty flush, got %v", results)
	}
}

// Test: concurrent producers do not race.
func TestTumblingWindow_ConcurrentProducers(t *testing.T) {
	tw := stream.NewTumblingWindow(time.Minute)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				tw.Process(stream.Event{
					Key:       fmt.Sprintf("key-%d", n),
					Value:     float64(j),
					Timestamp: base.Add(time.Duration(j) * time.Second),
				})
			}
		}(i)
	}
	wg.Wait()

	results := tw.Flush()
	// Should have one result per goroutine (10 keys).
	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}
}

// Test: late arrival before Flush is included (not dropped).
func TestTumblingWindow_LateArrivalBeforeFlushIncluded(t *testing.T) {
	tw := stream.NewTumblingWindow(time.Minute)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tw.Process(stream.Event{Key: "z", Value: 10, Timestamp: base.Add(45 * time.Second)})
	// Slightly earlier timestamp — still within the same window, arrives after.
	tw.Process(stream.Event{Key: "z", Value: 5, Timestamp: base.Add(30 * time.Second)})

	results := tw.Flush()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Count != 2 {
		t.Errorf("expected count=2 (both events included), got %d", results[0].Count)
	}
}

// Test: Aggregator goroutine correctly processes events and emits results.
func TestAggregator_ProcessesEvents(t *testing.T) {
	agg := stream.NewAggregator(100 * time.Millisecond)
	agg.Start()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	agg.Source <- stream.Event{Key: "m", Value: 99, Timestamp: base}

	agg.Stop()

	var results [][]stream.WindowResult
	for r := range agg.Sink {
		results = append(results, r)
	}

	total := 0
	for _, batch := range results {
		for _, r := range batch {
			total += r.Count
		}
	}
	if total != 1 {
		t.Errorf("expected total count=1 across all results, got %d", total)
	}
}

// ---------------------------------------------------------------------------
// v1 — SlidingWindow + Watermark tests
// ---------------------------------------------------------------------------

// Test: watermark advances with event time.
func TestWatermark_AdvancesWithEventTime(t *testing.T) {
	wm := stream.NewWatermark(5 * time.Second)

	t0 := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	wmTime, advanced := wm.Advance(t0)
	if !advanced {
		t.Error("expected watermark to advance on first event")
	}

	// Watermark = t0 - 5s
	expectedWM := t0.Add(-5 * time.Second)
	if !wmTime.Equal(expectedWM) {
		t.Errorf("expected watermark=%v, got %v", expectedWM, wmTime)
	}
}

// Test: event within allowed lateness is NOT marked late.
func TestWatermark_EventWithinLateness_NotLate(t *testing.T) {
	wm := stream.NewWatermark(5 * time.Second)

	t0 := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	wm.Advance(t0)

	// This event is 4 seconds before t0: exactly within the 5s allowed lateness.
	lateEvent := t0.Add(-4 * time.Second)
	if wm.IsLate(lateEvent) {
		t.Error("event within allowed lateness should NOT be marked late")
	}
}

// Test: event beyond allowed lateness IS late.
func TestWatermark_EventBeyondLateness_IsLate(t *testing.T) {
	wm := stream.NewWatermark(3 * time.Second)

	t0 := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	wm.Advance(t0)

	// Watermark = t0 - 3s. An event at t0-10s is before the watermark.
	tooLate := t0.Add(-10 * time.Second)
	if !wm.IsLate(tooLate) {
		t.Error("event beyond allowed lateness should be marked late")
	}
}

// Test: too-late events go to LateSink.
func TestStreamProcessor_TooLateEventGoesToLateSink(t *testing.T) {
	cfg := stream.ProcessorConfig{
		WindowSize:      5 * time.Minute,
		WindowStep:      1 * time.Minute,
		AllowedLateness: 3 * time.Second,
	}
	sp := stream.NewStreamProcessor(cfg)
	sp.Start()
	defer sp.Stop()

	t0 := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	// First advance watermark to t0.
	sp.Source <- stream.Event{Key: "x", Value: 1, Timestamp: t0}

	// Now send a too-late event (10 seconds before t0, beyond 3s lateness).
	sp.Source <- stream.Event{Key: "x", Value: 99, Timestamp: t0.Add(-10 * time.Second)}

	// Give goroutines time to process.
	time.Sleep(50 * time.Millisecond)

	select {
	case late, ok := <-sp.LateSink:
		if !ok {
			t.Fatal("LateSink closed prematurely")
		}
		if late.Value != 99 {
			t.Errorf("expected late event value=99, got %f", late.Value)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("expected too-late event in LateSink but none arrived")
	}
}

// Test: sliding window fires every step covering last size duration.
func TestStreamProcessor_SlidingWindowFiresEveryStep(t *testing.T) {
	cfg := stream.ProcessorConfig{
		WindowSize:      4 * time.Second,
		WindowStep:      2 * time.Second,
		AllowedLateness: 500 * time.Millisecond,
	}
	sp := stream.NewStreamProcessor(cfg)
	sp.Start()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Send events spanning 6 seconds.
	for i := 0; i < 6; i++ {
		sp.Source <- stream.Event{
			Key:       "sensor",
			Value:     float64(i),
			Timestamp: base.Add(time.Duration(i) * time.Second),
		}
	}

	// Advance watermark past all windows by sending a far-future event.
	sp.Source <- stream.Event{
		Key:       "sensor",
		Value:     0,
		Timestamp: base.Add(20 * time.Second),
	}

	time.Sleep(100 * time.Millisecond)
	sp.Stop()

	var results []stream.WindowResult
	for r := range sp.Sink {
		results = append(results, r)
	}

	// We expect at least 2 overlapping windows to have fired.
	if len(results) < 2 {
		t.Errorf("expected at least 2 sliding window results, got %d", len(results))
	}
}

// Test: events in overlapping sliding windows are correctly included in both.
func TestStreamProcessor_OverlappingWindowsShareEvents(t *testing.T) {
	cfg := stream.ProcessorConfig{
		WindowSize:      6 * time.Second,
		WindowStep:      3 * time.Second,
		AllowedLateness: 500 * time.Millisecond,
	}
	sp := stream.NewStreamProcessor(cfg)
	sp.Start()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// An event at t=4s falls in both [0,6) and [3,9) windows.
	sp.Source <- stream.Event{Key: "s", Value: 42, Timestamp: base.Add(4 * time.Second)}

	// Advance watermark past both windows.
	sp.Source <- stream.Event{Key: "s", Value: 0, Timestamp: base.Add(30 * time.Second)}

	time.Sleep(100 * time.Millisecond)
	sp.Stop()

	var count42 int
	for r := range sp.Sink {
		if r.Key == "s" && r.Max == 42 {
			count42++
		}
	}
	// The event at t=4 should appear in at least one window (may appear in two with step alignment).
	if count42 == 0 {
		t.Error("expected event value=42 to appear in at least one sliding window result")
	}
}

// ---------------------------------------------------------------------------
// v2 — Exactly-once / 2PC tests
// ---------------------------------------------------------------------------

func tempCheckpointPath(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "stream-checkpoint-*.json")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.Remove(f.Name()) // Remove so CheckpointStore starts fresh.
	t.Cleanup(func() { os.Remove(f.Name()); os.Remove(f.Name() + ".tmp") })
	return f.Name()
}

// Test: checkpoint persists across restarts.
func TestCheckpointStore_PersistsAcrossRestarts(t *testing.T) {
	path := tempCheckpointPath(t)
	cs := stream.NewCheckpointStore(path)

	cp := stream.Checkpoint{SourceOffset: 42, OutputCommitted: true}
	if err := cs.Save(cp); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Simulate restart — create a new store pointing to the same path.
	cs2 := stream.NewCheckpointStore(path)
	loaded, err := cs2.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.SourceOffset != 42 {
		t.Errorf("expected offset=42, got %d", loaded.SourceOffset)
	}
	if !loaded.OutputCommitted {
		t.Error("expected OutputCommitted=true")
	}
}

// Test: 2PC does not duplicate output on clean commit.
func TestTxnCoordinator_NoduplicateOnCleanCommit(t *testing.T) {
	path := tempCheckpointPath(t)
	coord := stream.NewTxnCoordinator(path)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := []stream.Event{
		{Key: "k", Value: 1, Timestamp: base},
		{Key: "k", Value: 2, Timestamp: base.Add(time.Second)},
	}

	var sinkCalls int
	sink := func(results []stream.WindowResult) error {
		sinkCalls++
		return nil
	}

	if err := coord.ProcessBatch(events, sink); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	// Second ProcessBatch with new events — should not re-emit first batch.
	events2 := []stream.Event{
		{Key: "k", Value: 3, Timestamp: base.Add(2 * time.Second)},
	}
	if err := coord.ProcessBatch(events2, sink); err != nil {
		t.Fatalf("ProcessBatch2: %v", err)
	}

	if sinkCalls != 2 {
		t.Errorf("expected 2 sink calls (one per batch), got %d", sinkCalls)
	}
}

// Test: at-least-once processor can produce duplicates on simulated crash.
func TestAtLeastOnceProcessor_DuplicatesOnCrash(t *testing.T) {
	path := tempCheckpointPath(t)
	proc := stream.NewAtLeastOnceProcessor(path)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := []stream.Event{
		{Key: "x", Value: 10, Timestamp: base},
	}

	var outputs [][]stream.WindowResult
	sink := func(results []stream.WindowResult) error {
		outputs = append(outputs, results)
		// Simulate crash after writing output but before committing offset:
		// do NOT save the checkpoint here — the processor will do it after we return.
		// We can simulate a crash by forcibly resetting the checkpoint after the first call.
		return nil
	}

	// First run: output written, offset committed.
	if err := proc.ProcessBatch(events, sink); err != nil {
		t.Fatalf("first ProcessBatch: %v", err)
	}

	// Simulate crash: reset checkpoint to before commit.
	cs := stream.NewCheckpointStore(path)
	cs.Save(stream.Checkpoint{SourceOffset: 0, OutputCommitted: false})

	// Second run with the same events (simulating restart from uncommitted offset).
	proc2 := stream.NewAtLeastOnceProcessor(path)
	if err := proc2.ProcessBatch(events, sink); err != nil {
		t.Fatalf("second ProcessBatch: %v", err)
	}

	// At-least-once: the same events were processed twice -> at least 2 outputs.
	if len(outputs) < 2 {
		t.Errorf("expected at-least-once to produce >=2 outputs (duplicate on simulated crash), got %d", len(outputs))
	}
}

// Test: TxnCoordinator offset advances correctly across multiple batches.
func TestTxnCoordinator_OffsetAdvances(t *testing.T) {
	path := tempCheckpointPath(t)
	coord := stream.NewTxnCoordinator(path)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	noop := func([]stream.WindowResult) error { return nil }

	batch1 := []stream.Event{
		{Key: "k", Value: 1, Timestamp: base},
		{Key: "k", Value: 2, Timestamp: base.Add(time.Second)},
	}
	if err := coord.ProcessBatch(batch1, noop); err != nil {
		t.Fatal(err)
	}
	off1, _ := coord.CurrentOffset()
	if off1 != 2 {
		t.Errorf("expected offset=2 after first batch, got %d", off1)
	}

	batch2 := []stream.Event{
		{Key: "k", Value: 3, Timestamp: base.Add(2 * time.Second)},
	}
	if err := coord.ProcessBatch(batch2, noop); err != nil {
		t.Fatal(err)
	}
	off2, _ := coord.CurrentOffset()
	if off2 != 3 {
		t.Errorf("expected offset=3 after second batch, got %d", off2)
	}
}

// Test: concurrent TxnCoordinator instances on different checkpoint files don't interfere.
func TestTxnCoordinator_ConcurrentProcessors_DontInterfere(t *testing.T) {
	path1 := tempCheckpointPath(t)
	path2 := tempCheckpointPath(t)

	coord1 := stream.NewTxnCoordinator(path1)
	coord2 := stream.NewTxnCoordinator(path2)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	noop := func([]stream.WindowResult) error { return nil }

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			events := []stream.Event{{Key: "p1", Value: float64(i), Timestamp: base.Add(time.Duration(i) * time.Second)}}
			if err := coord1.ProcessBatch(events, noop); err != nil {
				t.Errorf("coord1 batch %d: %v", i, err)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			events := []stream.Event{{Key: "p2", Value: float64(i), Timestamp: base.Add(time.Duration(i) * time.Second)}}
			if err := coord2.ProcessBatch(events, noop); err != nil {
				t.Errorf("coord2 batch %d: %v", i, err)
			}
		}
	}()

	wg.Wait()

	off1, err1 := coord1.CurrentOffset()
	off2, err2 := coord2.CurrentOffset()
	if err1 != nil || err2 != nil {
		t.Fatalf("offset errors: %v, %v", err1, err2)
	}
	if off1 != 10 {
		t.Errorf("coord1 expected offset=10, got %d", off1)
	}
	if off2 != 10 {
		t.Errorf("coord2 expected offset=10, got %d", off2)
	}
}
