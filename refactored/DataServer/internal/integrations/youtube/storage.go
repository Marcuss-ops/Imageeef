// Package youtube provides YouTube Data API integration and management functionality.
package youtube

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Custom errors
var (
	ErrGroupExists         = errors.New("group already exists")
	ErrGroupNotFound       = errors.New("group not found")
	ErrTargetGroupNotFound = errors.New("target group not found")
	ErrChannelExists       = errors.New("channel already in group")
	ErrChannelNotFound     = errors.New("channel not found")
)

// Storage handles persistence of YouTube manager data
type Storage struct {
	mu       sync.RWMutex
	data     *StorageData
	filePath string
}

// NewStorage creates a new Storage instance
func NewStorage(dataDir string) (*Storage, error) {
	// Default data directory
	if dataDir == "" {
		dataDir = "./data"
	}

	// Use the GroupYoutubeManager directory for ChannelsSaved.json
	ytDir := filepath.Join(dataDir, "youtube", "GroupYoutubeManager")
	if err := os.MkdirAll(ytDir, 0755); err != nil {
		return nil, err
	}

	// Use ChannelsSaved.json as the storage file
	filePath := filepath.Join(ytDir, "ChannelsSaved.json")

	s := &Storage{
		filePath: filePath,
		data: &StorageData{
			Groups: make(map[string]*Group),
		},
	}

	// Load existing data
	if err := s.load(); err != nil {
		log.Printf("⚠️ YouTube storage: starting with empty data (%v)", err)
	}

	return s, nil
}

// load reads data from the JSON file (accepts Python-style ISO times without timezone)
func (s *Storage) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, use defaults
		}
		return err
	}

	var raw storageDataLoad
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.data.TrackedNiches = raw.TrackedNiches
	s.data.Groups = make(map[string]*Group)
	for name, g := range raw.Groups {	group := &Group{Name: name}
			group.CreatedAt = parseFlexTime(g.CreatedAt)
			group.Channels = make([]Channel, 0, len(g.Channels))
			for _, c := range g.Channels {
				ch := Channel{
					ID: c.ID, URL: c.URL, Title: c.Title, Name: c.Name, Thumbnail: c.Thumbnail,
					Notes: c.Notes, Keywords: c.Keywords, ViewCount: c.ViewCount, SubCount: c.SubCount,
					Language: c.Language,
				}
				ch.AddedAt = parseFlexTime(c.AddedAt)
				ch.LastSync = parseFlexTime(c.LastSync)
				group.Channels = append(group.Channels, ch)
		}
		s.data.Groups[name] = group
	}
	return nil
}

type storageDataLoad struct {
	Groups        map[string]*groupLoad `json:"groups"`
	TrackedNiches []string              `json:"tracked_niches,omitempty"`
}
type groupLoad struct {
	Name      string        `json:"name"`
	CreatedAt string        `json:"created_at"`
	Channels  []channelLoad `json:"channels"`
}
type channelLoad struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	Name      string   `json:"name,omitempty"`
	Thumbnail string   `json:"thumbnail"`
	Notes     string   `json:"notes,omitempty"`
	AddedAt   string   `json:"added_at"`
	Keywords  []string `json:"keywords,omitempty"`
	ViewCount int64    `json:"view_count,omitempty"`
	SubCount  int64    `json:"subscriber_count,omitempty"`
	Language  string   `json:"language,omitempty"`
	LastSync  string   `json:"last_sync,omitempty"`
}

func parseFlexTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z07:00",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// save writes data to the JSON file atomically
func (s *Storage) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file then rename
	tempPath := s.filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempPath, s.filePath)
}

// LoadData returns the current storage data
func (s *Storage) LoadData() *StorageData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent race conditions
	data := &StorageData{
		Groups:        make(map[string]*Group),
		TrackedNiches: s.data.TrackedNiches,
	}
	for k, v := range s.data.Groups {
		group := *v
		group.Channels = make([]Channel, len(v.Channels))
		copy(group.Channels, v.Channels)
		data.Groups[k] = &group
	}
	return data
}

// SaveData replaces the storage data
func (s *Storage) SaveData(data *StorageData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = data
	return s.save()
}

// ListGroups returns all groups
func (s *Storage) ListGroups() (map[string]*Group, []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groups := make(map[string]*Group)
	for k, v := range s.data.Groups {
		group := *v
		group.Channels = make([]Channel, len(v.Channels))
		copy(group.Channels, v.Channels)
		groups[k] = &group
	}

	return groups, s.data.TrackedNiches
}

// GetGroup returns a specific group
func (s *Storage) GetGroup(name string) (*Group, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	group, ok := s.data.Groups[name]
	if !ok {
		return nil, false
	}

	// Return a copy
	g := *group
	g.Channels = make([]Channel, len(group.Channels))
	copy(g.Channels, group.Channels)
	return &g, true
}

// CreateGroup creates a new group with the specified type ("upload", "manager", or empty)
func (s *Storage) CreateGroup(name string, groupType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data.Groups[name]; exists {
		return ErrGroupExists
	}

	s.data.Groups[name] = &Group{
		Name:      name,
		CreatedAt: time.Now(),
		Channels:  []Channel{},
		GroupType: groupType,
	}

	return s.save()
}

// DeleteGroup removes a group
func (s *Storage) DeleteGroup(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data.Groups[name]; !exists {
		return ErrGroupNotFound
	}

	delete(s.data.Groups, name)
	return s.save()
}

// AddChannel adds a channel to a group
func (s *Storage) AddChannel(groupName string, channel Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.data.Groups[groupName]
	if !ok {
		return ErrGroupNotFound
	}

	// Check for duplicates
	for _, ch := range group.Channels {
		if ch.URL == channel.URL {
			return ErrChannelExists
		}
	}

	group.Channels = append(group.Channels, channel)
	return s.save()
}

// RemoveChannel removes a channel from a group
func (s *Storage) RemoveChannel(groupName, channelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.data.Groups[groupName]
	if !ok {
		return ErrGroupNotFound
	}

	for i, ch := range group.Channels {
		if ch.ID == channelID {
			group.Channels = append(group.Channels[:i], group.Channels[i+1:]...)
			return s.save()
		}
	}

	return ErrChannelNotFound
}

// MoveChannel moves a channel from one group to another
func (s *Storage) MoveChannel(sourceGroup, channelID, targetGroup string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	source, ok := s.data.Groups[sourceGroup]
	if !ok {
		return ErrGroupNotFound
	}

	target, ok := s.data.Groups[targetGroup]
	if !ok {
		return ErrTargetGroupNotFound
	}

	// Find the channel
	var channel *Channel
	var channelIdx = -1
	for i, ch := range source.Channels {
		if ch.ID == channelID {
			channelCopy := ch
			channel = &channelCopy
			channelIdx = i
			break
		}
	}

	if channel == nil {
		return ErrChannelNotFound
	}

	// Check if already in target
	for _, ch := range target.Channels {
		if ch.URL == channel.URL {
			// Just remove from source
			source.Channels = append(source.Channels[:channelIdx], source.Channels[channelIdx+1:]...)
			return s.save()
		}
	}

	// Add to target
	target.Channels = append(target.Channels, *channel)
	// Remove from source
	source.Channels = append(source.Channels[:channelIdx], source.Channels[channelIdx+1:]...)

	return s.save()
}

// UpdateChannelLanguage updates the language for a channel in a group.
// Returns the updated channel and any error.
func (s *Storage) UpdateChannelLanguage(groupName, channelID, language string) (*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.data.Groups[groupName]
	if !ok {
		return nil, ErrGroupNotFound
	}

	for i, ch := range group.Channels {
		if ch.ID == channelID {
			group.Channels[i].Language = language
			if err := s.save(); err != nil {
				return nil, err
			}
			result := group.Channels[i]
			return &result, nil
		}
	}

	return nil, ErrChannelNotFound
}

// UpdateChannelMetadata updates Title, Name, and Thumbnail for a channel in a group.
func (s *Storage) UpdateChannelMetadata(groupName, channelID, title, name, thumbnail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.data.Groups[groupName]
	if !ok {
		return ErrGroupNotFound
	}

	for i, ch := range group.Channels {
		if ch.ID == channelID {
			if title != "" {
				group.Channels[i].Title = title
			}
			if name != "" {
				group.Channels[i].Name = name
			}
			if thumbnail != "" {
				group.Channels[i].Thumbnail = thumbnail
			}
			return s.save()
		}
	}

	return ErrChannelNotFound
}

// UpdateChannelStats updates the stats for a channel
func (s *Storage) UpdateChannelStats(groupName, channelID string, viewCount, subCount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, ok := s.data.Groups[groupName]
	if !ok {
		return ErrGroupNotFound
	}

	for i, ch := range group.Channels {
		if ch.ID == channelID {
			group.Channels[i].ViewCount = viewCount
			group.Channels[i].SubCount = subCount
			group.Channels[i].LastSync = time.Now()
			return s.save()
		}
	}

	return ErrChannelNotFound
}

// GetAllChannels returns all channels across all groups
func (s *Storage) GetAllChannels() []Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var channels []Channel
	for _, group := range s.data.Groups {
		channels = append(channels, group.Channels...)
	}
	return channels
}

// GetGroupChannels returns channel IDs for a specific group
func (s *Storage) GetGroupChannels(groupName string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	group, ok := s.data.Groups[groupName]
	if !ok {
		return nil, ErrGroupNotFound
	}

	var urls []string
	for _, ch := range group.Channels {
		urls = append(urls, ch.URL)
	}
	return urls, nil
}

// CleanupOldData removes cached channel metadata that is older than the retention period.
// This is required to comply with YouTube's data retention policies (max 13 days).
// Purges ALL YouTube API-derived fields: Title, Thumbnail, ViewCount, SubCount, Keywords.
func (s *Storage) CleanupOldData(retention time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	removedCount := 0

	// We don't remove the channel itself from the group (to keep the tracking link),
	// but we purge ALL "YouTube API Content" fields if not synced recently.
	for _, group := range s.data.Groups {
		for i := range group.Channels {
			ch := &group.Channels[i]
			if !ch.LastSync.IsZero() && now.Sub(ch.LastSync) > retention {
				// Purge ALL YouTube API-derived content to comply with data retention policies
				ch.Title = ""
				ch.Thumbnail = ""
				ch.ViewCount = 0
				ch.SubCount = 0
				ch.Keywords = nil
				removedCount++
			}
		}
	}

	if removedCount > 0 {
		s.save()
	}
	return removedCount
}

// ClearCache invalidates any cached data (for compatibility with Python)
func (s *Storage) ClearCache() {
	// No-op for now since we don't cache reads
	// In future, this could clear feed cache
}

// MigrateFromGroupsJSON migrates upload groups from the old groups.json format
// into the unified Storage with GroupType="upload".
// It enriches channels with OAuth metadata from channels.json when available.
// Returns the number of groups migrated or an error.
func (s *Storage) MigrateFromGroupsJSON(groupsJSONPath, channelsJSONPath string) (int, error) {
	data, err := os.ReadFile(groupsJSONPath)
	if err != nil {
		return 0, err
	}

	// Try array format first (current Service format)
	var groupsArray []struct {
		Name        string   `json:"name"`
		Channels    []string `json:"channels"`
		Description string   `json:"description,omitempty"`
		Privacy     string   `json:"privacy,omitempty"`
	}
	if err := json.Unmarshal(data, &groupsArray); err == nil && len(groupsArray) > 0 {
		return s.migrateGroupsArray(groupsArray, channelsJSONPath)
	}

	// Try map format
	var groupsMap map[string]struct {
		Name        string   `json:"name"`
		Channels    []string `json:"channels"`
		Description string   `json:"description,omitempty"`
		Privacy     string   `json:"privacy,omitempty"`
	}
	if err := json.Unmarshal(data, &groupsMap); err == nil && len(groupsMap) > 0 {
		groupsArray2 := make([]struct {
			Name        string   `json:"name"`
			Channels    []string `json:"channels"`
			Description string   `json:"description,omitempty"`
			Privacy     string   `json:"privacy,omitempty"`
		}, 0, len(groupsMap))
		for _, g := range groupsMap {
			groupsArray2 = append(groupsArray2, g)
		}
		return s.migrateGroupsArray(groupsArray2, channelsJSONPath)
	}

	return 0, nil
}

func (s *Storage) migrateGroupsArray(groups []struct {
	Name        string   `json:"name"`
	Channels    []string `json:"channels"`
	Description string   `json:"description,omitempty"`
	Privacy     string   `json:"privacy,omitempty"`
}, channelsJSONPath string) (int, error) {
	// Load channels.json for metadata enrichment
	channelTitles := make(map[string]string)
	if channelsJSONPath != "" {
		if data, err := os.ReadFile(channelsJSONPath); err == nil {
			var channelData map[string]struct {
				Title string `json:"title"`
			}
			if err := json.Unmarshal(data, &channelData); err == nil {
				for id, info := range channelData {
					channelTitles[id] = info.Title
				}
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	migrated := 0
	for _, g := range groups {
		if g.Name == "" {
			continue
		}

		// Skip if group already exists in Storage (don't overwrite)
		if _, exists := s.data.Groups[g.Name]; exists {
			continue
		}

		channels := make([]Channel, 0, len(g.Channels))
		for _, chID := range g.Channels {
			title := channelTitles[chID]
			if title == "" {
				title = chID
			}
			channels = append(channels, Channel{
				ID:      chID,
				URL:     "https://www.youtube.com/channel/" + chID,
				Title:   title,
				AddedAt: time.Now(),
			})
		}

		s.data.Groups[g.Name] = &Group{
			Name:      g.Name,
			CreatedAt: time.Now(),
			Channels:  channels,
			GroupType: "upload",
		}
		migrated++
	}

	if migrated > 0 {
		if err := s.save(); err != nil {
			return migrated, err
		}
		log.Printf("✅ Migrated %d upload groups from groups.json to unified Storage", migrated)
	}

	return migrated, nil
}
