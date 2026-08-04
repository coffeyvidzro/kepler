package feedback

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

var DefaultMetrics = NewMetrics()

type eventMetricKey struct {
	stage     string
	eventType string
	outcome   string
}

type durationMetric struct {
	count uint64
	sum   float64
}

type QueueSnapshot struct {
	Due          int64
	Scheduled    int64
	DeadLettered int64
	Unlinked     int64
}

type Metrics struct {
	mu               sync.RWMutex
	events           map[eventMetricKey]uint64
	durations        map[string]durationMetric
	queue            map[string]float64
	lastClaimedBatch float64
}

func NewMetrics() *Metrics {
	return &Metrics{
		events:    make(map[eventMetricKey]uint64),
		durations: make(map[string]durationMetric),
		queue: map[string]float64{
			"due":           0,
			"scheduled":     0,
			"dead_lettered": 0,
			"unlinked":      0,
		},
	}
}

func (m *Metrics) RecordEvent(stage, eventType, outcome string) {
	if m == nil {
		return
	}
	key := eventMetricKey{
		stage:     metricLabel(stage),
		eventType: metricLabel(eventType),
		outcome:   metricLabel(outcome),
	}
	m.mu.Lock()
	m.events[key]++
	m.mu.Unlock()
}

func (m *Metrics) ObserveOperation(operation string, elapsed time.Duration) {
	if m == nil {
		return
	}
	operation = metricLabel(operation)
	seconds := elapsed.Seconds()
	if seconds < 0 {
		seconds = 0
	}
	m.mu.Lock()
	value := m.durations[operation]
	value.count++
	value.sum += seconds
	m.durations[operation] = value
	m.mu.Unlock()
}

func (m *Metrics) SetReconciliationQueue(snapshot QueueSnapshot) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.queue["due"] = nonNegativeFloat(snapshot.Due)
	m.queue["scheduled"] = nonNegativeFloat(snapshot.Scheduled)
	m.queue["dead_lettered"] = nonNegativeFloat(snapshot.DeadLettered)
	m.queue["unlinked"] = nonNegativeFloat(snapshot.Unlinked)
	m.mu.Unlock()
}

func (m *Metrics) SetLastClaimedBatch(size int) {
	if m == nil {
		return
	}
	if size < 0 {
		size = 0
	}
	m.mu.Lock()
	m.lastClaimedBatch = float64(size)
	m.mu.Unlock()
}

func (m *Metrics) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	if m == nil {
		http.Error(response, "metrics are not configured", http.StatusServiceUnavailable)
		return
	}

	m.mu.RLock()
	events := make(map[eventMetricKey]uint64, len(m.events))
	for key, value := range m.events {
		events[key] = value
	}
	durations := make(map[string]durationMetric, len(m.durations))
	for key, value := range m.durations {
		durations[key] = value
	}
	queue := make(map[string]float64, len(m.queue))
	for key, value := range m.queue {
		queue[key] = value
	}
	lastClaimedBatch := m.lastClaimedBatch
	m.mu.RUnlock()

	response.Header().Set("Content-Type", metricsContentType)
	response.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprintln(response, "# HELP dugble_email_feedback_events_total Email feedback events observed by lifecycle stage and outcome.")
	_, _ = fmt.Fprintln(response, "# TYPE dugble_email_feedback_events_total counter")
	eventKeys := make([]eventMetricKey, 0, len(events))
	for key := range events {
		eventKeys = append(eventKeys, key)
	}
	sort.Slice(eventKeys, func(left, right int) bool {
		if eventKeys[left].stage != eventKeys[right].stage {
			return eventKeys[left].stage < eventKeys[right].stage
		}
		if eventKeys[left].eventType != eventKeys[right].eventType {
			return eventKeys[left].eventType < eventKeys[right].eventType
		}
		return eventKeys[left].outcome < eventKeys[right].outcome
	})
	for _, key := range eventKeys {
		_, _ = fmt.Fprintf(
			response,
			"dugble_email_feedback_events_total{stage=\"%s\",event_type=\"%s\",outcome=\"%s\"} %d\n",
			escapeMetricLabel(key.stage),
			escapeMetricLabel(key.eventType),
			escapeMetricLabel(key.outcome),
			events[key],
		)
	}

	_, _ = fmt.Fprintln(response, "# HELP dugble_email_feedback_operation_duration_seconds Time spent in email feedback operations.")
	_, _ = fmt.Fprintln(response, "# TYPE dugble_email_feedback_operation_duration_seconds summary")
	operations := make([]string, 0, len(durations))
	for operation := range durations {
		operations = append(operations, operation)
	}
	sort.Strings(operations)
	for _, operation := range operations {
		value := durations[operation]
		label := escapeMetricLabel(operation)
		_, _ = fmt.Fprintf(response, "dugble_email_feedback_operation_duration_seconds_sum{operation=\"%s\"} %s\n", label, formatMetricFloat(value.sum))
		_, _ = fmt.Fprintf(response, "dugble_email_feedback_operation_duration_seconds_count{operation=\"%s\"} %d\n", label, value.count)
	}

	_, _ = fmt.Fprintln(response, "# HELP dugble_email_feedback_reconciliation_queue_events Current durable reconciliation queue state.")
	_, _ = fmt.Fprintln(response, "# TYPE dugble_email_feedback_reconciliation_queue_events gauge")
	queueStates := make([]string, 0, len(queue))
	for state := range queue {
		queueStates = append(queueStates, state)
	}
	sort.Strings(queueStates)
	for _, state := range queueStates {
		_, _ = fmt.Fprintf(response, "dugble_email_feedback_reconciliation_queue_events{state=\"%s\"} %s\n", escapeMetricLabel(state), formatMetricFloat(queue[state]))
	}

	_, _ = fmt.Fprintln(response, "# HELP dugble_email_feedback_reconciliation_last_claimed_batch Number of events claimed by the latest reconciliation batch.")
	_, _ = fmt.Fprintln(response, "# TYPE dugble_email_feedback_reconciliation_last_claimed_batch gauge")
	_, _ = fmt.Fprintf(response, "dugble_email_feedback_reconciliation_last_claimed_batch %s\n", formatMetricFloat(lastClaimedBatch))
}

func metricLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	return value
}

func escapeMetricLabel(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return strings.ReplaceAll(value, "\"", "\\\"")
}

func formatMetricFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func nonNegativeFloat(value int64) float64 {
	if value < 0 {
		return 0
	}
	return float64(value)
}
