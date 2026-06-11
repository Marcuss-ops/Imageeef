package script

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func normalizeStringList(source map[string]interface{}, keys ...string) []string {
	if source == nil {
		return nil
	}
	var values []string
	for _, key := range keys {
		v, ok := source[key]
		if !ok {
			continue
		}
		switch vv := v.(type) {
		case []string:
			values = append(values, vv...)
		case []interface{}:
			for _, item := range vv {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					values = append(values, strings.TrimSpace(s))
				}
			}
		case string:
			for _, line := range strings.Split(vv, "\n") {
				if s := strings.TrimSpace(line); s != "" {
					values = append(values, s)
				}
			}
		}
	}
	return dedupeStrings(values)
}

func firstNonEmptyString(source map[string]interface{}, keys ...string) string {
	if source == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := source[key]; ok {
			switch vv := v.(type) {
			case string:
				if s := strings.TrimSpace(vv); s != "" {
					return s
				}
			case fmt.Stringer:
				if s := strings.TrimSpace(vv.String()); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func floatFromPayload(source map[string]interface{}, fallback float64, keys ...string) float64 {
	if source == nil {
		return fallback
	}
	for _, key := range keys {
		if v, ok := source[key]; ok {
			switch vv := v.(type) {
			case float64:
				if vv > 0 {
					return vv
				}
			case float32:
				if vv > 0 {
					return float64(vv)
				}
			case int:
				if vv > 0 {
					return float64(vv)
				}
			case int64:
				if vv > 0 {
					return float64(vv)
				}
			case json.Number:
				if f, err := vv.Float64(); err == nil && f > 0 {
					return f
				}
			case string:
				if f, err := strconv.ParseFloat(strings.TrimSpace(vv), 64); err == nil && f > 0 {
					return f
				}
			}
		}
	}
	return fallback
}

func intFromPayload(source map[string]interface{}, fallback int, key string) int {
	if source == nil {
		return fallback
	}
	if v, ok := source[key]; ok {
		switch vv := v.(type) {
		case int:
			if vv > 0 {
				return vv
			}
		case int64:
			if vv > 0 {
				return int(vv)
			}
		case float64:
			if vv > 0 {
				return int(vv)
			}
		case json.Number:
			if n, err := vv.Int64(); err == nil && n > 0 {
				return int(n)
			}
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(vv)); err == nil && n > 0 {
				return n
			}
		}
	}
	return fallback
}

func ensureInt(value interface{}, fallback int) int {
	switch v := value.(type) {
	case int:
		if v > 0 {
			return v
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case float64:
		if v > 0 {
			return int(v)
		}
	case json.Number:
		if n, err := v.Int64(); err == nil && n > 0 {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func normalizedDuration(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f
	default:
		return 0
	}
}

func ensureRFC3339(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return value
	}
	return fallback
}

func sanitizeVideoName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('_')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func buildScriptText(payload map[string]interface{}) string {
	var parts []string
	if s := firstNonEmptyString(payload, "topic", "title"); s != "" {
		parts = append(parts, s)
	}
	if s := firstNonEmptyString(payload, "source_text"); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		parts = append(parts, "script with images")
	}
	return strings.Join(parts, " - ")
}

func isLikelyMediaSource(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	return strings.HasPrefix(value, "http://") ||
		strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "file://") ||
		strings.HasSuffix(value, ".mp4") ||
		strings.HasSuffix(value, ".mov") ||
		strings.HasSuffix(value, ".mkv") ||
		strings.HasSuffix(value, ".webm") ||
		strings.HasSuffix(value, ".mp3") ||
		strings.HasSuffix(value, ".wav") ||
		strings.HasSuffix(value, ".m4a")
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func mustJSON(v interface{}) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func renderJobResponse(job map[string]interface{}, full bool) map[string]interface{} {
	if job == nil {
		return map[string]interface{}{"ok": false}
	}
	response := map[string]interface{}{
		"ok":                  true,
		"job_id":              firstString(job, "job_id"),
		"script_id":           firstString(job, "job_id", "script_id"),
		"status":              firstString(job, "status"),
		"video_name":          firstString(job, "video_name", "title"),
		"job_run_id":          firstString(job, "job_run_id", "run_id"),
		"run_id":              firstString(job, "run_id", "job_run_id"),
		"created_at":          job["created_at"],
		"updated_at":          job["updated_at"],
		"started_at":          job["started_at"],
		"completed_at":        job["completed_at"],
		"output_path":         firstString(job, "output_path"),
		"drive_output_folder": firstString(job, "drive_output_folder"),
		"scene_count":         job["scene_count"],
		"voiceover_count":     job["voiceover_count"],
		"video_mode":          firstString(job, "video_mode"),
	}
	if errMsg := firstString(job, "error", "last_error", "error_message"); errMsg != "" {
		response["error"] = errMsg
	}
	if result := job["result"]; result != nil {
		response["result"] = result
	}
	if full {
		response["job"] = job
		response["request"] = job["request"]
	}
	return response
}

func firstString(source map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := source[key]; ok {
			switch vv := v.(type) {
			case string:
				if strings.TrimSpace(vv) != "" {
					return strings.TrimSpace(vv)
				}
			case fmt.Stringer:
				if strings.TrimSpace(vv.String()) != "" {
					return strings.TrimSpace(vv.String())
				}
			}
		}
	}
	return ""
}
