// Package dispatchtrace writes structured, privacy-safe diagnostic events for
// the dispatch path: from ORDER_PLACED to offer persistence to the rider's
// socket.
//
// One line per event, machine-searchable:
//
//	[DISPATCH] event=dispatch.eligibility.evaluated order_id=13294 result=error reason_code=eligibility_query_failed ...
//
// Rules this package enforces:
//   - Event names and reason codes are fixed strings from this file.
//   - Values are sanitised to a conservative character set and truncated, so
//     a value can never forge a second log line.
//   - Callers never pass coordinates, addresses, phone numbers, names,
//     tokens or payloads. Distances are rounded kilometres; ages are seconds.
//
// The service has no metrics stack; these stable event names and reason
// codes are what an operator counts (grep / log queries).
package dispatchtrace

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Event names.
const (
	EventAttemptStarted       = "dispatch.attempt.started"
	EventEligibilityEvaluated = "dispatch.eligibility.evaluated"
	EventNoEligibleRiders     = "dispatch.no_eligible_riders"
	EventTargetRiderDecision  = "dispatch.eligibility.target_rider"
	EventOfferPersisted       = "dispatch.offer.persisted"
	EventOfferPersistFailed   = "dispatch.offer.persist_failed"
	EventOfferPublish         = "websocket.offer.lookup"
	EventConnectionOpened     = "websocket.connection.opened"
	EventConnectionRejected   = "websocket.connection.rejected"
	EventConnectionClosed     = "websocket.connection.closed"
	EventWriteFailed          = "websocket.write_failed"
	EventTraceConfig          = "dispatch.trace.config"
	EventRedisSearch          = "dispatch.search.redis"
	EventOfferAccepted        = "dispatch.offer.accepted"
	EventOfferAcceptRejected  = "dispatch.offer.accept_rejected"
	EventOfferDeclined        = "dispatch.offer.declined"
	EventOfferExpired         = "dispatch.offer.expired"
	EventOfferWithdrawn       = "dispatch.offer.withdrawn"
	EventLocationIndexFailed  = "rider.location.index_failed"
)

// Reason codes. Bounded: add here, never inline.
const (
	ReasonEligibilityQueryFailed = "eligibility_query_failed"
	ReasonNoEligibleRiders       = "no_eligible_riders"
	ReasonOfferPersistFailed     = "offer_persist_failed"
	ReasonOfferNotReopened       = "offer_not_reopened"
	ReasonNoMatchingConnection   = "no_matching_connection"
	ReasonSocketBackpressure     = "socket_backpressure"
	ReasonTokenMissing           = "token_missing"
	ReasonTokenInvalid           = "token_invalid"
	ReasonMissingSubject         = "missing_subject"
	ReasonUpgradeFailed          = "upgrade_failed"
	ReasonSocketWriteFailed      = "socket_write_failed"
	ReasonPeerClosed             = "peer_closed"
	ReasonServerClosed           = "server_closed"
	ReasonRedisUnavailable       = "redis_unavailable"
	ReasonRedisNoCandidates      = "redis_no_candidates"
	ReasonRedisTooFewVerified    = "redis_too_few_verified"
	ReasonOfferExpired           = "offer_expired"
	ReasonAlreadyAssigned        = "already_assigned"
	ReasonNotPending             = "offer_not_pending"
	ReasonNotOwner               = "offer_not_owned"
	ReasonAssignedToOther        = "assigned_to_other_rider"
	ReasonOfferNotFound          = "offer_not_found"
	ReasonAcceptFailed           = "accept_failed"
)

// Fields are the key/value pairs of one event.
type Fields map[string]interface{}

// logf is the sink; tests replace it with SetOutput.
var logf = log.Printf

// SetOutput redirects events (for tests) and returns a restore function.
func SetOutput(f func(format string, args ...interface{})) (restore func()) {
	previous := logf
	logf = f
	return func() { logf = previous }
}

// Emit writes one event line. Keys are sorted so lines are stable.
func Emit(event string, fields Fields) {
	var b strings.Builder
	b.WriteString("[DISPATCH] event=")
	b.WriteString(clean(event))
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteByte(' ')
		b.WriteString(clean(k))
		b.WriteByte('=')
		b.WriteString(formatValue(fields[k]))
	}
	logf("%s", b.String())
}

// ErrorText reduces an error to a short single-line string for a log field.
// Database errors from lib/pq carry the message, not row data.
func ErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

const maxValueLen = 160

func formatValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return "-"
	case string:
		return clean(t)
	case error:
		return clean(t.Error())
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', 2, 64)
	case time.Duration:
		return strconv.FormatFloat(t.Seconds(), 'f', 1, 64) + "s"
	case map[string]int:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, clean(k)+":"+strconv.Itoa(t[k]))
		}
		return strings.Join(parts, ",")
	default:
		return clean(fmt.Sprint(t))
	}
}

// clean keeps letters, digits and a few separators; everything else becomes
// '_'. Newlines, spaces and '=' can therefore never forge another field or
// another log line.
func clean(s string) string {
	if s == "" {
		return "-"
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= maxValueLen {
			b.WriteString("…")
			break
		}
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		case strings.ContainsRune("._-:/@", r):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		n++
	}
	return b.String()
}

// --- target trace ------------------------------------------------------------

// Trace is the opt-in, per-target diagnostic mode. When active, dispatch
// logs a full pass/fail decision vector for one rider against one order
// instead of only aggregate counts.
//
// It is off unless DISPATCH_TRACE_UNTIL is a future RFC 3339 time, and never
// lasts more than MaxTraceWindow from process start. Environment variables:
//
//	DISPATCH_TRACE_UNTIL     required, e.g. 2026-09-12T10:00:00Z
//	DISPATCH_TRACE_ORDER_ID  optional; only this order is traced
//	DISPATCH_TRACE_RIDER_ID  the rider whose decision vector is logged
type Trace struct {
	OrderID int
	RiderID string
	Until   time.Time
}

// MaxTraceWindow caps a trace that was left configured by mistake.
const MaxTraceWindow = 24 * time.Hour

// LoadTrace reads the trace configuration. An absent, invalid or past expiry
// disables tracing; a far-future one is clamped to MaxTraceWindow.
func LoadTrace(getenv func(string) string, now time.Time) Trace {
	raw := strings.TrimSpace(getenv("DISPATCH_TRACE_UNTIL"))
	if raw == "" {
		return Trace{}
	}
	until, err := time.Parse(time.RFC3339, raw)
	if err != nil || !until.After(now) {
		return Trace{}
	}
	if until.Sub(now) > MaxTraceWindow {
		until = now.Add(MaxTraceWindow)
	}
	orderID, _ := strconv.Atoi(strings.TrimSpace(getenv("DISPATCH_TRACE_ORDER_ID")))
	riderID := strings.TrimSpace(getenv("DISPATCH_TRACE_RIDER_ID"))
	if riderID == "" {
		return Trace{}
	}
	return Trace{OrderID: orderID, RiderID: riderID, Until: until}
}

// LoadTraceFromEnv is LoadTrace with the process environment.
func LoadTraceFromEnv() Trace { return LoadTrace(os.Getenv, time.Now()) }

// Covers reports whether the trace applies to this order right now.
func (t Trace) Covers(orderID int, now time.Time) bool {
	if t.RiderID == "" || !now.Before(t.Until) {
		return false
	}
	return t.OrderID == 0 || t.OrderID == orderID
}

// RiderDecision is one rider's result for each dispatch filter, in the order
// FindNearestRiders applies them.
type RiderDecision struct {
	AvailabilityRow bool
	Online          bool
	Available       bool
	Idle            bool
	LocationRow     bool
	LocationFresh   bool
	WithinRadius    bool
	LocationAgeSec  *int64
	DistanceKm      *float64
}

// FirstFailure names the first filter the rider fails, or "" if they pass.
func (d RiderDecision) FirstFailure() string {
	switch {
	case !d.AvailabilityRow:
		return "rider_availability_missing"
	case !d.Online:
		return "rider_not_online"
	case !d.Available:
		return "rider_not_available"
	case !d.Idle:
		return "rider_busy"
	case !d.LocationRow:
		return "rider_location_missing"
	case !d.LocationFresh:
		return "rider_location_stale"
	case !d.WithinRadius:
		return "rider_outside_radius"
	}
	return ""
}

// Fields renders the decision vector. Distance is rounded to 0.1 km; no
// coordinates are included.
func (d RiderDecision) Fields() Fields {
	f := Fields{
		"availability_row": d.AvailabilityRow,
		"online":           d.Online,
		"available":        d.Available,
		"idle":             d.Idle,
		"location_row":     d.LocationRow,
		"location_fresh":   d.LocationFresh,
		"within_radius":    d.WithinRadius,
		"eligible":         d.FirstFailure() == "",
		"first_failure":    d.FirstFailure(),
	}
	if d.LocationAgeSec != nil {
		f["location_age_s"] = *d.LocationAgeSec
	}
	if d.DistanceKm != nil {
		f["distance_km"] = float64(int64(*d.DistanceKm*10+0.5)) / 10
	}
	return f
}
