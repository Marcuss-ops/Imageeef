package api

import (
	"testing"

	"velox-worker-agent/pkg/config"
)

func TestEndpointAdapter_NewAPI(t *testing.T) {
	adapter := NewEndpointAdapter(config.APIModeNewAPI)

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"RegisterWorker", adapter.RegisterWorker(), "/api/workers/register"},
		{"UnregisterWorker", adapter.UnregisterWorker(), "/api/workers/unregister"},
		{"Heartbeat", adapter.Heartbeat(), "/api/workers/heartbeat"},
		{"GetJob", adapter.GetJob(), "/api/jobs/get"},
		{"GetCommands", adapter.GetCommands(), "/api/workers/commands"},
		{"SubmitResult", adapter.SubmitResult(), "/api/jobs/result"},
		{"HealthCheck", adapter.HealthCheck(), "/health"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}

	if adapter.Mode() != config.APIModeNewAPI {
		t.Errorf("Mode() = %q, want %q", adapter.Mode(), config.APIModeNewAPI)
	}
}

func TestEndpointAdapter_EmptyMode(t *testing.T) {
	// Empty mode should still resolve to canonical endpoints
	adapter := NewEndpointAdapter("")

	if adapter.Mode() != "" {
		t.Errorf("Mode() = %q, want empty", adapter.Mode())
	}

	if adapter.RegisterWorker() != "/api/workers/register" {
		t.Errorf("RegisterWorker() = %q, want %q", adapter.RegisterWorker(), "/api/workers/register")
	}
}

func TestEndpointAdapter_UnknownMode(t *testing.T) {
	// Unknown mode should still resolve to canonical endpoints
	adapter := NewEndpointAdapter("unknown_mode")

	if adapter.Mode() != "unknown_mode" {
		t.Errorf("Mode() = %q, want %q", adapter.Mode(), "unknown_mode")
	}

	// Should still return valid endpoints
	if adapter.RegisterWorker() != "/api/workers/register" {
		t.Errorf("RegisterWorker() = %q, want %q", adapter.RegisterWorker(), "/api/workers/register")
	}
}

func TestEndpointAdapter_Endpoints(t *testing.T) {
	adapter := NewEndpointAdapter(config.APIModeNewAPI)
	endpoints := adapter.Endpoints()

	if endpoints.RegisterWorker != "/api/workers/register" {
		t.Errorf("Endpoints().RegisterWorker = %q, want %q", endpoints.RegisterWorker, "/api/workers/register")
	}

	if endpoints.GetJob != "/api/jobs/get" {
		t.Errorf("Endpoints().GetJob = %q, want %q", endpoints.GetJob, "/api/jobs/get")
	}
}
