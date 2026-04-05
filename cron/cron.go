package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OutputChannel defines where job output is sent.
type OutputChannel string

const (
	ChannelChat OutputChannel = "chat" // default — pipe to agent chat window
)

// Job defines a scheduled task.
type Job struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Schedule     string        `json:"schedule"`      // cron expression (e.g. "*/5 * * * *")
	Instruction  string        `json:"instruction"`   // prompt sent to the agent
	AgentID      string        `json:"agent_id"`      // agent to run (empty = primary)
	Output       OutputChannel `json:"output"`         // where to send results
	Enabled      bool          `json:"enabled"`
	LastRunAt    *time.Time    `json:"last_run_at,omitempty"`
	LastStatus   string        `json:"last_status,omitempty"` // "success", "error", "running"
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// JobsDir returns the directory where cron jobs are stored.
func JobsDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "helm", "cron")
}

func jobPath(homeDir, id string) string {
	return filepath.Join(JobsDir(homeDir), id+".json")
}

// LoadAll reads all cron jobs from disk.
func LoadAll(homeDir string) ([]Job, error) {
	dir := JobsDir(homeDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var jobs []Job
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var j Job
		if err := json.Unmarshal(data, &j); err != nil {
			continue
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// Load reads a single cron job by ID.
func Load(homeDir, id string) (*Job, error) {
	data, err := os.ReadFile(jobPath(homeDir, id))
	if err != nil {
		return nil, fmt.Errorf("cron job %q not found: %w", id, err)
	}
	var j Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("parse cron job %q: %w", id, err)
	}
	return &j, nil
}

// Save writes a cron job to disk.
func Save(homeDir string, j *Job) error {
	dir := JobsDir(homeDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create cron dir: %w", err)
	}

	if j.ID == "" {
		j.ID = sanitizeID(j.Name)
	}
	if j.ID == "" {
		return fmt.Errorf("cron job must have a name")
	}
	if j.Output == "" {
		j.Output = ChannelChat
	}

	now := time.Now()
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	j.UpdatedAt = now

	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cron job: %w", err)
	}
	return os.WriteFile(jobPath(homeDir, j.ID), data, 0644)
}

// Delete removes a cron job by ID.
func Delete(homeDir, id string) error {
	path := jobPath(homeDir, id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("cron job %q not found", id)
	}
	return os.Remove(path)
}

// UpdateStatus updates the last run status of a job.
func UpdateStatus(homeDir string, id, status string) {
	j, err := Load(homeDir, id)
	if err != nil {
		return
	}
	now := time.Now()
	j.LastRunAt = &now
	j.LastStatus = status
	Save(homeDir, j)
}

func sanitizeID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// --- Cron Expression Parser ---

// NextRun calculates the next run time from a cron expression relative to `from`.
// Supports standard 5-field cron: minute hour day-of-month month day-of-week
// Also supports shorthand: @every 5m, @hourly, @daily, @weekly
func NextRun(schedule string, from time.Time) (time.Time, error) {
	schedule = strings.TrimSpace(schedule)

	// Handle shorthand
	if strings.HasPrefix(schedule, "@every ") {
		durStr := strings.TrimPrefix(schedule, "@every ")
		d, err := time.ParseDuration(durStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q: %w", durStr, err)
		}
		return from.Add(d), nil
	}

	switch schedule {
	case "@hourly":
		return nextCron("0 * * * *", from)
	case "@daily", "@midnight":
		return nextCron("0 0 * * *", from)
	case "@weekly":
		return nextCron("0 0 * * 0", from)
	default:
		return nextCron(schedule, from)
	}
}

func nextCron(expr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("invalid cron expression %q: expected 5 fields", expr)
	}

	// Try every minute for up to 366 days
	t := from.Truncate(time.Minute).Add(time.Minute)
	limit := from.Add(366 * 24 * time.Hour)
	for t.Before(limit) {
		if cronMatch(fields, t) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no next run found within a year for %q", expr)
}

func cronMatch(fields []string, t time.Time) bool {
	return fieldMatch(fields[0], t.Minute(), 0, 59) &&
		fieldMatch(fields[1], t.Hour(), 0, 23) &&
		fieldMatch(fields[2], t.Day(), 1, 31) &&
		fieldMatch(fields[3], int(t.Month()), 1, 12) &&
		fieldMatch(fields[4], int(t.Weekday()), 0, 6)
}

func fieldMatch(field string, val, min, max int) bool {
	if field == "*" {
		return true
	}

	// Handle */N (step)
	if strings.HasPrefix(field, "*/") {
		step := 0
		fmt.Sscanf(field[2:], "%d", &step)
		if step <= 0 {
			return false
		}
		return (val-min)%step == 0
	}

	// Handle comma-separated values
	for _, part := range strings.Split(field, ",") {
		// Handle range N-M
		if strings.Contains(part, "-") {
			var lo, hi int
			fmt.Sscanf(part, "%d-%d", &lo, &hi)
			if val >= lo && val <= hi {
				return true
			}
			continue
		}
		// Single value
		var n int
		fmt.Sscanf(part, "%d", &n)
		if n == val {
			return true
		}
	}
	return false
}

// --- Scheduler ---

// Scheduler manages running cron jobs on schedule.
type Scheduler struct {
	homeDir    string
	mu         sync.Mutex
	running    bool
	stopCh     chan struct{}
	onRun      func(job Job) // callback invoked when a job fires
}

// NewScheduler creates a new cron scheduler.
func NewScheduler(homeDir string, onRun func(Job)) *Scheduler {
	return &Scheduler{
		homeDir: homeDir,
		onRun:   onRun,
	}
}

// Start begins the scheduler loop. It checks jobs every 30 seconds.
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go s.loop()
}

// Stop halts the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stopCh)
}

// IsRunning returns whether the scheduler is active.
func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Run once immediately on start
	s.tick()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	jobs, err := LoadAll(s.homeDir)
	if err != nil {
		return
	}

	now := time.Now()
	for _, j := range jobs {
		if !j.Enabled {
			continue
		}
		if j.LastStatus == "running" {
			continue
		}

		// Calculate when this job should have last run
		var from time.Time
		if j.LastRunAt != nil {
			from = *j.LastRunAt
		} else {
			from = j.CreatedAt
		}

		next, err := NextRun(j.Schedule, from)
		if err != nil {
			continue
		}

		if now.After(next) || now.Equal(next) {
			if s.onRun != nil {
				go s.onRun(j)
			}
		}
	}
}
