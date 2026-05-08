package im

import (
	"testing"

	"gorm.io/gorm"
)

func TestValidateSessionMode(t *testing.T) {
	tests := []struct {
		name        string
		sessionMode string
		wantErr     bool
	}{
		{"user mode", "user", false},
		{"thread mode", "thread", false},
		{"empty defaults to user in BeforeCreate", "", false},
		{"invalid mode", "invalid", true},
		{"random string", "foo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &IMChannel{SessionMode: tt.sessionMode}
			err := ch.validateSessionMode()

			if tt.sessionMode == "" {
				// empty is handled by BeforeCreate, not validateSessionMode
				// validateSessionMode treats empty as invalid
				if err == nil {
					t.Error("expected error for empty session_mode in validateSessionMode")
				}
				return
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("validateSessionMode(%q) error = %v, wantErr %v", tt.sessionMode, err, tt.wantErr)
			}
		})
	}
}

func TestIMChannelBeforeCreate_SessionModeDefault(t *testing.T) {
	tests := []struct {
		name               string
		platform           string
		inputMode          string
		expectedSession    string
		expectedChannelMode string
		expectedOut        string
	}{
		{name: "empty defaults to user", platform: "slack", inputMode: "", expectedSession: "user", expectedChannelMode: "websocket", expectedOut: "stream"},
		{name: "user preserved", platform: "slack", inputMode: "user", expectedSession: "user", expectedChannelMode: "websocket", expectedOut: "stream"},
		{name: "thread preserved", platform: "slack", inputMode: "thread", expectedSession: "thread", expectedChannelMode: "websocket", expectedOut: "stream"},
		{name: "wechat forced longpoll", platform: "wechat", inputMode: "", expectedSession: "user", expectedChannelMode: "longpoll", expectedOut: "full"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &IMChannel{
				TenantID:    1,
				AgentID:     "agent-1",
				Platform:    tt.platform,
				SessionMode: tt.inputMode,
				Credentials: []byte("{}"),
			}
			err := ch.BeforeCreate(&gorm.DB{})
			if err != nil {
				t.Fatalf("BeforeCreate error: %v", err)
			}
			if ch.SessionMode != tt.expectedSession {
				t.Errorf("SessionMode = %q, want %q", ch.SessionMode, tt.expectedSession)
			}
			if ch.Mode != tt.expectedChannelMode {
				t.Errorf("Mode = %q, want %q", ch.Mode, tt.expectedChannelMode)
			}
			if ch.OutputMode != tt.expectedOut {
				t.Errorf("OutputMode = %q, want %q", ch.OutputMode, tt.expectedOut)
			}
		})
	}
}

func TestIMChannelBeforeCreate_InvalidSessionMode(t *testing.T) {
	ch := &IMChannel{
		TenantID:    1,
		AgentID:     "agent-1",
		Platform:    "slack",
		SessionMode: "invalid",
		Credentials: []byte("{}"),
	}
	err := ch.BeforeCreate(&gorm.DB{})
	if err == nil {
		t.Error("expected error for invalid session_mode")
	}
}

func TestIMChannelBeforeSave_SessionModeValidation(t *testing.T) {
	tests := []struct {
		name            string
		platform        string
		sessionMode     string
		wantSessionMode string
		wantChannelMode string
		wantOut         string
		wantErr         bool
	}{
		{name: "empty defaults to user", platform: "slack", sessionMode: "", wantSessionMode: "user", wantChannelMode: "websocket", wantOut: "stream", wantErr: false},
		{name: "user preserved", platform: "slack", sessionMode: "user", wantSessionMode: "user", wantChannelMode: "websocket", wantOut: "stream", wantErr: false},
		{name: "thread preserved", platform: "slack", sessionMode: "thread", wantSessionMode: "thread", wantChannelMode: "websocket", wantOut: "stream", wantErr: false},
		{name: "wechat forced longpoll", platform: "wechat", sessionMode: "", wantSessionMode: "user", wantChannelMode: "longpoll", wantOut: "full", wantErr: false},
		{name: "invalid rejected", platform: "slack", sessionMode: "invalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &IMChannel{
				Platform:    tt.platform,
				SessionMode: tt.sessionMode,
				Credentials: []byte("{}"),
			}
			err := ch.BeforeSave(&gorm.DB{})
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ch.SessionMode != tt.wantSessionMode {
				t.Errorf("SessionMode = %q, want %q", ch.SessionMode, tt.wantSessionMode)
			}
			if ch.Mode != tt.wantChannelMode {
				t.Errorf("Mode = %q, want %q", ch.Mode, tt.wantChannelMode)
			}
			if ch.OutputMode != tt.wantOut {
				t.Errorf("OutputMode = %q, want %q", ch.OutputMode, tt.wantOut)
			}
		})
	}
}

func TestSessionModeConstants(t *testing.T) {
	if SessionModeUser != "user" {
		t.Errorf("SessionModeUser = %q, want %q", SessionModeUser, "user")
	}
	if SessionModeThread != "thread" {
		t.Errorf("SessionModeThread = %q, want %q", SessionModeThread, "thread")
	}
}

func TestChannelSessionThreadIDField(t *testing.T) {
	cs := ChannelSession{
		Platform: "slack",
		UserID:   "U123",
		ChatID:   "C456",
		ThreadID: "1234567890.123456",
	}
	if cs.ThreadID != "1234567890.123456" {
		t.Errorf("ThreadID = %q, want %q", cs.ThreadID, "1234567890.123456")
	}

	// empty ThreadID for user-mode sessions
	csUser := ChannelSession{
		Platform: "slack",
		UserID:   "U123",
		ChatID:   "C456",
	}
	if csUser.ThreadID != "" {
		t.Errorf("ThreadID = %q, want empty", csUser.ThreadID)
	}
}
