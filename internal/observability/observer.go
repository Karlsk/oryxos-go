package observability

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Observer records in-process application observations without an export surface.
type Observer interface {
	ObserveHTTP(ctx context.Context, method, route string, status int, duration time.Duration)
	SetReady(ready bool)
	Snapshot() Snapshot
}

// Snapshot is a copy of the observer's current state.
type Snapshot struct {
	Ready        bool
	HTTPRequests []HTTPRequestSnapshot
}

// HTTPRequestSnapshot is an aggregate for a fixed HTTP request dimension tuple.
type HTTPRequestSnapshot struct {
	Method        string
	Route         string
	Status        int
	Count         uint64
	TotalDuration time.Duration
}

type httpRequestKey struct {
	method string
	route  string
	status int
}

type observer struct {
	mu       sync.RWMutex
	ready    bool
	requests map[httpRequestKey]HTTPRequestSnapshot
}

// NewObserver creates a concurrency-safe in-process observer with readiness false.
func NewObserver() Observer {
	return &observer{requests: make(map[httpRequestKey]HTTPRequestSnapshot)}
}

func (o *observer) ObserveHTTP(_ context.Context, method, route string, status int, duration time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	key := httpRequestKey{method: method, route: route, status: status}
	request := o.requests[key]
	request.Method = method
	request.Route = route
	request.Status = status
	request.Count++
	request.TotalDuration += duration
	o.requests[key] = request
}

func (o *observer) SetReady(ready bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.ready = ready
}

func (o *observer) Snapshot() Snapshot {
	o.mu.RLock()
	defer o.mu.RUnlock()
	snapshot := Snapshot{Ready: o.ready, HTTPRequests: make([]HTTPRequestSnapshot, 0, len(o.requests))}
	for _, request := range o.requests {
		snapshot.HTTPRequests = append(snapshot.HTTPRequests, request)
	}
	sort.Slice(snapshot.HTTPRequests, func(left, right int) bool {
		first, second := snapshot.HTTPRequests[left], snapshot.HTTPRequests[right]
		if first.Method != second.Method {
			return first.Method < second.Method
		}
		if first.Route != second.Route {
			return first.Route < second.Route
		}
		return first.Status < second.Status
	})
	return snapshot
}
