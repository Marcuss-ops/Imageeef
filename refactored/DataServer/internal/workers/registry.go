package workers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"velox-server/internal/store"
)

const workerPrefix = "velox:worker:"

// WorkerInfo contains all information about a registered worker
type WorkerInfo struct {
	WorkerID      string                 `json:"worker_id"`
	WorkerName    string                 `json:"worker_name"`
	DisplayName   string                 `json:"display_name"`
	Status        string                 `json:"status"`
	LastHB        string                 `json:"last_heartbeat"`
	FirstSeen     string                 `json:"first_seen"`
	CurrentJob    string                 `json:"current_job"`
	Drain         bool                   `json:"drain"`
	Schedulable   bool                   `json:"schedulable"`
	WorkerGroup   string                 `json:"worker_group"`
	IPAddress     string                 `json:"ip_address"`
	Host          string                 `json:"host"`
	CodeVersion   string                 `json:"code_version"`
	BundleVersion string                 `json:"bundle_version"`
	BootID        string                 `json:"boot_id,omitempty"`
	BootTS        string                 `json:"boot_ts,omitempty"`
	Readiness     map[string]interface{} `json:"readiness,omitempty"`
	RecentLogs    []string               `json:"recent_logs,omitempty"`
	RecentErrors  []string               `json:"recent_errors,omitempty"`
	Metrics       map[string]interface{} `json:"metrics,omitempty"`
}

type Registry struct {
	mu       sync.RWMutex
	redis    *redis.Client
	inMem    map[string]WorkerInfo
	useRedis bool
	dbStore  *store.SQLiteStore
}

func New(rdb *redis.Client, useRedis bool, dbStore *store.SQLiteStore) *Registry {
	return &Registry{redis: rdb, inMem: make(map[string]WorkerInfo), useRedis: useRedis, dbStore: dbStore}
}

func (r *Registry) Heartbeat(ctx context.Context, workerID, workerName, status, currentJob string, extra map[string]interface{}) error {
	now := time.Now().UTC().Format(time.RFC3339)

	// Preserve existing state unless explicitly updated by heartbeat payload.
	r.mu.Lock()
	existing, hasExisting := r.inMem[workerID]
	r.mu.Unlock()

	info := WorkerInfo{
		WorkerID:    workerID,
		WorkerName:  workerName,
		Status:      status,
		LastHB:      now,
		CurrentJob:  currentJob,
		Schedulable: true,
	}
	if hasExisting {
		info = existing
		info.WorkerID = workerID
		if workerName != "" {
			info.WorkerName = workerName
		}
		info.Status = status
		info.LastHB = now
		info.CurrentJob = currentJob
	}

	if extra != nil {
		if v, ok := extra["drain"]; ok {
			if b, ok := v.(bool); ok {
				info.Drain = b
			}
		}
		if v, ok := extra["schedulable"]; ok {
			if b, ok := v.(bool); ok {
				info.Schedulable = b
			}
		}
		if v, ok := extra["worker_group"]; ok {
			if s, ok := v.(string); ok && s != "" {
				info.WorkerGroup = s
			}
		}
		if v, ok := extra["code_version"].(string); ok && v != "" {
			info.CodeVersion = v
		}
		if v, ok := extra["bundle_version"].(string); ok && v != "" {
			info.BundleVersion = v
		}
		if v, ok := extra["readiness"].(map[string]interface{}); ok {
			info.Readiness = v
		}
		if v, ok := extra["metrics"].(map[string]interface{}); ok {
			info.Metrics = v
		}
		if v, ok := extra["recent_logs"]; ok {
			info.RecentLogs = extractStringSlice(v)
		}
		if v, ok := extra["recent_errors"]; ok {
			info.RecentErrors = extractStringSlice(v)
		}
	}

	r.mu.Lock()
	r.inMem[workerID] = info
	r.mu.Unlock()
	if r.dbStore != nil {
		raw, _ := json.Marshal(info)
		if err := r.dbStore.UpsertWorker(raw); err != nil {
			log.Printf("sqlite upsert worker heartbeat failed: %v", err)
		}
	}
	if r.useRedis && r.redis != nil {
		key := workerPrefix + workerID
		extraJSON, _ := json.Marshal(extra)
		return r.redis.HSet(ctx, key,
			"worker_id", workerID,
			"worker_name", workerName,
			"status", status,
			"last_heartbeat", now,
			"current_job", currentJob,
			"extra", string(extraJSON),
			"drain", strconv.FormatBool(info.Drain),
			"schedulable", strconv.FormatBool(info.Schedulable),
			"worker_group", info.WorkerGroup,
		).Err()
	}
	return nil
}

func (r *Registry) IsRegistered(ctx context.Context, workerID string) bool {
	if r.useRedis && r.redis != nil {
		n, _ := r.redis.Exists(ctx, workerPrefix+workerID).Result()
		return n > 0
	}
	r.mu.RLock()
	_, ok := r.inMem[workerID]
	r.mu.RUnlock()
	return ok
}

// GetWorker returns a single worker's info by ID
func (r *Registry) GetWorker(ctx context.Context, workerID string) *WorkerInfo {
	if r.useRedis && r.redis != nil {
		m, err := r.redis.HGetAll(ctx, workerPrefix+workerID).Result()
		if err != nil || len(m) == 0 {
			return nil
		}
		drain := m["drain"] == "true"
		schedulable := m["schedulable"] != "false"
		info := &WorkerInfo{
			WorkerID:    m["worker_id"],
			WorkerName:  m["worker_name"],
			Status:      m["status"],
			LastHB:      m["last_heartbeat"],
			CurrentJob:  m["current_job"],
			Drain:       drain,
			Schedulable: schedulable,
			WorkerGroup: m["worker_group"],
		}
		if extraRaw, ok := m["extra"]; ok && extraRaw != "" {
			var extra map[string]interface{}
			if err := json.Unmarshal([]byte(extraRaw), &extra); err == nil {
				info.RecentLogs = extractStringSlice(extra["recent_logs"])
				info.RecentErrors = extractStringSlice(extra["recent_errors"])
			}
		}
		return info
	}
	r.mu.RLock()
	info, ok := r.inMem[workerID]
	r.mu.RUnlock()
	if !ok {
		return nil
	}
	return &info
}

func (r *Registry) List(ctx context.Context) []WorkerInfo {
	if r.useRedis && r.redis != nil {
		var out []WorkerInfo
		iter := r.redis.Scan(ctx, 0, workerPrefix+"*", 100).Iterator()
		for iter.Next(ctx) {
			m, _ := r.redis.HGetAll(ctx, iter.Val()).Result()
			if len(m) > 0 {
				info := WorkerInfo{
					WorkerID:   m["worker_id"],
					WorkerName: m["worker_name"],
					Status:     m["status"],
					LastHB:     m["last_heartbeat"],
					CurrentJob: m["current_job"],
				}
				if extraRaw, ok := m["extra"]; ok && extraRaw != "" {
					var extra map[string]interface{}
					if err := json.Unmarshal([]byte(extraRaw), &extra); err == nil {
						info.RecentLogs = extractStringSlice(extra["recent_logs"])
						info.RecentErrors = extractStringSlice(extra["recent_errors"])
					}
				}
				out = append(out, info)
			}
		}
		return out
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]WorkerInfo, 0, len(r.inMem))
	for _, v := range r.inMem {
		list = append(list, v)
	}
	return list
}

func extractStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, it := range t {
			if s, ok := it.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	default:
		return nil
	}
}

// RegisterWorker registers a new worker or updates an existing one
func (r *Registry) RegisterWorker(ctx context.Context, workerID, workerName, ipAddress string, extra map[string]interface{}) error {
	now := time.Now().UTC().Format(time.RFC3339)

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already registered (preserve first_seen, display_name, worker_group)
	existing, ok := r.inMem[workerID]
	firstSeen := now
	displayName := workerName
	workerGroup := ""

	if ok {
		firstSeen = existing.FirstSeen
		if existing.DisplayName != "" {
			displayName = existing.DisplayName
		}
		if existing.WorkerGroup != "" {
			workerGroup = existing.WorkerGroup
		}
	}

	// Extract extra fields
	if extra != nil {
		if v, ok := extra["display_name"].(string); ok && v != "" {
			displayName = v
		}
		if v, ok := extra["worker_group"].(string); ok && v != "" {
			workerGroup = v
		}
	}

	info := WorkerInfo{
		WorkerID:    workerID,
		WorkerName:  workerName,
		DisplayName: displayName,
		Status:      "online",
		LastHB:      now,
		FirstSeen:   firstSeen,
		IPAddress:   ipAddress,
		Host:        ipAddress,
		Schedulable: true,
		WorkerGroup: workerGroup,
	}

	r.inMem[workerID] = info
	if r.dbStore != nil {
		raw, _ := json.Marshal(info)
		if err := r.dbStore.UpsertWorker(raw); err != nil {
			log.Printf("sqlite upsert worker register failed: %v", err)
		}
	}

	if r.useRedis && r.redis != nil {
		key := workerPrefix + workerID
		return r.redis.HSet(ctx, key,
			"worker_id", workerID,
			"worker_name", workerName,
			"display_name", displayName,
			"status", "online",
			"last_heartbeat", now,
			"first_seen", firstSeen,
			"ip_address", ipAddress,
			"host", ipAddress,
			"schedulable", "true",
			"worker_group", workerGroup,
		).Err()
	}
	return nil
}

// UnregisterWorker removes a worker from the registry
func (r *Registry) UnregisterWorker(ctx context.Context, workerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.inMem, workerID)

	if r.useRedis && r.redis != nil {
		return r.redis.Del(ctx, workerPrefix+workerID).Err()
	}
	return nil
}

// UpdateWorker updates specific fields of a worker
func (r *Registry) UpdateWorker(ctx context.Context, workerID string, updates map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, ok := r.inMem[workerID]
	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	// Apply updates
	if v, ok := updates["worker_name"].(string); ok {
		info.WorkerName = v
	}
	if v, ok := updates["display_name"].(string); ok {
		info.DisplayName = v
	}
	if v, ok := updates["worker_group"].(string); ok {
		info.WorkerGroup = v
	}
	if v, ok := updates["status"].(string); ok {
		info.Status = v
	}
	if v, ok := updates["drain"].(bool); ok {
		info.Drain = v
	}
	if v, ok := updates["schedulable"].(bool); ok {
		info.Schedulable = v
	}
	if v, ok := updates["current_job"].(string); ok {
		info.CurrentJob = v
	}
	if v, ok := updates["code_version"].(string); ok {
		info.CodeVersion = v
	}
	if v, ok := updates["bundle_version"].(string); ok {
		info.BundleVersion = v
	}
	if v, ok := updates["ip_address"].(string); ok {
		info.IPAddress = v
		info.Host = v
	}
	if v, ok := updates["recent_logs"].([]string); ok {
		info.RecentLogs = v
	}
	if v, ok := updates["recent_errors"].([]string); ok {
		info.RecentErrors = v
	}
	if v, ok := updates["readiness"].(map[string]interface{}); ok {
		info.Readiness = v
	}
	if v, ok := updates["metrics"].(map[string]interface{}); ok {
		info.Metrics = v
	}

	info.LastHB = time.Now().UTC().Format(time.RFC3339)
	r.inMem[workerID] = info
	if r.dbStore != nil {
		raw, _ := json.Marshal(info)
		if err := r.dbStore.UpsertWorker(raw); err != nil {
			log.Printf("sqlite upsert worker update failed: %v", err)
		}
	}

	if r.useRedis && r.redis != nil {
		key := workerPrefix + workerID
		fields := make([]interface{}, 0, len(updates)*2)
		for k, v := range updates {
			fields = append(fields, k, fmt.Sprintf("%v", v))
		}
		if len(fields) > 0 {
			return r.redis.HSet(ctx, key, fields...).Err()
		}
	}
	return nil
}

// SetWorkerDrain sets the drain status for a worker
func (r *Registry) SetWorkerDrain(ctx context.Context, workerID string, drain bool) error {
	return r.UpdateWorker(ctx, workerID, map[string]interface{}{"drain": drain})
}

// SetWorkerGroup sets the group for a worker
func (r *Registry) SetWorkerGroup(ctx context.Context, workerID string, group string) error {
	return r.UpdateWorker(ctx, workerID, map[string]interface{}{"worker_group": group})
}

// GetWorkersByGroup returns all workers in a specific group
func (r *Registry) GetWorkersByGroup(ctx context.Context, group string) []WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []WorkerInfo
	for _, w := range r.inMem {
		if w.WorkerGroup == group {
			result = append(result, w)
		}
	}
	return result
}

// GetActiveWorkers returns workers that have sent a heartbeat recently
func (r *Registry) GetActiveWorkers(ctx context.Context, timeout time.Duration) []WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now().UTC()
	var result []WorkerInfo

	for _, w := range r.inMem {
		if w.LastHB != "" {
			t, err := time.Parse(time.RFC3339, w.LastHB)
			if err == nil && now.Sub(t.UTC()) < timeout {
				result = append(result, w)
			}
		}
	}
	return result
}

// GetSchedulableWorkers returns workers that can accept new jobs
func (r *Registry) GetSchedulableWorkers(ctx context.Context) []WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []WorkerInfo
	for _, w := range r.inMem {
		if w.Schedulable && !w.Drain && w.Status != "offline" {
			result = append(result, w)
		}
	}
	return result
}

// WorkerRegistry manages file-based worker persistence
type WorkerRegistry struct {
	mu       sync.RWMutex
	filePath string
	workers  map[string]WorkerInfo
	revoked  map[string]bool
	dbStore  *store.SQLiteStore
}

// NewWorkerRegistry creates a new file-based worker registry
func NewWorkerRegistry(dataDir string, dbStore *store.SQLiteStore) *WorkerRegistry {
	wr := &WorkerRegistry{
		filePath: filepath.Join(dataDir, "workers.json"),
		workers:  make(map[string]WorkerInfo),
		revoked:  make(map[string]bool),
		dbStore:  dbStore,
	}
	wr.load()
	return wr
}

// load reads workers from file
func (wr *WorkerRegistry) load() error {
	data, err := os.ReadFile(wr.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var persisted struct {
		Workers map[string]WorkerInfo `json:"workers"`
		Revoked []string              `json:"revoked,omitempty"`
	}

	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}

	wr.mu.Lock()
	wr.workers = persisted.Workers
	if wr.workers == nil {
		wr.workers = make(map[string]WorkerInfo)
	}
	for _, id := range persisted.Revoked {
		wr.revoked[id] = true
	}
	wr.mu.Unlock()

	return nil
}

// save writes workers to file
func (wr *WorkerRegistry) save() error {
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	var revoked []string
	for id := range wr.revoked {
		revoked = append(revoked, id)
	}

	persisted := struct {
		Workers map[string]WorkerInfo `json:"workers"`
		Revoked []string              `json:"revoked,omitempty"`
	}{
		Workers: wr.workers,
		Revoked: revoked,
	}

	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first, then rename for atomicity
	tempPath := wr.filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tempPath, wr.filePath); err != nil {
		return err
	}

	// Best-effort dual-write to SQLite
	if wr.dbStore != nil {
		rawWorkers := make(map[string][]byte, len(wr.workers))
		for id, w := range wr.workers {
			b, err := json.Marshal(w)
			if err != nil {
				continue
			}
			rawWorkers[id] = b
		}
		if err := wr.dbStore.ReplaceWorkers(rawWorkers, wr.revoked); err != nil {
			log.Printf("sqlite dual-write workers failed: %v", err)
		}
	}
	return nil
}

// Register registers or updates a worker
func (wr *WorkerRegistry) Register(workerID, workerName, ipAddress string) {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)

	existing, ok := wr.workers[workerID]
	firstSeen := now
	displayName := workerName

	if ok {
		firstSeen = existing.FirstSeen
		if existing.DisplayName != "" {
			displayName = existing.DisplayName
		}
	}

	wr.workers[workerID] = WorkerInfo{
		WorkerID:    workerID,
		WorkerName:  workerName,
		DisplayName: displayName,
		Status:      "online",
		LastHB:      now,
		FirstSeen:   firstSeen,
		IPAddress:   ipAddress,
		Host:        ipAddress,
		Schedulable: true,
	}

	go wr.save()
}

// IsRevoked checks if a worker is revoked
func (wr *WorkerRegistry) IsRevoked(workerID string) bool {
	wr.mu.RLock()
	defer wr.mu.RUnlock()
	return wr.revoked[workerID]
}

// RevokeWorker marks a worker as revoked
func (wr *WorkerRegistry) RevokeWorker(workerID string) {
	wr.mu.Lock()
	wr.revoked[workerID] = true
	delete(wr.workers, workerID)
	wr.mu.Unlock()
	go wr.save()
}

// UnrevokeWorker removes a worker from the revoked list
func (wr *WorkerRegistry) UnrevokeWorker(workerID string) {
	wr.mu.Lock()
	delete(wr.revoked, workerID)
	wr.mu.Unlock()
	go wr.save()
}

// GetWorker returns a worker by ID
func (wr *WorkerRegistry) GetWorker(workerID string) *WorkerInfo {
	wr.mu.RLock()
	defer wr.mu.RUnlock()
	if w, ok := wr.workers[workerID]; ok {
		return &w
	}
	return nil
}

// ListWorkers returns all workers
func (wr *WorkerRegistry) ListWorkers() []WorkerInfo {
	wr.mu.RLock()
	defer wr.mu.RUnlock()
	list := make([]WorkerInfo, 0, len(wr.workers))
	for _, w := range wr.workers {
		list = append(list, w)
	}
	return list
}

// GenerateWorkerID generates a unique worker ID
func GenerateWorkerID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CleanupStaleWorkers removes workers that haven't sent a heartbeat in the given duration
func (r *Registry) CleanupStaleWorkers(ctx context.Context, maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	count := 0

	for id, w := range r.inMem {
		if w.LastHB != "" {
			t, err := time.Parse(time.RFC3339, w.LastHB)
			if err == nil && now.Sub(t.UTC()) > maxAge {
				delete(r.inMem, id)
				count++
				log.Printf("🧹 Cleaned up stale worker: %s (last seen %s)", id, w.LastHB)
			}
		}
	}

	return count
}
