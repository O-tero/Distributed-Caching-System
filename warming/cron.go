package warming

import (
	"context"
	"fmt"
	"sync"
	"time"

	"encore.dev/cron"
)

// Scheduler manages scheduled warming jobs using Encore cron.
type Scheduler struct {
	service *Service
	jobs    map[string]*ScheduledJob
	mu      sync.RWMutex
	stopChan chan struct{}
	wg      sync.WaitGroup
}

// ScheduledJob represents a recurring warming job.
type ScheduledJob struct {
	ID          string
	Name        string
	Schedule    string  // Cron expression
	Strategy    string
	KeyPattern  string
	Limit       int
	Priority    int
	Enabled     bool
	LastRun     *time.Time
	NextRun     *time.Time
	RunCount    int64
	FailCount   int64
}

// NewScheduler creates a new job scheduler.
func NewScheduler(service *Service) *Scheduler {
	return &Scheduler{
		service:  service,
		jobs:     make(map[string]*ScheduledJob),
		stopChan: make(chan struct{}),
	}
}

// Encore cron jobs for pre-defined warming schedules

// DailyWarmup warms critical cache keys daily at 2 AM.
var _ = cron.NewJob("daily-warmup", cron.JobConfig{
	Title:    "Daily Cache Warmup",
	Schedule: "0 2 * * *", // 2 AM daily
	Endpoint: DailyWarmup,
})

//encore:api private
func DailyWarmup(ctx context.Context) error {
	if svc == nil {
		return nil
	}

	// Trigger predictive warming for critical keys
	_, err := svc.TriggerPredictive(ctx)
	return err
}

// HourlyRefresh refreshes frequently accessed keys every hour.
var _ = cron.NewJob("hourly-refresh", cron.JobConfig{
	Title:    "Hourly Cache Refresh",
	Schedule: "0 * * * *", // Every hour
	Endpoint: HourlyRefresh,
})

//encore:api private
func HourlyRefresh(ctx context.Context) error {
	if svc == nil {
		return nil
	}

	// Predict hot keys for next hour and warm them
	hotKeys, err := svc.predictor.PredictHotKeys(ctx, 1*time.Hour, 50)
	if err != nil {
		return err
	}

	if len(hotKeys) == 0 {
		return nil
	}

	_, err = svc.WarmKey(ctx, &WarmKeyRequest{
		Keys:     hotKeys,
		Priority: 70,
		Strategy: "priority",
	})

	return err
}

// PeakHoursWarmup warms cache before expected peak hours (8 AM, 12 PM, 6 PM).
var _ = cron.NewJob("peak-hours-warmup", cron.JobConfig{
	Title:    "Peak Hours Cache Warmup",
	Schedule: "0 7,11,17 * * *", // 7 AM, 11 AM, 5 PM (1 hour before peaks)
	Endpoint: PeakHoursWarmup,
})

//encore:api private
func PeakHoursWarmup(ctx context.Context) error {
	if svc == nil {
		return nil
	}

	// Warm more aggressively before peak hours
	hotKeys, err := svc.predictor.PredictHotKeys(ctx, 2*time.Hour, 100)
	if err != nil {
		return err
	}

	if len(hotKeys) == 0 {
		return nil
	}

	_, err = svc.WarmKey(ctx, &WarmKeyRequest{
		Keys:     hotKeys,
		Priority: 90, // High priority
		Strategy: "priority",
	})

	return err
}

// RegisterJob registers a custom scheduled warming job.
func (s *Scheduler) RegisterJob(job *ScheduledJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("job %s already exists", job.ID)
	}

	// TODO: Parse and validate cron schedule
	// For now, just store the job
	s.jobs[job.ID] = job

	return nil
}

// UnregisterJob removes a scheduled job.
func (s *Scheduler) UnregisterJob(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[jobID]; !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	delete(s.jobs, jobID)
	return nil
}

// ListJobs returns all registered jobs.
func (s *Scheduler) ListJobs() []*ScheduledJob {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*ScheduledJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}

	return jobs
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}

// executeJob runs a scheduled warming job.
func (s *Scheduler) executeJob(ctx context.Context, job *ScheduledJob) error {
	if !job.Enabled {
		return nil
	}

	now := time.Now()
	job.LastRun = &now

	// Select strategy
	strategy, exists := s.service.strategies[job.Strategy]
	if !exists {
		job.FailCount++
		return fmt.Errorf("unknown strategy: %s", job.Strategy)
	}

	// Get keys to warm
	var keys []string
	if job.KeyPattern != "" {
		// Use predictor to find matching keys
		predicted, err := s.service.predictor.PredictHotKeys(ctx, 1*time.Hour, job.Limit)
		if err != nil {
			job.FailCount++
			return fmt.Errorf("prediction failed: %w", err)
		}
		keys = filterByPattern(predicted, job.KeyPattern)
	}

	if len(keys) == 0 {
		return nil // No keys to warm
	}

	// Plan tasks
	tasks, err := strategy.Plan(ctx, PlanOptions{
		Keys:     keys,
		Priority: job.Priority,
		Limit:    job.Limit,
	})
	if err != nil {
		job.FailCount++
		return fmt.Errorf("planning failed: %w", err)
	}

// Queue tasks
	queued := s.service.workerPool.QueueTasks(tasks)
	
	if queued > 0 {
		job.RunCount++
		s.service.metrics.JobsTotal.Add(int64(queued))
	}

	return nil
}