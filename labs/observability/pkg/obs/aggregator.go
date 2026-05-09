package obs

import (
	"bufio"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// LogAggregator — merge multiple structured log streams by timestamp
// ---------------------------------------------------------------------------

// parsedEntry is an internal representation used for merging.
type parsedEntry struct {
	raw       string
	timestamp time.Time
}

// LogAggregator reads JSON log lines from multiple io.Reader sources and
// merges them into a single time-ordered stream written to out.
//
// All lines in all sources must be valid JSON with a "timestamp" field in
// RFC3339Nano format (as produced by Logger). Lines that cannot be parsed are
// passed through as-is at the end of the merged stream.
func LogAggregator(out io.Writer, sources ...io.Reader) error {
	var all []parsedEntry

	for _, src := range sources {
		scanner := bufio.NewScanner(src)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var m map[string]any
			ts := time.Time{}
			if err := json.Unmarshal([]byte(line), &m); err == nil {
				if tsStr, ok := m["timestamp"].(string); ok {
					if t, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
						ts = t
					}
				}
			}
			all = append(all, parsedEntry{raw: line, timestamp: ts})
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	}

	// Stable sort by timestamp; zero timestamps sort to the end.
	sort.SliceStable(all, func(i, j int) bool {
		ti, tj := all[i].timestamp, all[j].timestamp
		if ti.IsZero() {
			return false
		}
		if tj.IsZero() {
			return true
		}
		return ti.Before(tj)
	})

	for _, e := range all {
		if _, err := io.WriteString(out, e.raw+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Alert evaluation
// ---------------------------------------------------------------------------

// Alert describes a Prometheus-style alerting rule.
type Alert struct {
	Name        string
	Expr        string            // "metric_name > threshold" (simple form)
	For         time.Duration     // minimum firing duration (not enforced in toy evaluator)
	Labels      map[string]string // extra labels attached when firing
	Annotations map[string]string // human-readable description
}

// FiringAlert is an Alert whose expression currently evaluates to true.
type FiringAlert struct {
	Alert
	CurrentValue float64
}

// EvaluateAlerts checks each alert expression against current metric values
// from registry. Only "metric_name OP threshold" expressions are supported
// (operators: >, <, >=, <=, ==).
func EvaluateAlerts(registry *MetricsRegistry, alerts []Alert) []FiringAlert {
	values := gatherValues(registry)
	var firing []FiringAlert
	for _, alert := range alerts {
		name, op, threshold, ok := parseExpr(alert.Expr)
		if !ok {
			continue
		}
		val, exists := values[name]
		if !exists {
			continue
		}
		var fires bool
		switch op {
		case ">":
			fires = val > threshold
		case "<":
			fires = val < threshold
		case ">=":
			fires = val >= threshold
		case "<=":
			fires = val <= threshold
		case "==":
			fires = val == threshold
		}
		if fires {
			firing = append(firing, FiringAlert{Alert: alert, CurrentValue: val})
		}
	}
	return firing
}

// gatherValues returns a flat map of metric names to scalar values.
// Histograms expose "<name>_count" and "<name>_sum" entries.
func gatherValues(registry *MetricsRegistry) map[string]float64 {
	families := registry.Gather()
	values := make(map[string]float64, len(families))
	for _, fam := range families {
		switch fam.Type {
		case MetricCounter, MetricGauge:
			if len(fam.Samples) > 0 {
				values[fam.Name] = fam.Samples[0].Value
			}
		case MetricHistogram:
			for _, s := range fam.Samples {
				switch s.Labels["__type__"] {
				case "count":
					values[fam.Name+"_count"] = s.Value
				case "sum":
					values[fam.Name+"_sum"] = s.Value
				}
			}
		}
	}
	return values
}

// parseExpr parses "metric_name OP threshold" into its components.
// Operators are tried longest-first to avoid ">" matching ">=" incorrectly.
func parseExpr(expr string) (name, op string, threshold float64, ok bool) {
	expr = strings.TrimSpace(expr)
	for _, candidate := range []string{">=", "<=", "==", ">", "<"} {
		idx := strings.Index(expr, candidate)
		if idx < 0 {
			continue
		}
		lhs := strings.TrimSpace(expr[:idx])
		rhs := strings.TrimSpace(expr[idx+len(candidate):])
		f, err := strconv.ParseFloat(rhs, 64)
		if err != nil {
			continue
		}
		return lhs, candidate, f, true
	}
	return "", "", 0, false
}
