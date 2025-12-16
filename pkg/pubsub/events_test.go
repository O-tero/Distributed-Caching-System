package pubsub

import (
	"testing"
	"time"
)

func TestInvalidationEvent_Validate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		event   InvalidationEvent
		wantErr bool
	}{
		{
			name: "valid with keys",
			event: InvalidationEvent{
				Version:     EventVersion1,
				Service:     "cache-manager",
				Keys:        []string{"user:123", "user:456"},
				TriggeredAt: now,
				RequestID:   "req-123",
			},
			wantErr: false,
		},
		{
			name: "valid with pattern",
			event: InvalidationEvent{
				Version:     EventVersion1,
				Service:     "api-gateway",
				Pattern:     "users:*",
				TriggeredAt: now,
				RequestID:   "req-456",
			},
			wantErr: false,
		},
		{
			name: "valid with both keys and pattern",
			event: InvalidationEvent{
				Version:     EventVersion1,
				Service:     "cache-manager",
				Keys:        []string{"user:123"},
				Pattern:     "sessions:*",
				TriggeredAt: now,
				RequestID:   "req-789",
			},
			wantErr: false,
		},
		{
			name: "invalid version",
			event: InvalidationEvent{
				Version:     999,
				Service:     "cache-manager",
				Keys:        []string{"user:123"},
				TriggeredAt: now,
				RequestID:   "req-123",
			},
			wantErr: true,
		},
		{
			name: "missing service",
			event: InvalidationEvent{
				Version:     EventVersion1,
				Keys:        []string{"user:123"},
				TriggeredAt: now,
				RequestID:   "req-123",
			},
			wantErr: true,
		},
		{
			name: "missing keys and pattern",
			event: InvalidationEvent{
				Version:     EventVersion1,
				Service:     "cache-manager",
				TriggeredAt: now,
				RequestID:   "req-123",
			},
			wantErr: true,
		},
		{
			name: "zero triggered_at",
			event: InvalidationEvent{
				Version:   EventVersion1,
				Service:   "cache-manager",
				Keys:      []string{"user:123"},
				RequestID: "req-123",
			},
			wantErr: true,
		},
		{
			name: "missing request_id",
			event: InvalidationEvent{
				Version:     EventVersion1,
				Service:     "cache-manager",
				Keys:        []string{"user:123"},
				TriggeredAt: now,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInvalidationEvent_JSON(t *testing.T) {
	now := time.Now().Truncate(time.Second) // Truncate for JSON comparison

	event := InvalidationEvent{
		Version:     EventVersion1,
		Service:     "cache-manager",
		Keys:        []string{"user:123", "user:456"},
		Pattern:     "sessions:*",
		TriggeredAt: now,
		Meta:        map[string]string{"reason": "user_logout"},
		RequestID:   "req-123",
	}

	// Marshal to JSON
	data, err := event.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	// Unmarshal from JSON
	decoded, err := InvalidationEventFromJSON(data)
	if err != nil {
		t.Fatalf("InvalidationEventFromJSON() error = %v", err)
	}

	// Verify fields
	if decoded.Version != event.Version {
		t.Errorf("Version = %v, want %v", decoded.Version, event.Version)
	}
	if decoded.Service != event.Service {
		t.Errorf("Service = %v, want %v", decoded.Service, event.Service)
	}
	if len(decoded.Keys) != len(event.Keys) {
		t.Errorf("Keys length = %v, want %v", len(decoded.Keys), len(event.Keys))
	}
	if decoded.Pattern != event.Pattern {
		t.Errorf("Pattern = %v, want %v", decoded.Pattern, event.Pattern)
	}
	if !decoded.TriggeredAt.Equal(event.TriggeredAt) {
		t.Errorf("TriggeredAt = %v, want %v", decoded.TriggeredAt, event.TriggeredAt)
	}
	if decoded.Meta["reason"] != event.Meta["reason"] {
		t.Errorf("Meta[reason] = %v, want %v", decoded.Meta["reason"], event.Meta["reason"])
	}
	if decoded.RequestID != event.RequestID {
		t.Errorf("RequestID = %v, want %v", decoded.RequestID, event.RequestID)
	}
}

func TestRefreshEvent_Validate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		event   RefreshEvent
		wantErr bool
	}{
		{
			name: "valid",
			event: RefreshEvent{
				Version:     EventVersion1,
				Service:     "warming",
				Keys:        []string{"user:123", "user:456"},
				Priority:    5,
				TriggeredAt: now,
				RequestID:   "req-123",
			},
			wantErr: false,
		},
		{
			name: "invalid version",
			event: RefreshEvent{
				Version:     999,
				Service:     "warming",
				Keys:        []string{"user:123"},
				TriggeredAt: now,
				RequestID:   "req-123",
			},
			wantErr: true,
		},
		{
			name: "missing service",
			event: RefreshEvent{
				Version:     EventVersion1,
				Keys:        []string{"user:123"},
				TriggeredAt: now,
				RequestID:   "req-123",
			},
			wantErr: true,
		},
		{
			name: "empty keys",
			event: RefreshEvent{
				Version:     EventVersion1,
				Service:     "warming",
				Keys:        []string{},
				TriggeredAt: now,
				RequestID:   "req-123",
			},
			wantErr: true,
		},
		{
			name: "zero triggered_at",
			event: RefreshEvent{
				Version:   EventVersion1,
				Service:   "warming",
				Keys:      []string{"user:123"},
				RequestID: "req-123",
			},
			wantErr: true,
		},
		{
			name: "missing request_id",
			event: RefreshEvent{
				Version:     EventVersion1,
				Service:     "warming",
				Keys:        []string{"user:123"},
				TriggeredAt: now,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWarmCompletedEvent_Validate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		event   WarmCompletedEvent
		wantErr bool
	}{
		{
			name: "valid success",
			event: WarmCompletedEvent{
				Version:     EventVersion1,
				Service:     "warming",
				Status:      "success",
				Duration:    5 * time.Second,
				KeysWarmed:  100,
				KeysFailed:  0,
				CompletedAt: now,
				RequestID:   "req-123",
			},
			wantErr: false,
		},
		{
			name: "valid partial",
			event: WarmCompletedEvent{
				Version:     EventVersion1,
				Service:     "warming",
				Status:      "partial",
				Duration:    10 * time.Second,
				KeysWarmed:  80,
				KeysFailed:  20,
				Error:       "some keys failed to load",
				CompletedAt: now,
				RequestID:   "req-456",
			},
			wantErr: false,
		},
		{
			name: "valid failed",
			event: WarmCompletedEvent{
				Version:     EventVersion1,
				Service:     "warming",
				Status:      "failed",
				Duration:    2 * time.Second,
				KeysWarmed:  0,
				KeysFailed:  100,
				Error:       "database connection failed",
				CompletedAt: now,
				RequestID:   "req-789",
			},
			wantErr: false,
		},
		{
			name: "invalid status",
			event: WarmCompletedEvent{
				Version:     EventVersion1,
				Service:     "warming",
				Status:      "unknown",
				Duration:    5 * time.Second,
				KeysWarmed:  100,
				CompletedAt: now,
				RequestID:   "req-123",
			},
			wantErr: true,
		},
		{
			name: "negative duration",
			event: WarmCompletedEvent{
				Version:     EventVersion1,
				Service:     "warming",
				Status:      "success",
				Duration:    -1 * time.Second,
				KeysWarmed:  100,
				CompletedAt: now,
				RequestID:   "req-123",
			},
			wantErr: true,
		},
		{
			name: "negative keys_warmed",
			event: WarmCompletedEvent{
				Version:     EventVersion1,
				Service:     "warming",
				Status:      "success",
				Duration:    5 * time.Second,
				KeysWarmed:  -10,
				CompletedAt: now,
				RequestID:   "req-123",
			},
			wantErr: true,
		},
		{
			name: "zero completed_at",
			event: WarmCompletedEvent{
				Version:    EventVersion1,
				Service:    "warming",
				Status:     "success",
				Duration:   5 * time.Second,
				KeysWarmed: 100,
				RequestID:  "req-123",
			},
			wantErr: true,
		},
		{
			name: "missing request_id",
			event: WarmCompletedEvent{
				Version:     EventVersion1,
				Service:     "warming",
				Status:      "success",
				Duration:    5 * time.Second,
				KeysWarmed:  100,
				CompletedAt: now,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWarmCompletedEvent_JSON(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	event := WarmCompletedEvent{
		Version:     EventVersion1,
		Service:     "warming",
		Status:      "partial",
		Duration:    10 * time.Second,
		KeysWarmed:  80,
		KeysFailed:  20,
		Error:       "timeout on some keys",
		CompletedAt: now,
		Meta:        map[string]string{"batch_id": "batch-123"},
		RequestID:   "req-456",
	}

	// Marshal to JSON
	data, err := event.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	// Unmarshal from JSON
	decoded, err := WarmCompletedEventFromJSON(data)
	if err != nil {
		t.Fatalf("WarmCompletedEventFromJSON() error = %v", err)
	}

	// Verify fields
	if decoded.Status != event.Status {
		t.Errorf("Status = %v, want %v", decoded.Status, event.Status)
	}
	if decoded.Duration != event.Duration {
		t.Errorf("Duration = %v, want %v", decoded.Duration, event.Duration)
	}
	if decoded.KeysWarmed != event.KeysWarmed {
		t.Errorf("KeysWarmed = %v, want %v", decoded.KeysWarmed, event.KeysWarmed)
	}
	if decoded.KeysFailed != event.KeysFailed {
		t.Errorf("KeysFailed = %v, want %v", decoded.KeysFailed, event.KeysFailed)
	}
	if decoded.Error != event.Error {
		t.Errorf("Error = %v, want %v", decoded.Error, event.Error)
	}
	if !decoded.CompletedAt.Equal(event.CompletedAt) {
		t.Errorf("CompletedAt = %v, want %v", decoded.CompletedAt, event.CompletedAt)
	}
}