package verifymetrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const metricsContentType = "text/plain; version=0.0.4; charset=utf-8"

var Default = New()

type key struct {
	operation string
	outcome   string
}

type duration struct {
	count uint64
	sum   float64
}

type Metrics struct {
	mu         sync.RWMutex
	operations map[key]uint64
	durations  map[string]duration
	expired    uint64
}

func New() *Metrics {
	return &Metrics{operations: make(map[key]uint64), durations: make(map[string]duration)}
}

func (metrics *Metrics) Observe(operation, outcome string, elapsed time.Duration) {
	if metrics == nil {
		return
	}
	operation = label(operation)
	outcome = label(outcome)
	seconds := elapsed.Seconds()
	if seconds < 0 {
		seconds = 0
	}
	metrics.mu.Lock()
	metrics.operations[key{operation: operation, outcome: outcome}]++
	value := metrics.durations[operation]
	value.count++
	value.sum += seconds
	metrics.durations[operation] = value
	metrics.mu.Unlock()
}

func (metrics *Metrics) AddExpired(count int) {
	if metrics == nil || count <= 0 {
		return
	}
	metrics.mu.Lock()
	metrics.expired += uint64(count)
	metrics.mu.Unlock()
}

func Outcome(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}

func (metrics *Metrics) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	if metrics == nil {
		http.Error(response, "verify metrics are not configured", http.StatusServiceUnavailable)
		return
	}
	metrics.mu.RLock()
	operations := make(map[key]uint64, len(metrics.operations))
	for metric, value := range metrics.operations {
		operations[metric] = value
	}
	durations := make(map[string]duration, len(metrics.durations))
	for operation, value := range metrics.durations {
		durations[operation] = value
	}
	expired := metrics.expired
	metrics.mu.RUnlock()

	response.Header().Set("Content-Type", metricsContentType)
	response.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(response, "# HELP dugble_verify_operations_total Verify operations by operation and outcome.")
	_, _ = fmt.Fprintln(response, "# TYPE dugble_verify_operations_total counter")
	keys := make([]key, 0, len(operations))
	for metric := range operations {
		keys = append(keys, metric)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].operation != keys[right].operation {
			return keys[left].operation < keys[right].operation
		}
		return keys[left].outcome < keys[right].outcome
	})
	for _, metric := range keys {
		_, _ = fmt.Fprintf(response, "dugble_verify_operations_total{operation=\"%s\",outcome=\"%s\"} %d\n", escape(metric.operation), escape(metric.outcome), operations[metric])
	}

	_, _ = fmt.Fprintln(response, "# HELP dugble_verify_operation_duration_seconds Time spent in Verify operations.")
	_, _ = fmt.Fprintln(response, "# TYPE dugble_verify_operation_duration_seconds summary")
	names := make([]string, 0, len(durations))
	for operation := range durations {
		names = append(names, operation)
	}
	sort.Strings(names)
	for _, operation := range names {
		value := durations[operation]
		_, _ = fmt.Fprintf(response, "dugble_verify_operation_duration_seconds_sum{operation=\"%s\"} %s\n", escape(operation), strconv.FormatFloat(value.sum, 'g', -1, 64))
		_, _ = fmt.Fprintf(response, "dugble_verify_operation_duration_seconds_count{operation=\"%s\"} %d\n", escape(operation), value.count)
	}

	_, _ = fmt.Fprintln(response, "# HELP dugble_verify_expired_total Verifications expired by the expiry worker.")
	_, _ = fmt.Fprintln(response, "# TYPE dugble_verify_expired_total counter")
	_, _ = fmt.Fprintf(response, "dugble_verify_expired_total %d\n", expired)
}

func label(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	return value
}

func escape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return strings.ReplaceAll(value, "\"", "\\\"")
}
