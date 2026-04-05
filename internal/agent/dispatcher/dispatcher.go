// Package dispatcher routes an incoming user query to a specialised
// sub-agent. It is the entry half of the orchestrator → sub-agent pattern:
// one coordinator receives every request, classifies it, and delegates to
// the sub-agent best equipped to answer.
//
// The dispatcher lives above the existing ReAct engine and does not know
// how sub-agents are implemented. It consumes a list of Route descriptors
// (name + description + keywords) and returns a Decision identifying which
// route the orchestrator should invoke. The actual invocation is the
// caller's responsibility — dispatcher is classification only.
//
// Classification strategy is a deterministic keyword + description scorer.
// It is cheap (no LLM call), explainable (score breakdown is in Decision),
// and good enough to route ~80% of traffic correctly. Higher-accuracy
// strategies (embedding similarity, LLM classifier) can be added by
// wrapping a custom Classifier without touching the public API.
package dispatcher

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Route describes one sub-agent the dispatcher can route to.
//
// Name is the stable identifier the orchestrator uses to look up the
// sub-agent (e.g. "kb_search", "web_research", "sql_analyst").
// Description is the natural-language purpose — used both for scoring and
// for building the system prompt when an LLM classifier is layered on top.
// Keywords are exact-match triggers; they score higher than description
// overlap so operators can guarantee routing for known phrases.
// Handler is an opaque identifier the orchestrator resolves to an actual
// runtime (function, engine, external service). Dispatcher never touches
// Handler — treat it as passthrough metadata.
type Route struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords,omitempty"`
	Handler     string   `json:"handler,omitempty"`
}

// Decision is the classifier's output. Route is nil only when no routes
// are registered; otherwise it always carries the fallback/default.
type Decision struct {
	Route  *Route  `json:"route"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
	// Fallback is true when the scorer found no signal and the dispatcher
	// returned the default route. Callers should surface this to traces so
	// misroutes are debuggable.
	Fallback bool `json:"fallback"`
}

// Classifier is the pluggable scoring interface. Implementations return a
// score per route for the given query; higher wins. A zero-length result
// or all-zero scores forces Dispatcher to fall back to its default.
type Classifier interface {
	Score(ctx context.Context, query string, routes []*Route) ([]float64, error)
}

// Dispatcher routes queries to registered sub-agents. Safe for concurrent
// use; routes are registered once at startup and read-only thereafter.
type Dispatcher struct {
	mu         sync.RWMutex
	routes     []*Route
	defaultKey string
	classifier Classifier
}

// New builds a Dispatcher with the given classifier. Passing nil uses the
// built-in keyword classifier, which is the expected default.
func New(classifier Classifier) *Dispatcher {
	if classifier == nil {
		classifier = &KeywordClassifier{}
	}
	return &Dispatcher{classifier: classifier}
}

// Register adds a route. Panics on duplicate Name — duplicates are almost
// always a wiring bug and should fail loudly at startup rather than
// silently mask earlier registrations.
func (d *Dispatcher) Register(r *Route) {
	if r == nil || r.Name == "" {
		panic("dispatcher: route must have a Name")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, existing := range d.routes {
		if existing.Name == r.Name {
			panic(fmt.Sprintf("dispatcher: duplicate route %q", r.Name))
		}
	}
	d.routes = append(d.routes, r)
}

// SetDefault marks a registered route name as the fallback. When the
// classifier finds no signal, Dispatch returns this route. If SetDefault
// is never called, the first registered route is the implicit default.
func (d *Dispatcher) SetDefault(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, r := range d.routes {
		if r.Name == name {
			d.defaultKey = name
			return nil
		}
	}
	return fmt.Errorf("dispatcher: default route %q not registered", name)
}

// Routes returns a snapshot of registered routes. Used by traces / UIs.
func (d *Dispatcher) Routes() []*Route {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]*Route, len(d.routes))
	copy(out, d.routes)
	return out
}

// Dispatch classifies the query and returns the chosen route. Returns an
// error only when no routes are registered at all; a query with no signal
// yields a Decision with Fallback=true rather than an error.
func (d *Dispatcher) Dispatch(ctx context.Context, query string) (*Decision, error) {
	d.mu.RLock()
	routes := make([]*Route, len(d.routes))
	copy(routes, d.routes)
	defaultKey := d.defaultKey
	d.mu.RUnlock()

	if len(routes) == 0 {
		return nil, fmt.Errorf("dispatcher: no routes registered")
	}

	scores, err := d.classifier.Score(ctx, query, routes)
	if err != nil {
		return nil, fmt.Errorf("classifier: %w", err)
	}
	if len(scores) != len(routes) {
		return nil, fmt.Errorf("dispatcher: classifier returned %d scores for %d routes",
			len(scores), len(routes))
	}

	bestIdx := -1
	bestScore := 0.0
	for i, s := range scores {
		if s > bestScore {
			bestScore = s
			bestIdx = i
		}
	}

	if bestIdx == -1 {
		fallback := routes[0]
		if defaultKey != "" {
			for _, r := range routes {
				if r.Name == defaultKey {
					fallback = r
					break
				}
			}
		}
		return &Decision{
			Route:    fallback,
			Score:    0,
			Reason:   "no route matched; fell back to default",
			Fallback: true,
		}, nil
	}

	return &Decision{
		Route:  routes[bestIdx],
		Score:  bestScore,
		Reason: fmt.Sprintf("%s scored highest (%.2f)", routes[bestIdx].Name, bestScore),
	}, nil
}

// -------------------------------------------------------------------------
// KeywordClassifier — default scorer.
// -------------------------------------------------------------------------

// KeywordClassifier scores by counting keyword and description-term hits
// in the query. Deterministic, fast, zero-dep.
type KeywordClassifier struct{}

// Score implements Classifier.
func (KeywordClassifier) Score(_ context.Context, query string, routes []*Route) ([]float64, error) {
	q := strings.ToLower(query)
	qTokens := tokenise(q)
	qSet := make(map[string]bool, len(qTokens))
	for _, t := range qTokens {
		qSet[t] = true
	}

	out := make([]float64, len(routes))
	for i, r := range routes {
		if r == nil {
			continue
		}
		score := 0.0
		// Explicit keywords — high weight, substring match against the
		// whole lowercase query so multi-word phrases work.
		for _, kw := range r.Keywords {
			kwLower := strings.ToLower(strings.TrimSpace(kw))
			if kwLower == "" {
				continue
			}
			if strings.Contains(q, kwLower) {
				score += 3.0
			}
		}
		// Description terms — softer signal.
		for _, t := range tokenise(strings.ToLower(r.Description)) {
			if qSet[t] {
				score += 0.5
			}
		}
		out[i] = score
	}
	return out, nil
}

// tokenise splits text into lowercase tokens and filters short English
// stopwords. Kept here (not shared with longterm) so dispatcher has no
// cross-package coupling.
func tokenise(s string) []string {
	out := make([]string, 0, 4)
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		word := cur.String()
		cur.Reset()
		if len(word) < 3 && !hasCJK(word) {
			return
		}
		out = append(out, word)
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z':
			cur.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff:
			cur.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return out
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// SortByName returns routes sorted by Name. Useful for stable UI listings.
func SortByName(routes []*Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		return routes[i].Name < routes[j].Name
	})
}
