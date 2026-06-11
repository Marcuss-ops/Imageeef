package workers

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"velox-server/internal/store"
)

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

// ListRevoked returns the list of revoked worker IDs
func (wr *WorkerRegistry) ListRevoked() []string {
	wr.mu.RLock()
	defer wr.mu.RUnlock()
	ids := make([]string, 0, len(wr.revoked))
	for id := range wr.revoked {
		ids = append(ids, id)
	}
	return ids
}
