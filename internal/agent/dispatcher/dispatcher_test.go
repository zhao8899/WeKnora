package dispatcher

import (
	"context"
	"testing"
)

func buildDispatcher() *Dispatcher {
	d := New(nil)
	d.Register(&Route{
		Name:        "kb_search",
		Description: "search the knowledge base for documents and chunks",
		Keywords:    []string{"knowledge base", "document", "文档"},
		Handler:     "rag_pipeline",
	})
	d.Register(&Route{
		Name:        "web_research",
		Description: "search the open web for current information",
		Keywords:    []string{"web", "google", "search online", "news"},
		Handler:     "web_tool",
	})
	d.Register(&Route{
		Name:        "sql_analyst",
		Description: "query the database and compute analytics",
		Keywords:    []string{"sql", "database", "report"},
		Handler:     "sql_tool",
	})
	return d
}

func TestDispatch_RoutesByKeyword(t *testing.T) {
	d := buildDispatcher()
	cases := []struct {
		query string
		want  string
	}{
		{"search the knowledge base for auth docs", "kb_search"},
		{"give me today's news about AI", "web_research"},
		{"run a sql report on last month's signups", "sql_analyst"},
		{"查一下这份文档", "kb_search"},
	}
	for _, c := range cases {
		dec, err := d.Dispatch(context.Background(), c.query)
		if err != nil {
			t.Fatalf("query=%q err=%v", c.query, err)
		}
		if dec.Route.Name != c.want {
			t.Errorf("query=%q → got %s want %s (score=%.2f)",
				c.query, dec.Route.Name, c.want, dec.Score)
		}
	}
}

func TestDispatch_FallbackWhenNoSignal(t *testing.T) {
	d := buildDispatcher()
	_ = d.SetDefault("kb_search")
	dec, err := d.Dispatch(context.Background(), "hi")
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !dec.Fallback {
		t.Errorf("expected fallback, got %+v", dec)
	}
	if dec.Route.Name != "kb_search" {
		t.Errorf("fallback route = %s want kb_search", dec.Route.Name)
	}
}

func TestDispatch_DescriptionOverlapBoosts(t *testing.T) {
	d := buildDispatcher()
	// "analytics" appears in sql_analyst's description but not its keywords
	dec, _ := d.Dispatch(context.Background(), "I want analytics computed")
	if dec.Route.Name != "sql_analyst" {
		t.Errorf("expected description overlap to route to sql_analyst, got %s (score=%.2f)",
			dec.Route.Name, dec.Score)
	}
}

func TestDispatch_NoRoutesError(t *testing.T) {
	d := New(nil)
	if _, err := d.Dispatch(context.Background(), "anything"); err == nil {
		t.Error("expected error for empty dispatcher")
	}
}

func TestRegister_PanicsOnDuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate Name")
		}
	}()
	d := New(nil)
	d.Register(&Route{Name: "x"})
	d.Register(&Route{Name: "x"})
}

func TestSetDefault_UnknownRouteErrors(t *testing.T) {
	d := New(nil)
	d.Register(&Route{Name: "a"})
	if err := d.SetDefault("b"); err == nil {
		t.Error("expected error for unknown default")
	}
}

// Custom classifier — used to demonstrate pluggability.
type constClassifier struct{ scores []float64 }

func (c constClassifier) Score(_ context.Context, _ string, routes []*Route) ([]float64, error) {
	return c.scores, nil
}

func TestDispatch_CustomClassifier(t *testing.T) {
	d := New(constClassifier{scores: []float64{0.1, 5.0}})
	d.Register(&Route{Name: "a"})
	d.Register(&Route{Name: "b"})
	dec, err := d.Dispatch(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Route.Name != "b" || dec.Score != 5.0 {
		t.Errorf("got %+v", dec)
	}
}
