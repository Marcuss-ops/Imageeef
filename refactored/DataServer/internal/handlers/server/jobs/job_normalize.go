package jobs

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func fingerprintPayload(data map[string]interface{}) string {
	payload := map[string]string{
		"video_name":       getStringOrEmpty(data, "video_name"),
		"voiceovers":       getStringOrEmpty(data, "voiceovers_urls", "voiceovers"),
		"intro_clips":      getStringOrEmpty(data, "intro_clips_urls", "intro_clips", "intro_clip_paths"),
		"start_clips":      getStringOrEmpty(data, "start_clips_urls", "start_clips"),
		"middle_clips":     getStringOrEmpty(data, "middle_clips_urls", "middle_clips"),
		"end_clips":        getStringOrEmpty(data, "end_clips_urls", "end_clips"),
		"stock_clips":      getStringOrEmpty(data, "stock_clips_urls", "stock_clips"),
		"stock_clip_paths": getStringOrEmpty(data, "stock_clip_paths"),
		"background":       getStringOrEmpty(data, "background_path_url", "background"),
		"background_music": getStringOrEmpty(data, "background_music_urls", "background_music"),
		"entities":         getStringOrEmpty(data, "json_entities", "entities"),
		"output_video_id":  getStringOrEmpty(data, "output_video_id"),
		"video_mode":       getStringOrEmpty(data, "video_mode"),
	}

	dataBytes, _ := json.Marshal(payload)
	hash := sha256.Sum256(dataBytes)
	return fmt.Sprintf("%x", hash)
}

func getStringOrEmpty(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := data[key]; ok {
			switch v := val.(type) {
			case string:
				return strings.TrimSpace(v)
			case []interface{}:
				var parts []string
				for _, item := range v {
					if s, ok := item.(string); ok {
						parts = append(parts, strings.TrimSpace(s))
					}
				}
				return strings.Join(parts, "\n")
			}
		}
	}
	return ""
}

func normalizeList(val interface{}) string {
	switch v := val.(type) {
	case []interface{}:
		var parts []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		return strings.Join(parts, "\n")
	case string:
		return strings.TrimSpace(v)
	}
	return ""
}

func normalizeListToArray(val interface{}) []string {
	if val == nil {
		return nil
	}

	switch v := val.(type) {
	case []interface{}:
		var result []string
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				result = append(result, strings.TrimSpace(s))
			}
		}
		return result
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		if strings.Contains(s, "\n") {
			var result []string
			for _, line := range strings.Split(s, "\n") {
				if trimmed := strings.TrimSpace(line); trimmed != "" {
					result = append(result, trimmed)
				}
			}
			return result
		}
		return []string{s}
	}
	return nil
}

func extractDriveID(url string) string {
	s := strings.TrimSpace(url)
	if s == "" {
		return ""
	}

	patterns := []string{
		`/d/([A-Za-z0-9_-]{10,})`,
		`id=([A-Za-z0-9_-]{10,})`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if m := re.FindStringSubmatch(s); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	dataBytes, _ := json.Marshal(m)
	var result map[string]interface{}
	json.Unmarshal(dataBytes, &result)
	return result
}

func pickMappingForVoiceover(mapping map[string]interface{}, voiceoverURL string) (string, map[string]interface{}) {
	voID := extractDriveID(voiceoverURL)
	voBase := voiceoverURL
	if idx := strings.LastIndex(voiceoverURL, "/"); idx >= 0 {
		voBase = voiceoverURL[idx+1:]
	}
	if idx := strings.Index(voBase, "?"); idx >= 0 {
		voBase = voBase[:idx]
	}

	for outID, info := range mapping {
		infoMap, ok := info.(map[string]interface{})
		if !ok {
			continue
		}

		voName, _ := infoMap["voiceover_name"].(string)
		voPath, _ := infoMap["voiceover_path"].(string)

		if voiceoverURL != "" && voiceoverURL == voPath {
			return outID, infoMap
		}
		if voID != "" && voID == extractDriveID(voPath) {
			return outID, infoMap
		}
		if voBase != "" && voBase == voName {
			return outID, infoMap
		}
	}

	return "", nil
}

func splitByVoiceoverJobs(data map[string]interface{}) []map[string]interface{} {
	voiceoverList := normalizeListToArray(data["voiceovers"])
	if len(voiceoverList) == 0 {
		voiceoverList = normalizeListToArray(data["voiceovers_urls"])
	}

	if len(voiceoverList) == 0 {
		return []map[string]interface{}{data}
	}

	mapping, _ := data["output_video_mapping"].(map[string]interface{})

	result := make([]map[string]interface{}, 0)
	for _, vo := range voiceoverList {
		payload := deepCopyMap(data)
		payload["voiceovers"] = []string{vo}
		payload["voiceovers_urls"] = vo

		if len(mapping) > 0 {
			outID, info := pickMappingForVoiceover(mapping, vo)
			if outID != "" {
				payload["output_video_id"] = outID
				payload["output_video_mapping"] = map[string]interface{}{outID: info}
			}
		}

		if vmap, ok := payload["voiceover_channel_mapping"].(map[string]interface{}); ok && len(vmap) > 0 {
			voBase := vo
			if idx := strings.LastIndex(vo, "/"); idx >= 0 {
				voBase = vo[idx+1:]
			}
			if idx := strings.Index(voBase, "?"); idx >= 0 {
				voBase = voBase[:idx]
			}
			if mappedVal, exists := vmap[voBase]; exists {
				payload["voiceover_channel_mapping"] = map[string]interface{}{voBase: mappedVal}
			}
		}

		result = append(result, payload)
	}

	return result
}
