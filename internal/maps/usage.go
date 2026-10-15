package maps

import (
	"sort"
	"sync"
	"sync/atomic"
)

// Usage counts billable and avoided provider requests. It holds counters only:
// no coordinates, order content, addresses, query text or credentials ever
// enter it, so a snapshot is safe to log or expose to operators.
//
// Counter names are stable. The ones that matter for spend:
//
//	routes.calls.essentials / routes.calls.pro   successful provider responses by billing tier
//	routes.attempts                              provider HTTP attempts, including retries and failures
//	routes.refresh.<reason>                      why a route was (re)calculated
//	routes.reused                                requests answered from the active order's route
//	routes.deduped                               concurrent requests that shared one provider call
//	routes.denied.<kind>                         refreshes refused by a budget or cooldown
//	routes.error.<code>                          provider failures by error code
//	routes.fallback                              an existing route served because a refresh was refused/failed
//	routes.cache.hit/miss/error                  provider-level cache outcomes
type Usage struct {
	counters sync.Map // string -> *atomic.Int64
}

func NewUsage() *Usage { return &Usage{} }

// DefaultUsage is the process-wide instance used by the Google provider and
// the delivery route service.
var DefaultUsage = NewUsage()

func (u *Usage) Inc(name string) { u.Add(name, 1) }

func (u *Usage) Add(name string, n int64) {
	if u == nil || name == "" {
		return
	}
	if v, ok := u.counters.Load(name); ok {
		v.(*atomic.Int64).Add(n)
		return
	}
	v, _ := u.counters.LoadOrStore(name, new(atomic.Int64))
	v.(*atomic.Int64).Add(n)
}

// Snapshot returns the current values keyed by counter name.
func (u *Usage) Snapshot() map[string]int64 {
	out := map[string]int64{}
	if u == nil {
		return out
	}
	u.counters.Range(func(k, v any) bool {
		out[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	return out
}

// Names returns the counter names in sorted order.
func (u *Usage) Names() []string {
	snap := u.Snapshot()
	names := make([]string, 0, len(snap))
	for name := range snap {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RoutesBillingTier names the Routes pricing tier a request is billed under:
// traffic-aware preferences and TWO_WHEELER are treated as Pro, everything
// else (including the default TRAFFIC_UNAWARE DRIVE request) is Essentials.
func RoutesBillingTier(options RouteOptions) string {
	if options.TravelMode == "TWO_WHEELER" {
		return "pro"
	}
	switch options.RoutingPreference {
	case "TRAFFIC_AWARE", "TRAFFIC_AWARE_OPTIMAL":
		return "pro"
	}
	return "essentials"
}
