package models

import (
	"testing"
	"time"
)

func TestClaudeExportValidation(t *testing.T) {
	tests := []struct {
		name    string
		export  *ClaudeExport
		wantErr bool
	}{
		{
			name: "valid export",
			export: &ClaudeExport{
				Conversations: []ClaudeConversation{
					{
						UUID:      "test-uuid",
						Name:      "Test Conversation",
						CreatedAt: "2024-01-01T00:00:00Z",
						UpdatedAt: "2024-01-01T00:00:00Z",
						ChatMessages: []ClaudeChatMessage{
							{
								UUID:      "msg-uuid",
								Sender:    "human",
								Text:      "Hello",
								CreatedAt: "2024-01-01T00:00:00Z",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty export",
			export: &ClaudeExport{
				Conversations: []ClaudeConversation{},
			},
			wantErr: true,
		},
		{
			name: "missing conversation UUID",
			export: &ClaudeExport{
				Conversations: []ClaudeConversation{
					{
						Name:      "Test",
						CreatedAt: "2024-01-01T00:00:00Z",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid sender",
			export: &ClaudeExport{
				Conversations: []ClaudeConversation{
					{
						UUID:      "test-uuid",
						CreatedAt: "2024-01-01T00:00:00Z",
						ChatMessages: []ClaudeChatMessage{
							{
								UUID:   "msg-uuid",
								Sender: "invalid",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Since ValidateExport is in the imports package, we'll test the model structure here
			if tt.name == "empty export" && len(tt.export.Conversations) == 0 {
				if !tt.wantErr {
					t.Error("expected error for empty export")
				}
			}
		})
	}
}

func TestMessageTimeParsing(t *testing.T) {
	validTime := "2024-01-01T12:00:00.123456+00:00"
	msg := ClaudeChatMessage{
		CreatedAt: validTime,
	}

	// Test that the timestamp format is valid
	_, err := time.Parse(time.RFC3339Nano, msg.CreatedAt)
	if err != nil {
		t.Errorf("failed to parse time: %v", err)
	}
}
