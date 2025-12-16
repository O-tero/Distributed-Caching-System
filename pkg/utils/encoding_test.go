package utils

import (
	"testing"
	"time"

	"encore.app/pkg/models"
	"encore.app/pkg/pubsub"
)

func TestMarshalUnmarshalEntry(t *testing.T) {
	now := time.Now().Truncate(time.Second) // Truncate for JSON comparison

	entry := &models.Entry{
		Key:         "user:123",
		Value:       []byte("test data"),
		CreatedAt:   now,
		LastAccess:  now,
		AccessCount: 42,
		TTL:         5 * time.Minute,
		Metadata: map[string]string{
			"source": "api",
			"region": "us-east-1",
		},
	}

	// Marshal
	data, err := MarshalEntry(entry)
	if err != nil {
		t.Fatalf("MarshalEntry() error = %v", err)
	}

	if len(data) == 0 {
		t.Fatal("MarshalEntry() returned empty data")
	}

	// Unmarshal
	decoded, err := UnmarshalEntry(data)
	if err != nil {
		t.Fatalf("UnmarshalEntry() error = %v", err)
	}

	// Verify fields
	if decoded.Key != entry.Key {
		t.Errorf("Key = %v, want %v", decoded.Key, entry.Key)
	}

	if string(decoded.Value) != string(entry.Value) {
		t.Errorf("Value = %v, want %v", string(decoded.Value), string(entry.Value))
	}

	if !decoded.CreatedAt.Equal(entry.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", decoded.CreatedAt, entry.CreatedAt)
	}

	if !decoded.LastAccess.Equal(entry.LastAccess) {
		t.Errorf("LastAccess = %v, want %v", decoded.LastAccess, entry.LastAccess)
	}

	if decoded.AccessCount != entry.AccessCount {
		t.Errorf("AccessCount = %v, want %v", decoded.AccessCount, entry.AccessCount)
	}

	if decoded.TTL != entry.TTL {
		t.Errorf("TTL = %v, want %v", decoded.TTL, entry.TTL)
	}

	if decoded.Metadata["source"] != entry.Metadata["source"] {
		t.Errorf("Metadata[source] = %v, want %v", decoded.Metadata["source"], entry.Metadata["source"])
	}
}

func TestMarshalEntry_Nil(t *testing.T) {
	_, err := MarshalEntry(nil)
	if err == nil {
		t.Error("MarshalEntry(nil) should return error")
	}
}

func TestUnmarshalEntry_Empty(t *testing.T) {
	_, err := UnmarshalEntry([]byte{})
	if err == nil {
		t.Error("UnmarshalEntry(empty) should return error")
	}
}

func TestUnmarshalEntry_Invalid(t *testing.T) {
	_, err := UnmarshalEntry([]byte("invalid json"))
	if err == nil {
		t.Error("UnmarshalEntry(invalid) should return error")
	}
}

func TestMarshalUnmarshalEvent_InvalidationEvent(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	event := &pubsub.InvalidationEvent{
		Version:     pubsub.EventVersion1,
		Service:     "cache-manager",
		Keys:        []string{"user:123", "user:456"},
		Pattern:     "sessions:*",
		TriggeredAt: now,
		Meta:        map[string]string{"reason": "logout"},
		RequestID:   "req-123",
	}

	// Marshal
	data, err := MarshalEvent(event)
	if err != nil {
		t.Fatalf("MarshalEvent() error = %v", err)
	}

	// Unmarshal
	var decoded pubsub.InvalidationEvent
	err = UnmarshalEvent(data, &decoded)
	if err != nil {
		t.Fatalf("UnmarshalEvent() error = %v", err)
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

	if decoded.RequestID != event.RequestID {
		t.Errorf("RequestID = %v, want %v", decoded.RequestID, event.RequestID)
	}
}

func TestMarshalUnmarshalEvent_WarmCompletedEvent(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	event := &pubsub.WarmCompletedEvent{
		Version:     pubsub.EventVersion1,
		Service:     "warming",
		Status:      "success",
		Duration:    5 * time.Second,
		KeysWarmed:  100,
		KeysFailed:  0,
		CompletedAt: now,
		Meta:        map[string]string{"batch_id": "batch-123"},
		RequestID:   "req-456",
	}

	// Marshal
	data, err := MarshalEvent(event)
	if err != nil {
		t.Fatalf("MarshalEvent() error = %v", err)
	}

	// Unmarshal
	var decoded pubsub.WarmCompletedEvent
	err = UnmarshalEvent(data, &decoded)
	if err != nil {
		t.Fatalf("UnmarshalEvent() error = %v", err)
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
}

func TestMarshalEvent_Nil(t *testing.T) {
	_, err := MarshalEvent(nil)
	if err == nil {
		t.Error("MarshalEvent(nil) should return error")
	}
}

func TestUnmarshalEvent_Nil(t *testing.T) {
	err := UnmarshalEvent([]byte("{}"), nil)
	if err == nil {
		t.Error("UnmarshalEvent() with nil pointer should return error")
	}
}

func TestUnmarshalEvent_Empty(t *testing.T) {
	var event pubsub.InvalidationEvent
	err := UnmarshalEvent([]byte{}, &event)
	if err == nil {
		t.Error("UnmarshalEvent(empty) should return error")
	}
}

func TestMarshalUnmarshalJSON(t *testing.T) {
	data := map[string]interface{}{
		"name":  "test",
		"count": 42,
		"tags":  []string{"tag1", "tag2"},
	}

	// Marshal
	encoded, err := MarshalJSON(data)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	// Unmarshal
	var decoded map[string]interface{}
	err = UnmarshalJSON(encoded, &decoded)
	if err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	// Verify
	if decoded["name"] != data["name"] {
		t.Errorf("name = %v, want %v", decoded["name"], data["name"])
	}

	// Note: JSON unmarshals numbers as float64
	if decoded["count"].(float64) != float64(data["count"].(int)) {
		t.Errorf("count = %v, want %v", decoded["count"], data["count"])
	}
}

func TestCompactJSON(t *testing.T) {
	pretty := []byte(`{
  "name": "test",
  "count": 42
}`)

	compacted, err := CompactJSON(pretty)
	if err != nil {
		t.Fatalf("CompactJSON() error = %v", err)
	}

	expected := `{"name":"test","count":42}`
	if string(compacted) != expected {
		t.Errorf("CompactJSON() = %s, want %s", string(compacted), expected)
	}
}

func TestCompactJSON_Invalid(t *testing.T) {
	_, err := CompactJSON([]byte("invalid json"))
	if err == nil {
		t.Error("CompactJSON(invalid) should return error")
	}
}

func TestPrettyJSON(t *testing.T) {
	compact := []byte(`{"name":"test","count":42}`)

	pretty, err := PrettyJSON(compact)
	if err != nil {
		t.Fatalf("PrettyJSON() error = %v", err)
	}

	// Check that it has newlines (indented)
	if len(pretty) <= len(compact) {
		t.Error("PrettyJSON() should produce larger output with formatting")
	}

	// Verify it's still valid JSON
	var v interface{}
	err = UnmarshalJSON(pretty, &v)
	if err != nil {
		t.Errorf("PrettyJSON() produced invalid JSON: %v", err)
	}
}

func TestPrettyJSON_Invalid(t *testing.T) {
	_, err := PrettyJSON([]byte("invalid json"))
	if err == nil {
		t.Error("PrettyJSON(invalid) should return error")
	}
}

func TestEstimateEncodedSize(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  int // Approximate expected size
	}{
		{"empty map", map[string]string{}, 2},             // "{}"
		{"small string", "hello", 7},                      // "hello"
		{"number", 42, 2},                                 // "42"
		{"array", []int{1, 2, 3}, 7},                      // "[1,2,3]"
		{"nested", map[string]int{"a": 1, "b": 2}, 13},   // Approx
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := EstimateEncodedSize(tt.value)
			
			// Allow some variance for encoding overhead
			if size < tt.want-2 || size > tt.want+10 {
				t.Errorf("EstimateEncodedSize() = %d, want ~%d", size, tt.want)
			}
		})
	}
}

func TestEstimateEncodedSize_Invalid(t *testing.T) {
	// Channels cannot be marshaled
	ch := make(chan int)
	size := EstimateEncodedSize(ch)
	if size != 0 {
		t.Errorf("EstimateEncodedSize(unmarshalable) = %d, want 0", size)
	}
}

func BenchmarkMarshalEntry(b *testing.B) {
	entry := &models.Entry{
		Key:         "user:123",
		Value:       []byte("test data with some content"),
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 42,
		TTL:         5 * time.Minute,
		Metadata: map[string]string{
			"source": "api",
			"region": "us-east-1",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MarshalEntry(entry)
	}
}

func BenchmarkUnmarshalEntry(b *testing.B) {
	entry := &models.Entry{
		Key:         "user:123",
		Value:       []byte("test data with some content"),
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 42,
		TTL:         5 * time.Minute,
	}

	data, _ := MarshalEntry(entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		UnmarshalEntry(data)
	}
}

func BenchmarkMarshalEvent(b *testing.B) {
	event := &pubsub.InvalidationEvent{
		Version:     pubsub.EventVersion1,
		Service:     "cache-manager",
		Keys:        []string{"user:123", "user:456", "user:789"},
		TriggeredAt: time.Now(),
		RequestID:   "req-123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MarshalEvent(event)
	}
}