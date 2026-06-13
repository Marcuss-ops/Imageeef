package jobs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveVideoPathPrefersAbsoluteAndRelativeCandidates(t *testing.T) {
	root := t.TempDir()
	videosDir := filepath.Join(root, "completed_videos")
	if err := os.MkdirAll(videosDir, 0o755); err != nil {
		t.Fatalf("mkdir videos dir: %v", err)
	}

	absPath := filepath.Join(videosDir, "absolute.mp4")
	if err := os.WriteFile(absPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write abs file: %v", err)
	}

	if got := resolveVideoPath(videosDir, "job-1", map[string]interface{}{
		"master_video_path": absPath,
	}); got != absPath {
		t.Fatalf("want absolute path %q, got %q", absPath, got)
	}

	relPath := filepath.Join("completed_videos", "relative.mp4")
	relAbs := filepath.Join(videosDir, "relative.mp4")
	if err := os.WriteFile(relAbs, []byte("x"), 0o644); err != nil {
		t.Fatalf("write rel file: %v", err)
	}

	if got := resolveVideoPath(videosDir, "job-2", map[string]interface{}{
		"master_video_path": relPath,
	}); got != relAbs {
		t.Fatalf("want relative path resolved to %q, got %q", relAbs, got)
	}
}
