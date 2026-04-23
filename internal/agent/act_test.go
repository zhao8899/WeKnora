package agent

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
)

type concurrencyTracker struct {
	current int32
	max     int32
}

func (t *concurrencyTracker) enter() {
	current := atomic.AddInt32(&t.current, 1)
	for {
		max := atomic.LoadInt32(&t.max)
		if current <= max || atomic.CompareAndSwapInt32(&t.max, max, current) {
			break
		}
	}
}

func (t *concurrencyTracker) leave() {
	atomic.AddInt32(&t.current, -1)
}

type mockTool struct {
	name    string
	delay   time.Duration
	tracker *concurrencyTracker
	onStart func()
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return t.name }
func (t *mockTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t *mockTool) Execute(_ context.Context, _ json.RawMessage) (*types.ToolResult, error) {
	if t.onStart != nil {
		t.onStart()
	}
	if t.tracker != nil {
		t.tracker.enter()
		defer t.tracker.leave()
	}
	if t.delay > 0 {
		time.Sleep(t.delay)
	}
	return &types.ToolResult{Success: true, Output: t.name}, nil
}

func newActTestEngine(cfg *types.AgentConfig, tools ...types.Tool) *AgentEngine {
	registry := agenttools.NewToolRegistry()
	for _, tool := range tools {
		registry.RegisterTool(tool)
	}
	return &AgentEngine{
		config:       cfg,
		toolRegistry: registry,
		eventBus:     event.NewEventBus(),
	}
}

func TestExecuteToolCallsParallel_UsesConfiguredLimitAndPreservesOrder(t *testing.T) {
	tracker := &concurrencyTracker{}
	engine := newActTestEngine(
		&types.AgentConfig{
			ParallelToolCalls:    true,
			MaxParallelToolCalls: 2,
		},
		&mockTool{name: agenttools.ToolKnowledgeSearch, delay: 80 * time.Millisecond, tracker: tracker},
		&mockTool{name: agenttools.ToolWebSearch, delay: 80 * time.Millisecond, tracker: tracker},
		&mockTool{name: agenttools.ToolGetDocumentInfo, delay: 80 * time.Millisecond, tracker: tracker},
	)

	response := &types.ChatResponse{
		ToolCalls: []types.LLMToolCall{
			{ID: "call-1", Function: types.FunctionCall{Name: agenttools.ToolKnowledgeSearch, Arguments: `{}`}},
			{ID: "call-2", Function: types.FunctionCall{Name: agenttools.ToolWebSearch, Arguments: `{}`}},
			{ID: "call-3", Function: types.FunctionCall{Name: agenttools.ToolGetDocumentInfo, Arguments: `{}`}},
		},
	}
	step := &types.AgentStep{}

	engine.executeToolCalls(context.Background(), response, step, 0, "sess-1")

	if len(step.ToolCalls) != 3 {
		t.Fatalf("expected 3 tool calls, got %d", len(step.ToolCalls))
	}
	if step.ToolCalls[0].Name != agenttools.ToolKnowledgeSearch ||
		step.ToolCalls[1].Name != agenttools.ToolWebSearch ||
		step.ToolCalls[2].Name != agenttools.ToolGetDocumentInfo {
		t.Fatalf("expected original tool call order to be preserved, got %+v", step.ToolCalls)
	}
	if got := atomic.LoadInt32(&tracker.max); got != 2 {
		t.Fatalf("expected parallelism limit to be enforced at 2, got %d", got)
	}
}

func TestExecuteToolCallsParallel_KeepsUnsafeToolsSequential(t *testing.T) {
	tracker := &concurrencyTracker{}
	var (
		mu         sync.Mutex
		startOrder []string
	)
	recordStart := func(name string) func() {
		return func() {
			mu.Lock()
			startOrder = append(startOrder, name)
			mu.Unlock()
		}
	}

	engine := newActTestEngine(
		&types.AgentConfig{
			ParallelToolCalls:    true,
			MaxParallelToolCalls: 4,
		},
		&mockTool{name: agenttools.ToolKnowledgeSearch, delay: 20 * time.Millisecond, tracker: tracker, onStart: recordStart(agenttools.ToolKnowledgeSearch)},
		&mockTool{name: agenttools.ToolDatabaseQuery, delay: 20 * time.Millisecond, tracker: tracker, onStart: recordStart(agenttools.ToolDatabaseQuery)},
		&mockTool{name: agenttools.ToolWebSearch, delay: 20 * time.Millisecond, tracker: tracker, onStart: recordStart(agenttools.ToolWebSearch)},
	)

	response := &types.ChatResponse{
		ToolCalls: []types.LLMToolCall{
			{ID: "call-1", Function: types.FunctionCall{Name: agenttools.ToolKnowledgeSearch, Arguments: `{}`}},
			{ID: "call-2", Function: types.FunctionCall{Name: agenttools.ToolDatabaseQuery, Arguments: `{}`}},
			{ID: "call-3", Function: types.FunctionCall{Name: agenttools.ToolWebSearch, Arguments: `{}`}},
		},
	}
	step := &types.AgentStep{}

	engine.executeToolCalls(context.Background(), response, step, 0, "sess-1")

	if len(step.ToolCalls) != 3 {
		t.Fatalf("expected 3 tool calls, got %d", len(step.ToolCalls))
	}
	if atomic.LoadInt32(&tracker.max) != 1 {
		t.Fatalf("expected unsafe tool barrier to keep execution sequential, got max concurrency=%d", tracker.max)
	}
	wantOrder := []string{
		agenttools.ToolKnowledgeSearch,
		agenttools.ToolDatabaseQuery,
		agenttools.ToolWebSearch,
	}
	for i, want := range wantOrder {
		if startOrder[i] != want {
			t.Fatalf("expected start order %v, got %v", wantOrder, startOrder)
		}
	}
}
