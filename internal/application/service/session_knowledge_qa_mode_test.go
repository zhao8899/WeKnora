package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/agent/dispatcher"
	"github.com/Tencent/WeKnora/internal/types"
)

// testQueryRouter builds a small dispatcher for tests with the same route
// structure as the production newQueryRouter in container.go.
func testQueryRouter() *dispatcher.Dispatcher {
	d := dispatcher.New(nil)
	d.Register(&dispatcher.Route{
		Name:    "greeting",
		Keywords: []string{"你好", "hello", "hi"},
		Handler: string(types.ChatModeChat),
	})
	d.Register(&dispatcher.Route{
		Name:    "simple_qa",
		Keywords: []string{"什么是", "what is"},
		Handler: string(types.ChatModeRAGFast),
	})
	d.Register(&dispatcher.Route{
		Name:    "deep_analysis",
		Keywords: []string{"对比", "分析", "compare", "analyze"},
		Handler: string(types.ChatModeRAGDeep),
	})
	_ = d.SetDefault("simple_qa")
	return d
}

func TestResolveKnowledgeQAMode(t *testing.T) {
	svc := &sessionService{}
	ctx := context.Background()

	tests := []struct {
		name  string
		req   *types.QARequest
		hasKB bool
		want  types.ChatMode
	}{
		{
			name: "explicit chat mode wins",
			req: &types.QARequest{
				Mode:             types.ChatModeChat,
				WebSearchEnabled: true,
			},
			hasKB: true,
			want:  types.ChatModeChat,
		},
		{
			name: "explicit fast mode wins",
			req: &types.QARequest{
				Mode: types.ChatModeRAGFast,
			},
			hasKB: false,
			want:  types.ChatModeRAGFast,
		},
		{
			name: "explicit deep mode wins",
			req: &types.QARequest{
				Mode: types.ChatModeRAGDeep,
			},
			hasKB: false,
			want:  types.ChatModeRAGDeep,
		},
		{
			name: "web search falls back to deep",
			req: &types.QARequest{
				WebSearchEnabled: true,
			},
			hasKB: false,
			want:  types.ChatModeRAGDeep,
		},
		{
			name:  "knowledge selection falls back to fast (no router)",
			req:   &types.QARequest{},
			hasKB: true,
			want:  types.ChatModeRAGFast,
		},
		{
			name: "empty request falls back to chat",
			req:  &types.QARequest{},
			want: types.ChatModeChat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.resolveKnowledgeQAMode(ctx, tt.req, tt.hasKB)
			if got != tt.want {
				t.Fatalf("resolveKnowledgeQAMode() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestResolveKnowledgeQAMode_SmartRouting(t *testing.T) {
	svc := &sessionService{queryRouter: testQueryRouter()}
	ctx := context.Background()

	tests := []struct {
		name  string
		query string
		hasKB bool
		want  types.ChatMode
	}{
		{
			name:  "greeting routes to chat",
			query: "你好，请问有什么可以帮你",
			hasKB: true,
			want:  types.ChatModeChat,
		},
		{
			name:  "simple question routes to rag_fast",
			query: "什么是知识图谱",
			hasKB: true,
			want:  types.ChatModeRAGFast,
		},
		{
			name:  "analysis routes to rag_deep",
			query: "请对比这两种方案的优缺点并分析",
			hasKB: true,
			want:  types.ChatModeRAGDeep,
		},
		{
			name:  "unknown query falls back to default (rag_fast)",
			query: "一些随机的文本内容",
			hasKB: true,
			want:  types.ChatModeRAGFast,
		},
		{
			name:  "router skipped when no KB",
			query: "你好",
			hasKB: false,
			want:  types.ChatModeChat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &types.QARequest{Query: tt.query}
			got := svc.resolveKnowledgeQAMode(ctx, req, tt.hasKB)
			if got != tt.want {
				t.Fatalf("resolveKnowledgeQAMode(%q) = %s, want %s", tt.query, got, tt.want)
			}
		})
	}
}

func TestAssembleKnowledgeQAPipeline(t *testing.T) {
	tests := []struct {
		name         string
		req          *types.QARequest
		chatManage   *types.ChatManage
		mode         types.ChatMode
		supportsVLM  bool
		wantPipeline []types.EventType
		wantQuery    bool
		wantUserText string
	}{
		{
			name: "chat mode skips retrieval and sets fallback image text",
			req: &types.QARequest{
				Query:            "hello",
				ImageDescription: "image-desc",
			},
			chatManage: &types.ChatManage{
				PipelineRequest: types.PipelineRequest{
					MaxRounds:    2,
					EnableMemory: true,
				},
			},
			mode:        types.ChatModeChat,
			supportsVLM: false,
			wantPipeline: []types.EventType{
				types.LOAD_HISTORY,
				types.MEMORY_RETRIEVAL,
				types.CHAT_COMPLETION_STREAM,
				types.MEMORY_STORAGE,
			},
			wantUserText: "hello\n\n[用户上传图片内容]\nimage-desc",
		},
		{
			name: "rag fast skips rewrite web fetch and data analysis and clamps retrieval bounds",
			req: &types.QARequest{
				Query: "hello",
			},
			chatManage: &types.ChatManage{
				PipelineRequest: types.PipelineRequest{
					MaxRounds:            2,
					RerankModelID:        "rerank",
					EmbeddingTopK:        30,
					RerankTopK:           30,
					EnableRewrite:        true,
					EnableQueryExpansion: true,
					WebFetchEnabled:      true,
				},
			},
			mode:        types.ChatModeRAGFast,
			supportsVLM: true,
			wantPipeline: []types.EventType{
				types.LOAD_HISTORY,
				types.CHUNK_SEARCH_PARALLEL,
				types.CHUNK_RERANK,
				types.CHUNK_MERGE,
				types.FILTER_TOP_K,
				types.RETRIEVAL_GRADER,
				types.INTO_CHAT_MESSAGE,
				types.CHAT_COMPLETION_STREAM,
			},
		},
		{
			name: "rag deep keeps rewrite and web fetch when enabled",
			req: &types.QARequest{
				Query: "hello",
			},
			chatManage: &types.ChatManage{
				PipelineRequest: types.PipelineRequest{
					MaxRounds:            1,
					EnableRewrite:        true,
					WebSearchEnabled:     true,
					WebFetchEnabled:      true,
					EnableQueryExpansion: true,
				},
			},
			mode:        types.ChatModeRAGDeep,
			supportsVLM: true,
			wantPipeline: []types.EventType{
				types.LOAD_HISTORY,
				types.QUERY_UNDERSTAND,
				types.CHUNK_SEARCH_PARALLEL,
				types.CHUNK_RERANK,
				types.WEB_FETCH,
				types.CHUNK_MERGE,
				types.FILTER_TOP_K,
				types.RETRIEVAL_GRADER,
				types.DATA_ANALYSIS,
				types.INTO_CHAT_MESSAGE,
				types.CHAT_COMPLETION_STREAM,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assembleKnowledgeQAPipeline(tt.req, tt.chatManage, tt.mode, tt.supportsVLM)
			if len(got) != len(tt.wantPipeline) {
				t.Fatalf("pipeline length = %d, want %d; got=%v", len(got), len(tt.wantPipeline), got)
			}
			for i := range got {
				if got[i] != tt.wantPipeline[i] {
					t.Fatalf("pipeline[%d] = %s, want %s; full=%v", i, got[i], tt.wantPipeline[i], got)
				}
			}
			if tt.wantUserText != "" && tt.chatManage.UserContent != tt.wantUserText {
				t.Fatalf("user content = %q, want %q", tt.chatManage.UserContent, tt.wantUserText)
			}
			if tt.mode == types.ChatModeRAGFast {
				if tt.chatManage.EnableRewrite {
					t.Fatalf("rag_fast should disable rewrite")
				}
				if tt.chatManage.EnableQueryExpansion {
					t.Fatalf("rag_fast should disable query expansion")
				}
				if tt.chatManage.WebFetchEnabled {
					t.Fatalf("rag_fast should disable web fetch")
				}
				if tt.chatManage.EmbeddingTopK != 8 {
					t.Fatalf("rag_fast embedding top k = %d, want 8", tt.chatManage.EmbeddingTopK)
				}
				if tt.chatManage.RerankTopK != 5 {
					t.Fatalf("rag_fast rerank top k = %d, want 5", tt.chatManage.RerankTopK)
				}
			}
		})
	}
}
