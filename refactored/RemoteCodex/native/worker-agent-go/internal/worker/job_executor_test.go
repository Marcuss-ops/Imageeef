package worker

import (
	"testing"

	"velox-worker-agent/pkg/api"
)

func TestShouldUploadCompletedVideoSkipsHealthCheck(t *testing.T) {
	job := &api.Job{JobType: "health_check"}
	if shouldUploadCompletedVideo(job, map[string]interface{}{"status": "healthy"}) {
		t.Fatal("expected health_check jobs to skip upload")
	}
}

func TestShouldUploadCompletedVideoRequiresPathForVideoJobs(t *testing.T) {
	job := &api.Job{JobType: "process_video"}
	if shouldUploadCompletedVideo(job, map[string]interface{}{"status": "completed"}) {
		t.Fatal("expected video jobs without output path to skip upload")
	}

	if !shouldUploadCompletedVideo(job, map[string]interface{}{"output_path": "/tmp/out.mp4"}) {
		t.Fatal("expected video jobs with output path to upload")
	}
}

func TestShouldUploadCompletedVideoSkipsNilJob(t *testing.T) {
	if shouldUploadCompletedVideo(nil, nil) {
		t.Fatal("expected nil job to skip upload")
	}
}

func TestShouldUploadCompletedVideoSkipsRenderJobsWithoutPath(t *testing.T) {
	job := &api.Job{JobType: "render"}
	if shouldUploadCompletedVideo(job, map[string]interface{}{"status": "completed"}) {
		t.Fatal("expected render jobs without output path to skip upload")
	}
}

func TestShouldUploadCompletedVideoSkipsAudioJobs(t *testing.T) {
	job := &api.Job{JobType: "process_audio"}
	if shouldUploadCompletedVideo(job, map[string]interface{}{"status": "completed"}) {
		t.Fatal("expected audio jobs without output path to skip upload")
	}

	if !shouldUploadCompletedVideo(job, map[string]interface{}{"output_path": "/tmp/out.mp3"}) {
		t.Fatal("expected audio jobs with output path to upload")
	}
}

func TestHealthCheckJobTypeConstant(t *testing.T) {
	job := &api.Job{JobType: "health_check"}
	if job.JobType != "health_check" {
		t.Fatalf("expected job type health_check, got %s", job.JobType)
	}
}
