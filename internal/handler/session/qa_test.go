package session

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type stubMessageService struct {
	mu                   sync.Mutex
	nextID               int
	updatedMessages      []*types.Message
	indexCalls           chan indexCall
	executionMetaPatches []executionMetaPatch
}

type indexCall struct {
	userQuery string
	answer    string
	messageID string
	sessionID string
}

type executionMetaPatch struct {
	sessionID string
	messageID string
	meta      types.JSON
}

func newStubMessageService() *stubMessageService {
	return &stubMessageService{
		indexCalls: make(chan indexCall, 1),
	}
}

func (s *stubMessageService) CreateMessage(ctx context.Context, message *types.Message) (*types.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if message.ID == "" {
		s.nextID++
		message.ID = fmt.Sprintf("msg-%d", s.nextID)
	}
	return message, nil
}

func (s *stubMessageService) GetMessage(ctx context.Context, sessionID string, id string) (*types.Message, error) {
	return nil, nil
}

func (s *stubMessageService) GetMessagesBySession(
	ctx context.Context, sessionID string, page int, pageSize int,
) ([]*types.Message, error) {
	return nil, nil
}

func (s *stubMessageService) GetRecentMessagesBySession(
	ctx context.Context, sessionID string, limit int,
) ([]*types.Message, error) {
	return nil, nil
}

func (s *stubMessageService) GetMessagesBySessionBeforeTime(
	ctx context.Context, sessionID string, beforeTime time.Time, limit int,
) ([]*types.Message, error) {
	return nil, nil
}

func (s *stubMessageService) UpdateMessage(ctx context.Context, message *types.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := *message
	s.updatedMessages = append(s.updatedMessages, &cloned)
	return nil
}

func (s *stubMessageService) UpdateMessageImages(
	ctx context.Context, sessionID, messageID string, images types.MessageImages,
) error {
	return nil
}

func (s *stubMessageService) UpdateMessageRenderedContent(
	ctx context.Context, sessionID, messageID string, renderedContent string,
) error {
	return nil
}

func (s *stubMessageService) UpdateMessageExecutionMeta(
	ctx context.Context, sessionID, messageID string, executionMeta types.JSON,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executionMetaPatches = append(s.executionMetaPatches, executionMetaPatch{
		sessionID: sessionID,
		messageID: messageID,
		meta:      executionMeta,
	})
	return nil
}

func (s *stubMessageService) DeleteMessage(ctx context.Context, sessionID string, id string) error {
	return nil
}

func (s *stubMessageService) ClearSessionMessages(ctx context.Context, sessionID string) error {
	return nil
}

func (s *stubMessageService) SearchMessages(
	ctx context.Context, params *types.MessageSearchParams,
) (*types.MessageSearchResult, error) {
	return nil, nil
}

func (s *stubMessageService) IndexMessageToKB(
	ctx context.Context, userQuery string, assistantAnswer string, messageID string, sessionID string,
) {
	s.indexCalls <- indexCall{
		userQuery: userQuery,
		answer:    assistantAnswer,
		messageID: messageID,
		sessionID: sessionID,
	}
}

func (s *stubMessageService) DeleteMessageKnowledge(ctx context.Context, knowledgeID string) {}

func (s *stubMessageService) DeleteSessionKnowledge(ctx context.Context, sessionID string) {}

func (s *stubMessageService) GetChatHistoryKBStats(ctx context.Context) (*types.ChatHistoryKBStats, error) {
	return nil, nil
}

func (s *stubMessageService) UpdateMessageFeedback(ctx context.Context, sessionID, messageID, feedback string) error {
	return nil
}

type stubSessionService struct {
	agentQAFn     func(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error
	knowledgeQAFn func(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error
}

func (s *stubSessionService) CreateSession(ctx context.Context, session *types.Session) (*types.Session, error) {
	return session, nil
}

func (s *stubSessionService) GetSession(ctx context.Context, id string) (*types.Session, error) {
	return nil, nil
}

func (s *stubSessionService) GetSessionsByTenant(ctx context.Context) ([]*types.Session, error) {
	return nil, nil
}

func (s *stubSessionService) GetPagedSessionsByTenant(ctx context.Context, page *types.Pagination) (*types.PageResult, error) {
	return nil, nil
}

func (s *stubSessionService) UpdateSession(ctx context.Context, session *types.Session) error {
	return nil
}

func (s *stubSessionService) DeleteSession(ctx context.Context, id string) error {
	return nil
}

func (s *stubSessionService) BatchDeleteSessions(ctx context.Context, ids []string) error {
	return nil
}

func (s *stubSessionService) DeleteAllSessions(ctx context.Context) error {
	return nil
}

func (s *stubSessionService) GenerateTitle(
	ctx context.Context, session *types.Session, messages []types.Message, modelID string,
) (string, error) {
	return "", nil
}

func (s *stubSessionService) GenerateTitleAsync(
	ctx context.Context, session *types.Session, userQuery string, modelID string, eventBus *event.EventBus,
) {
}

func (s *stubSessionService) KnowledgeQA(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error {
	if s.knowledgeQAFn != nil {
		return s.knowledgeQAFn(ctx, req, eventBus)
	}
	return nil
}

func (s *stubSessionService) KnowledgeQAByEvent(ctx context.Context, chatManage *types.ChatManage, eventList []types.EventType) error {
	return nil
}

func (s *stubSessionService) SearchKnowledge(
	ctx context.Context, knowledgeBaseIDs []string, knowledgeIDs []string, query string,
) ([]*types.SearchResult, error) {
	return nil, nil
}

func (s *stubSessionService) AgentQA(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error {
	if s.agentQAFn != nil {
		return s.agentQAFn(ctx, req, eventBus)
	}
	return nil
}

func (s *stubSessionService) ClearContext(ctx context.Context, sessionID string) error {
	return nil
}

type stubStreamManager struct {
	mu     sync.Mutex
	events []interfaces.StreamEvent
}

func (s *stubStreamManager) AppendEvent(
	ctx context.Context, sessionID, messageID string, event interfaces.StreamEvent,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *stubStreamManager) GetEvents(
	ctx context.Context, sessionID, messageID string, fromOffset int,
) ([]interfaces.StreamEvent, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fromOffset >= len(s.events) {
		return nil, len(s.events), nil
	}
	events := append([]interfaces.StreamEvent(nil), s.events[fromOffset:]...)
	return events, len(s.events), nil
}

func TestCompleteAssistantMessage_WritesCompletedAtAndIndexesHistory(t *testing.T) {
	messageService := newStubMessageService()
	h := &Handler{messageService: messageService}
	assistantMessage := &types.Message{
		ID:        "msg-1",
		SessionID: "session-1",
		Content:   "final answer",
		ExecutionMeta: mustMarshalJSON(map[string]interface{}{
			"requested_mode": "knowledge",
		}),
	}

	h.completeAssistantMessage(context.Background(), assistantMessage, "user question")

	if !assistantMessage.IsCompleted {
		t.Fatalf("expected assistant message to be completed")
	}
	if assistantMessage.UpdatedAt.IsZero() {
		t.Fatalf("expected UpdatedAt to be set")
	}
	if len(messageService.updatedMessages) != 1 {
		t.Fatalf("expected one updated message, got %d", len(messageService.updatedMessages))
	}

	meta := mustExecutionMetaMap(t, assistantMessage.ExecutionMeta)
	if meta["completed_at"] == "" {
		t.Fatalf("expected completed_at in execution_meta, got %v", meta)
	}
	if _, err := time.Parse(time.RFC3339, meta["completed_at"].(string)); err != nil {
		t.Fatalf("expected completed_at to be RFC3339, got %q: %v", meta["completed_at"], err)
	}
	if _, exists := meta["stop_reason"]; exists {
		t.Fatalf("did not expect stop_reason for normal completion, got %v", meta)
	}

	select {
	case call := <-messageService.indexCalls:
		if call.userQuery != "user question" || call.answer != "final answer" {
			t.Fatalf("unexpected index call payload: %#v", call)
		}
		if call.messageID != "msg-1" || call.sessionID != "session-1" {
			t.Fatalf("unexpected index call identifiers: %#v", call)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected chat history indexing to be scheduled")
	}
}

func TestCompleteAssistantMessage_StopMarksUserRequestedAndSkipsIndexing(t *testing.T) {
	messageService := newStubMessageService()
	h := &Handler{messageService: messageService}
	assistantMessage := &types.Message{
		ID:        "msg-2",
		SessionID: "session-2",
		Content:   "partial answer",
		ExecutionMeta: mustMarshalJSON(map[string]interface{}{
			"requested_mode": "agent",
		}),
	}

	h.completeAssistantMessage(context.Background(), assistantMessage, "")

	if assistantMessage.Content != "User stopped this response." {
		t.Fatalf("expected stop content to be overwritten, got %q", assistantMessage.Content)
	}
	if !assistantMessage.IsCompleted {
		t.Fatalf("expected assistant message to be completed")
	}
	if len(messageService.updatedMessages) != 1 {
		t.Fatalf("expected one updated message, got %d", len(messageService.updatedMessages))
	}

	meta := mustExecutionMetaMap(t, assistantMessage.ExecutionMeta)
	if meta["completed_at"] == "" {
		t.Fatalf("expected completed_at in execution_meta, got %v", meta)
	}
	if _, err := time.Parse(time.RFC3339, meta["completed_at"].(string)); err != nil {
		t.Fatalf("expected completed_at to be RFC3339, got %q: %v", meta["completed_at"], err)
	}
	if meta["stop_reason"] != "user_requested" {
		t.Fatalf("expected stop_reason=user_requested, got %v", meta["stop_reason"])
	}

	select {
	case call := <-messageService.indexCalls:
		t.Fatalf("did not expect indexing for stop flow, got %#v", call)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestSetupStopEventHandler_CompletesMessageAndCancels(t *testing.T) {
	messageService := newStubMessageService()
	h := &Handler{messageService: messageService}
	bus := event.NewEventBus()
	assistantMessage := &types.Message{
		ID:        "msg-3",
		SessionID: "session-3",
		Content:   "streaming answer",
		ExecutionMeta: mustMarshalJSON(map[string]interface{}{
			"requested_mode": "agent",
		}),
	}
	cancelled := make(chan struct{}, 1)

	h.setupStopEventHandler(bus, "session-3", 42, assistantMessage, func() {
		cancelled <- struct{}{}
	})

	if err := bus.Emit(context.Background(), event.Event{
		Type:      event.EventStop,
		SessionID: "session-3",
		Data: event.StopData{
			SessionID: "session-3",
			MessageID: "msg-3",
			Reason:    "user_requested",
		},
	}); err != nil {
		t.Fatalf("emit stop event: %v", err)
	}

	select {
	case <-cancelled:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected cancel to be invoked")
	}

	if !assistantMessage.IsCompleted {
		t.Fatalf("expected assistant message to be completed")
	}
	if assistantMessage.Content != "User stopped this response." {
		t.Fatalf("expected final content to be overwritten, got %q", assistantMessage.Content)
	}
	if len(messageService.updatedMessages) != 1 {
		t.Fatalf("expected one updated message, got %d", len(messageService.updatedMessages))
	}

	meta := mustExecutionMetaMap(t, assistantMessage.ExecutionMeta)
	if meta["stop_reason"] != "user_requested" {
		t.Fatalf("expected stop_reason=user_requested, got %v", meta["stop_reason"])
	}
	if meta["completed_at"] == "" {
		t.Fatalf("expected completed_at in execution_meta, got %v", meta)
	}

	select {
	case call := <-messageService.indexCalls:
		t.Fatalf("did not expect indexing for stop flow, got %#v", call)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestBuildExecutionMeta_UsesRequestContextFields(t *testing.T) {
	reqCtx := &qaRequestContext{
		knowledgeBaseIDs:  []string{"kb-1", "kb-2"},
		knowledgeIDs:      []string{"doc-1"},
		webSearchEnabled:  true,
		enableMemory:      true,
		channel:           "web",
		images:            []ImageAttachment{{Data: "data:image/png;base64,abc"}},
		effectiveTenantID: 88,
		customAgent:       &types.CustomAgent{ID: "agent-1"},
	}

	meta := mustExecutionMetaMap(t, buildExecutionMeta(reqCtx, qaModeAgent))

	if meta["requested_mode"] != "agent" || meta["final_mode"] != "agent" {
		t.Fatalf("expected agent modes in execution_meta, got %v", meta)
	}
	if meta["agent_id"] != "agent-1" {
		t.Fatalf("expected agent_id=agent-1, got %v", meta["agent_id"])
	}
	if meta["web_search_enabled"] != true || meta["memory_enabled"] != true {
		t.Fatalf("expected web_search_enabled and memory_enabled to be true, got %v", meta)
	}
	if meta["channel"] != "web" {
		t.Fatalf("expected channel=web, got %v", meta["channel"])
	}
	if meta["has_images"] != true {
		t.Fatalf("expected has_images=true, got %v", meta["has_images"])
	}
	if meta["effective_tenant_id"] != float64(88) {
		t.Fatalf("expected effective_tenant_id=88, got %v", meta["effective_tenant_id"])
	}
	if meta["completed_at"] != "" || meta["stop_reason"] != "" || meta["error_stage"] != "" {
		t.Fatalf("expected empty final-state fields in initial execution_meta, got %v", meta)
	}
	if meta["requested_at"] == "" {
		t.Fatalf("expected requested_at in execution_meta, got %v", meta)
	}
	if _, err := time.Parse(time.RFC3339, meta["requested_at"].(string)); err != nil {
		t.Fatalf("expected requested_at to be RFC3339, got %q: %v", meta["requested_at"], err)
	}

	kbIDs := mustStringSlice(t, meta["kb_ids"])
	if len(kbIDs) != 2 || kbIDs[0] != "kb-1" || kbIDs[1] != "kb-2" {
		t.Fatalf("unexpected kb_ids: %v", kbIDs)
	}
	knowledgeIDs := mustStringSlice(t, meta["knowledge_ids"])
	if len(knowledgeIDs) != 1 || knowledgeIDs[0] != "doc-1" {
		t.Fatalf("unexpected knowledge_ids: %v", knowledgeIDs)
	}
}

func TestUpdateExecutionMetaJSON_PatchesAndRecoversFromInvalidJSON(t *testing.T) {
	current := mustMarshalJSON(map[string]interface{}{
		"requested_mode": "knowledge",
		"completed_at":   "",
	})
	updated := mustExecutionMetaMap(t, updateExecutionMetaJSON(current, map[string]interface{}{
		"completed_at": "2026-04-22T12:00:00Z",
		"error_stage":  "knowledge_qa_execution",
	}))

	if updated["requested_mode"] != "knowledge" {
		t.Fatalf("expected requested_mode to be preserved, got %v", updated["requested_mode"])
	}
	if updated["completed_at"] != "2026-04-22T12:00:00Z" {
		t.Fatalf("expected patched completed_at, got %v", updated["completed_at"])
	}
	if updated["error_stage"] != "knowledge_qa_execution" {
		t.Fatalf("expected patched error_stage, got %v", updated["error_stage"])
	}

	recovered := mustExecutionMetaMap(t, updateExecutionMetaJSON(types.JSON(`{invalid`), map[string]interface{}{
		"stop_reason": "user_requested",
	}))
	if recovered["stop_reason"] != "user_requested" {
		t.Fatalf("expected recovered map to include stop_reason, got %v", recovered)
	}
	if len(recovered) != 1 {
		t.Fatalf("expected invalid JSON recovery to start from empty map, got %v", recovered)
	}
}

func TestExecuteQA_AgentMode_PersistsInitialAndErrorExecutionMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)

	messageService := newStubMessageService()
	streamManager := &stubStreamManager{}
	sessionService := &stubSessionService{
		agentQAFn: func(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error {
			_ = eventBus.Emit(ctx, event.Event{
				Type:      event.EventAgentComplete,
				SessionID: req.Session.ID,
				Data: event.AgentCompleteData{
					MessageID:       req.AssistantMessageID,
					FinalAnswer:     "",
					TotalSteps:      1,
					TotalDurationMs: 1,
				},
			})
			return fmt.Errorf("agent failed")
		},
	}
	h := &Handler{
		messageService: messageService,
		sessionService: sessionService,
		streamManager:  streamManager,
	}

	done := make(chan struct{})
	reqCtx := newTestQARequestContext(t, "session-exec-error", "how are you", &types.CustomAgent{ID: "agent-x"})

	go func() {
		defer close(done)
		h.executeQA(reqCtx.reqCtx, qaModeAgent, false)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("executeQA did not finish")
	}

	if len(messageService.executionMetaPatches) < 2 {
		t.Fatalf("expected at least two execution_meta writes, got %d", len(messageService.executionMetaPatches))
	}

	initial := mustExecutionMetaMap(t, messageService.executionMetaPatches[0].meta)
	if initial["requested_mode"] != "agent" || initial["agent_id"] != "agent-x" {
		t.Fatalf("unexpected initial execution_meta: %v", initial)
	}

	last := mustExecutionMetaMap(t, messageService.executionMetaPatches[len(messageService.executionMetaPatches)-1].meta)
	if last["error_stage"] != "agent_execution" {
		t.Fatalf("expected error_stage=agent_execution, got %v", last["error_stage"])
	}

	if len(messageService.updatedMessages) == 0 {
		t.Fatalf("expected assistant message to be updated on completion")
	}
}

func TestExecuteQA_AgentMode_PersistsPanicExecutionMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)

	messageService := newStubMessageService()
	streamManager := &stubStreamManager{}
	sessionService := &stubSessionService{
		agentQAFn: func(ctx context.Context, req *types.QARequest, eventBus *event.EventBus) error {
			panic("kaboom")
		},
	}
	h := &Handler{
		messageService: messageService,
		sessionService: sessionService,
		streamManager:  streamManager,
	}

	done := make(chan struct{})
	reqCtx := newTestQARequestContext(t, "session-exec-panic", "trigger panic", &types.CustomAgent{ID: "agent-p"})

	go func() {
		defer close(done)
		h.executeQA(reqCtx.reqCtx, qaModeAgent, false)
	}()

	waitFor(t, 2*time.Second, func() bool {
		return len(messageService.executionMetaPatches) >= 2
	})
	reqCtx.cancelRequest()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("executeQA did not finish after request cancellation")
	}

	last := mustExecutionMetaMap(t, messageService.executionMetaPatches[len(messageService.executionMetaPatches)-1].meta)
	if last["error_stage"] != "Agent QA panic" {
		t.Fatalf("expected error_stage=Agent QA panic, got %v", last["error_stage"])
	}
}

func mustExecutionMetaMap(t *testing.T, raw types.JSON) map[string]interface{} {
	t.Helper()

	meta, err := raw.Map()
	if err != nil {
		t.Fatalf("failed to parse execution_meta: %v", err)
	}
	return meta
}

func mustStringSlice(t *testing.T, value interface{}) []string {
	t.Helper()

	items, ok := value.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", value)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("expected string item, got %T", item)
		}
		result = append(result, s)
	}
	return result
}

type testQARequestContext struct {
	reqCtx        *qaRequestContext
	cancelRequest context.CancelFunc
}

func newTestQARequestContext(t *testing.T, sessionID, query string, agent *types.CustomAgent) *testQARequestContext {
	t.Helper()

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	baseReq := httptest.NewRequest("POST", "/api/v1/agent-chat/"+sessionID, nil)
	reqCtx, cancel := context.WithCancel(context.Background())
	ginCtx.Request = baseReq.WithContext(reqCtx)

	return &testQARequestContext{
		reqCtx: &qaRequestContext{
			ctx:       reqCtx,
			c:         ginCtx,
			sessionID: sessionID,
			requestID: "req-" + sessionID,
			query:     query,
			session: &types.Session{
				ID:       sessionID,
				TenantID: 1,
				Title:    "existing title",
			},
			customAgent: agent,
			assistantMessage: &types.Message{
				SessionID: sessionID,
				RequestID: "req-" + sessionID,
				Role:      "assistant",
			},
			knowledgeBaseIDs: []string{"kb-1"},
			knowledgeIDs:     []string{"doc-1"},
			enableMemory:     true,
			webSearchEnabled: true,
			channel:          "web",
		},
		cancelRequest: cancel,
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
