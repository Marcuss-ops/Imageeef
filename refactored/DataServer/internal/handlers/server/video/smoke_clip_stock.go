package video

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"velox-server/internal/config"
	"velox-server/internal/queue"
)

// CreateSmokeClipStock accepts POST /api/v1/video/smoke-clip-stock and enqueues
// a minimal process_video job for the clip+stock pipeline.
func CreateSmokeClipStock(cfg *config.Config, q *queue.FileQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		if q == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "queue unavailable"})
			return
		}

		var body map[string]interface{}
		if err := c.ShouldBindJSON(&body); err != nil && err.Error() != "EOF" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid JSON"})
			return
		}
		if body == nil {
			body = map[string]interface{}{}
		}

		videoName := firstString(body, "video_name", "title", "project_name")
		if videoName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing video_name"})
			return
		}
		scriptText := firstString(body, "script_text", "script")
		if scriptText == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing script_text"})
			return
		}
		videoMode := firstString(body, "video_mode")
		if videoMode == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing video_mode"})
			return
		}

		voiceoverPaths := normalizePaths(body, "voiceover_paths")
		if len(voiceoverPaths) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing voiceover_paths"})
			return
		}
		introClipPaths := normalizePaths(body, "intro_clip_paths")
		if len(introClipPaths) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing intro_clip_paths"})
			return
		}
		stockClipPaths := normalizePaths(body, "stock_clip_paths")
		if len(stockClipPaths) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing stock_clip_paths"})
			return
		}
		clipSegments := body["clip_segments"]

		jobID := firstString(body, "job_id", "id")
		if jobID == "" {
			jobID = "smoke_clip_stock_" + uuid.NewString()
		}
		jobRunID := firstString(body, "job_run_id", "run_id")
		if jobRunID == "" {
			jobRunID = "run_" + uuid.NewString()
		}
		correlationID := firstString(body, "correlation_id")
		if correlationID == "" {
			correlationID = "corr_" + uuid.NewString()
		}
		now := time.Now().UTC().Format(time.RFC3339)
		outputPath := firstString(body, "output_path", "output")
		if outputPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing output_path"})
			return
		}
		driveOutputFolder := firstString(body, "drive_output_folder", "output_directory")

		normalized := map[string]interface{}{
			"job_id":                 jobID,
			"id":                     jobID,
			"job_run_id":             jobRunID,
			"run_id":                 jobRunID,
			"correlation_id":         correlationID,
			"job_type":               "process_video",
			"version":       "v1",
			"created_at":    ensureRFC3339(firstString(body, "created_at"), now),
			"updated_at":             ensureRFC3339(firstString(body, "updated_at"), now),
			"video_name":             videoName,
			"title":                  videoName,
			"script_text":            scriptText,
			"video_mode":             videoMode,
			"voiceover_paths":        voiceoverPaths,
			"voiceover_path":         voiceoverPaths[0],
			"audio_path":             voiceoverPaths[0],
			"intro_clip_paths":       introClipPaths,
			"stock_clip_paths":       stockClipPaths,
			"clip_segments":          clipSegments,
			"scenes":                 []interface{}{},
			"scenes_json":            "[]",
			"output_path":            outputPath,
			"drive_output_folder":    driveOutputFolder,
			"audio_language_for_srt": firstString(body, "audio_language_for_srt", "audio_lang"),
			"priority":               ensureInt(body["priority"], 1),
			"timeout_secs":           ensureInt(body["timeout_secs"], 3600),
			"submitted_via":          "api_v1_smoke_clip_stock",
			"source":                 "smoke_clip_stock_api",
			"scene_count":            0,
			"voiceover_count":        len(voiceoverPaths),
		}

		normalized["parameters"] = map[string]interface{}{
			"version": "v1",
			"job_id":                 jobID,
			"job_run_id":             jobRunID,
			"run_id":                 jobRunID,
			"correlation_id":         correlationID,
			"job_type":               "process_video",
			"video_name":             videoName,
			"script_text":            scriptText,
			"video_mode":             videoMode,
			"voiceover_paths":        voiceoverPaths,
			"voiceover_path":         voiceoverPaths[0],
			"audio_path":             voiceoverPaths[0],
			"intro_clip_paths":       introClipPaths,
			"stock_clip_paths":       stockClipPaths,
			"clip_segments":          clipSegments,
			"output_path":            outputPath,
			"drive_output_folder":    driveOutputFolder,
			"audio_language_for_srt": firstString(body, "audio_language_for_srt", "audio_lang"),
			"priority":               ensureInt(body["priority"], 1),
			"timeout_secs":           ensureInt(body["timeout_secs"], 3600),
			"submitted_via":          "api_v1_smoke_clip_stock",
			"source":                 "smoke_clip_stock_api",
		}

		if err := q.SubmitJob(c.Request.Context(), jobID, normalized); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"ok":                  true,
			"job_id":              jobID,
			"job_run_id":          jobRunID,
			"correlation_id":      correlationID,
			"job_type":            "process_video",
			"video_mode":          videoMode,
			"output_path":         outputPath,
			"drive_output_folder": driveOutputFolder,
			"voiceover_paths":     voiceoverPaths,
			"intro_clip_paths":    introClipPaths,
			"stock_clip_paths":    stockClipPaths,
			"clip_segments":       clipSegments,
			"status":              "PENDING",
			"queue":               "queued_for_workers",
		})
	}
}

func normalizePaths(body map[string]interface{}, key string) []string {
	val := body[key]
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if strings.TrimSpace(item) != "" {
				out = append(out, strings.TrimSpace(item))
			}
		}
		return out
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		return []string{s}
	default:
		return nil
	}
}
