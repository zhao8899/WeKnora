package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

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
			name:  "knowledge selection falls back to fast",
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
