package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// field bounds: [min, max] inclusive for each cron position.
var fieldBounds = [5][2]int{
	{0, 59}, // minute
	{0, 23}, // hour
	{1, 31}, // day-of-month
	{1, 12}, // month
	{0, 6},  // day-of-week (0 = Sunday)
}

// CronSchedule is a parsed 5-field cron expression.
// Each field is a set of allowed values (bit-set represented as []bool).
type CronSchedule struct {
	expr    string
	minutes [60]bool
	hours   [24]bool
	doms    [32]bool // 1-31
	months  [13]bool // 1-12
	dows    [7]bool  // 0-6
}

// Parse parses a 5-field cron expression string.
// Returns an error if the expression is invalid.
//
// Supported syntax per field:
//
//	*     any value
//	n     exact value
//	*/n   step (every n)
//	n-m   range inclusive
//	a,b   list of values or sub-expressions
func Parse(expr string) (*CronSchedule, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(parts), expr)
	}

	cs := &CronSchedule{expr: expr}
	targets := []interface{}{&cs.minutes, &cs.hours, &cs.doms, &cs.months, &cs.dows}

	for i, part := range parts {
		lo, hi := fieldBounds[i][0], fieldBounds[i][1]
		vals, err := parseField(part, lo, hi)
		if err != nil {
			return nil, fmt.Errorf("cron: field %d (%q) in %q: %w", i, part, expr, err)
		}
		switch t := targets[i].(type) {
		case *[60]bool:
			for _, v := range vals {
				t[v] = true
			}
		case *[24]bool:
			for _, v := range vals {
				t[v] = true
			}
		case *[32]bool:
			for _, v := range vals {
				t[v] = true
			}
		case *[13]bool:
			for _, v := range vals {
				t[v] = true
			}
		case *[7]bool:
			for _, v := range vals {
				t[v] = true
			}
		}
	}
	return cs, nil
}

// parseField parses a single cron field part (which may be a comma-separated
// list of sub-expressions) and returns all matching integers in [lo, hi].
func parseField(part string, lo, hi int) ([]int, error) {
	var result []int
	for _, segment := range strings.Split(part, ",") {
		vals, err := parseSegment(segment, lo, hi)
		if err != nil {
			return nil, err
		}
		result = append(result, vals...)
	}
	return result, nil
}

// parseSegment handles *, n, */n, n-m for a single segment.
func parseSegment(seg string, lo, hi int) ([]int, error) {
	// Step: */n or n-m/n
	if strings.Contains(seg, "/") {
		halves := strings.SplitN(seg, "/", 2)
		step, err := strconv.Atoi(halves[1])
		if err != nil || step < 1 {
			return nil, fmt.Errorf("invalid step %q", halves[1])
		}
		rangeLo, rangeHi := lo, hi
		if halves[0] != "*" {
			// n/step — start from n
			n, err := strconv.Atoi(halves[0])
			if err != nil {
				return nil, fmt.Errorf("invalid range start %q", halves[0])
			}
			rangeLo = n
		}
		var vals []int
		for v := rangeLo; v <= rangeHi; v += step {
			vals = append(vals, v)
		}
		return vals, nil
	}

	// Wildcard
	if seg == "*" {
		vals := make([]int, 0, hi-lo+1)
		for v := lo; v <= hi; v++ {
			vals = append(vals, v)
		}
		return vals, nil
	}

	// Range: n-m
	if strings.Contains(seg, "-") {
		halves := strings.SplitN(seg, "-", 2)
		start, err1 := strconv.Atoi(halves[0])
		end, err2 := strconv.Atoi(halves[1])
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("invalid range %q", seg)
		}
		if start < lo || end > hi || start > end {
			return nil, fmt.Errorf("range %q out of bounds [%d,%d]", seg, lo, hi)
		}
		vals := make([]int, 0, end-start+1)
		for v := start; v <= end; v++ {
			vals = append(vals, v)
		}
		return vals, nil
	}

	// Exact value
	n, err := strconv.Atoi(seg)
	if err != nil {
		return nil, fmt.Errorf("invalid value %q", seg)
	}
	if n < lo || n > hi {
		return nil, fmt.Errorf("value %d out of bounds [%d,%d]", n, lo, hi)
	}
	return []int{n}, nil
}

// Next returns the next time at or after (from + 1 minute) that matches this
// cron schedule. It iterates minute-by-minute up to a 4-year window.
// Returns the zero time if no match is found (should not happen for valid expressions).
func (cs *CronSchedule) Next(from time.Time) time.Time {
	// Start from the next minute boundary.
	t := from.Add(time.Minute).Truncate(time.Minute)

	// Search at most 4 years (2,102,400 minutes) to handle sparse expressions.
	limit := t.Add(4 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		// Month check (1-12)
		if !cs.months[t.Month()] {
			// Skip to the 1st of the next matching month.
			t = advanceToNextMonth(t, cs)
			continue
		}
		// Day-of-month check (1-31)
		if !cs.doms[t.Day()] {
			t = t.AddDate(0, 0, 1).Truncate(24 * time.Hour)
			continue
		}
		// Day-of-week check (0=Sunday)
		if !cs.dows[int(t.Weekday())] {
			t = t.AddDate(0, 0, 1).Truncate(24 * time.Hour)
			continue
		}
		// Hour check
		if !cs.hours[t.Hour()] {
			t = t.Add(time.Hour).Truncate(time.Hour)
			continue
		}
		// Minute check
		if !cs.minutes[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{} // no match (should not happen for valid expressions)
}

// advanceToNextMonth moves t to the first minute of the next month that
// matches the schedule's months field.
func advanceToNextMonth(t time.Time, cs *CronSchedule) time.Time {
	// Move to first of next month.
	t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
	for i := 0; i < 12; i++ {
		if cs.months[t.Month()] {
			return t
		}
		t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
	}
	return t
}

// String returns the original cron expression.
func (cs *CronSchedule) String() string { return cs.expr }
