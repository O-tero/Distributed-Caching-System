package warming

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Predictor predicts which cache keys are likely to be accessed in the near future.
// This interface allows plugging in different prediction algorithms, from simple
// heuristics to ML-based models.
type Predictor interface {
	PredictHotKeys(ctx context.Context, window time.Duration, limit int) ([]string, error)
}

// DefaultPredictor implements a lightweight heuristic-based predictor.
// Uses recent access patterns and growth rates to predict future hot keys.
//
// Algorithm:
// 1. Track access counts and timestamps for each key
// 2. Calculate access frequency (accesses per hour)
// 3. Calculate growth rate (recent vs historical frequency)
// 4. Score = frequency * (1 + growth_rate)
// 5. Return top N keys by score
//
// Trade-offs:
// - Less effective for sudden traffic spikes or new content
// - TODO: Replace with ML model for better accuracy
type DefaultPredictor struct {
	mu            sync.RWMutex
	accessLog     map[string]*AccessHistory
	windowSize    time.Duration
	decayFactor   float64
}

// AccessHistory tracks access patterns for a single key.
type AccessHistory struct {
	Key           string
	TotalAccesses int64
	RecentAccesses int64   
	FirstSeen     time.Time
	LastAccessed  time.Time
	AccessTimes   []time.Time 
}

// NewDefaultPredictor creates a new default predictor.
func NewDefaultPredictor() *DefaultPredictor {
	return &DefaultPredictor{
		accessLog:   make(map[string]*AccessHistory),
		windowSize:  1 * time.Hour,
		decayFactor: 0.9, 
	}
}

// RecordAccess records an access to a key for prediction.
// This should be called by cache-manager on every cache hit/miss.
func (p *DefaultPredictor) RecordAccess(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	history, exists := p.accessLog[key]
	if !exists {
		history = &AccessHistory{
			Key:         key,
			FirstSeen:   now,
			AccessTimes: make([]time.Time, 0, 100),
		}
		p.accessLog[key] = history
	}

	history.TotalAccesses++
	history.RecentAccesses++
	history.LastAccessed = now

	// Keep limited history (last 100 accesses)
	history.AccessTimes = append(history.AccessTimes, now)
	if len(history.AccessTimes) > 100 {
		history.AccessTimes = history.AccessTimes[1:]
	}
}

// PredictHotKeys predicts the top N keys likely to be accessed in the next window.
// Complexity: O(n log n) where n = total tracked keys
func (p *DefaultPredictor) PredictHotKeys(ctx context.Context, window time.Duration, limit int) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-window)

	// Calculate scores for all keys
	type keyScore struct {
		key   string
		score float64
	}

	scores := make([]keyScore, 0, len(p.accessLog))

	for key, history := range p.accessLog {
		score := p.calculateScore(history, now, cutoff)
		if score > 0 {
			scores = append(scores, keyScore{key: key, score: score})
		}
	}

	// Sort by score (highest first)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Take top N
	if limit > 0 && limit < len(scores) {
		scores = scores[:limit]
	}

	// Extract keys
	hotKeys := make([]string, len(scores))
	for i, ks := range scores {
		hotKeys[i] = ks.key
	}

	return hotKeys, nil
}

// calculateScore computes a prediction score for a key.
// Higher score = more likely to be accessed soon.
func (p *DefaultPredictor) calculateScore(history *AccessHistory, now, cutoff time.Time) float64 {
	if history.TotalAccesses == 0 {
		return 0
	}

	// Calculate frequency (accesses per hour)
	timeSinceFirst := now.Sub(history.FirstSeen).Hours()
	if timeSinceFirst == 0 {
		timeSinceFirst = 1
	}
	frequency := float64(history.TotalAccesses) / timeSinceFirst

	// Calculate recent activity (last hour)
	recentCount := 0
	for _, accessTime := range history.AccessTimes {
		if accessTime.After(cutoff) {
			recentCount++
		}
	}

	// Calculate growth rate
	recentFrequency := float64(recentCount)
	growthRate := 0.0
	if frequency > 0 {
		growthRate = (recentFrequency - frequency) / frequency
	}

	timeSinceLast := now.Sub(history.LastAccessed).Minutes()
	recencyBonus := 1.0
	if timeSinceLast < 5 {
		recencyBonus = 2.0 
	} else if timeSinceLast < 30 {
		recencyBonus = 1.5 
	}

	// Calculate final score
	// Score = frequency * (1 + growth_rate) * recency_bonus
	score := frequency * (1.0 + growthRate) * recencyBonus

	return score
}

// Cleanup removes old access history to prevent unbounded memory growth.
// Should be called periodically (e.g., daily).
func (p *DefaultPredictor) Cleanup(maxAge time.Duration) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-maxAge)
	removed := 0

	for key, history := range p.accessLog {
		if history.LastAccessed.Before(cutoff) {
			delete(p.accessLog, key)
			removed++
		}
	}

	return removed
}

// GetStats returns statistics about the predictor's state.
func (p *DefaultPredictor) GetStats() PredictorStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	totalAccesses := int64(0)
	for _, history := range p.accessLog {
		totalAccesses += history.TotalAccesses
	}

	return PredictorStats{
		TrackedKeys:   len(p.accessLog),
		TotalAccesses: totalAccesses,
	}
}

type PredictorStats struct {
	TrackedKeys   int   `json:"tracked_keys"`
	TotalAccesses int64 `json:"total_accesses"`
}

// MLPredictor is a placeholder for ML-based prediction.
// TODO: Implement ML-based predictor using trained model.
//
// Implementation notes:
// - Train model offline using historical access logs
// - Features: time of day, day of week, recent trends, key metadata
// - Model: LSTM for time series, or simpler gradient boosting
// - Load model at startup from file or API
// - Run inference in PredictHotKeys() method
// - Periodically retrain model with new data
//
// Example integration:
// type MLPredictor struct {
//     model *tensorflow.SavedModel
//     preprocessor *FeaturePreprocessor
// }
//
// func (p *MLPredictor) PredictHotKeys(ctx context.Context, window time.Duration, limit int) ([]string, error) {
//     features := p.preprocessor.ExtractFeatures(ctx, window)
//     predictions := p.model.Predict(features)
//     return p.topKKeys(predictions, limit), nil
// }
type MLPredictor struct {
	// TODO: Add ML model fields
}

// NewMLPredictor creates a new ML-based predictor.
// TODO: Implement once model is trained and available.
func NewMLPredictor() *MLPredictor {
	return &MLPredictor{}
}

// PredictHotKeys predicts hot keys using ML model.
// TODO: Implement ML inference.
func (p *MLPredictor) PredictHotKeys(ctx context.Context, window time.Duration, limit int) ([]string, error) {
	// Placeholder: return empty list
	// In production, this would:
	// 1. Load recent access patterns
	// 2. Extract features (time, trends, metadata)
	// 3. Run model inference
	// 4. Return top K predicted keys
	return []string{}, nil
}