package jobs

import (
	"context"
	"fmt"
	"strings"

	"velox-server/internal/workers"
)

// checkWorkerCompatibility validates that a worker is compatible with the master
// before assigning a job. Returns empty string if compatible, otherwise a rejection reason.
func (s *Service) checkWorkerCompatibility(ctx context.Context, worker *workers.WorkerInfo, jobType string) string {
	if worker == nil {
		return "worker not registered"
	}

	// Protocol version check
	protocolVersion := strings.TrimSpace(worker.ProtocolVersion)
	if protocolVersion == "" {
		return "worker missing protocol_version"
	}
	if protocolVersion != workers.DefaultWorkerProtocolVersion {
		return fmt.Sprintf("protocol_version mismatch: worker=%s master=%s", protocolVersion, workers.DefaultWorkerProtocolVersion)
	}

	// Capabilities check
	if len(worker.Capabilities) == 0 {
		return "worker missing capabilities"
	}

	// Supported job types check (only when a specific job type is requested)
	if jobType != "" {
		supportedTypes := worker.GetSupportedJobTypes()
		if len(supportedTypes) > 0 {
			found := false
			for _, t := range supportedTypes {
				if strings.EqualFold(strings.TrimSpace(t), jobType) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Sprintf("job_type %q not supported by worker (supported: %v)", jobType, supportedTypes)
			}
		}
	}

	return ""
}
