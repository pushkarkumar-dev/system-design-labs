package obs

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
)

// WritePrometheusText serialises all MetricFamilies from registry into the
// Prometheus text exposition format (version 0.0.4).
//
// Format reference: https://prometheus.io/docs/instrumenting/exposition_formats/
func WritePrometheusText(w io.Writer, families []MetricFamily) error {
	for _, fam := range families {
		// # HELP line
		if fam.Help != "" {
			if _, err := fmt.Fprintf(w, "# HELP %s %s\n", fam.Name, fam.Help); err != nil {
				return err
			}
		}
		// # TYPE line
		if _, err := fmt.Fprintf(w, "# TYPE %s %s\n", fam.Name, fam.Type); err != nil {
			return err
		}

		switch fam.Type {
		case MetricHistogram:
			if err := writeHistogramSamples(w, fam); err != nil {
				return err
			}
		default:
			for _, s := range fam.Samples {
				if err := writeSample(w, fam.Name, s); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeHistogramSamples writes _bucket, _sum, and _count lines.
func writeHistogramSamples(w io.Writer, fam MetricFamily) error {
	for _, s := range fam.Samples {
		typeTag, hasType := s.Labels["__type__"]
		if hasType {
			// _sum or _count line
			suffix := "_" + typeTag
			lbls := labelStringWithout(s.Labels, "__type__")
			if lbls != "" {
				if _, err := fmt.Fprintf(w, "%s%s{%s} %s\n",
					fam.Name, suffix, lbls, formatValue(s.Value)); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "%s%s %s\n",
					fam.Name, suffix, formatValue(s.Value)); err != nil {
					return err
				}
			}
			continue
		}
		// _bucket line (has "le" label)
		lbls := labelString(s.Labels)
		if _, err := fmt.Fprintf(w, "%s_bucket{%s} %s\n",
			fam.Name, lbls, formatValue(s.Value)); err != nil {
			return err
		}
	}
	return nil
}

func writeSample(w io.Writer, name string, s Sample) error {
	lbls := labelString(s.Labels)
	if lbls != "" {
		_, err := fmt.Fprintf(w, "%s{%s} %s\n", name, lbls, formatValue(s.Value))
		return err
	}
	_, err := fmt.Fprintf(w, "%s %s\n", name, formatValue(s.Value))
	return err
}

// labelString renders a sorted label map as key="value",... Prometheus pairs.
// Empty-string labels are omitted.
func labelString(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if labels[k] != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, k, labels[k]))
	}
	return strings.Join(parts, ",")
}

// labelStringWithout renders labels omitting the given key.
func labelStringWithout(labels map[string]string, omit string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if k != omit && labels[k] != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, k, labels[k]))
	}
	return strings.Join(parts, ",")
}

// formatValue renders a float64 in a format Prometheus parsers accept.
func formatValue(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	default:
		return fmt.Sprintf("%g", v)
	}
}

// MetricsHandler returns an http.HandlerFunc that serves the Prometheus text
// format for all metrics in registry. Mount at /metrics.
func MetricsHandler(registry *MetricsRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		families := registry.Gather()
		if err := WritePrometheusText(w, families); err != nil {
			// Headers already sent — best we can do is log.
			_ = err
		}
	}
}
