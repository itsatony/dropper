package dropper

import (
	"log/slog"
	"net/http"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// DiskUsageInfo holds filesystem usage statistics.
type DiskUsageInfo struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

// HealthResponse is the /healthz response body.
type HealthResponse struct {
	Status string         `json:"status"`
	Disk   *DiskUsageInfo `json:"disk,omitempty"`
}

// HandleHealthz returns a handler for the /healthz endpoint.
// Reports disk usage of the configured root directory.
func HandleHealthz(rootDir string, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		disk, err := getDiskUsage(rootDir)
		if err != nil {
			logger.Warn(ErrMsgDiskUsage, LogFieldError, err, LogFieldRootDir, rootDir)
			RespondOK(w, &HealthResponse{Status: HealthStatusOK})
			return
		}
		RespondOK(w, &HealthResponse{
			Status: HealthStatusOK,
			Disk:   disk,
		})
	}
}

// getDiskUsage returns disk usage statistics for the given path.
// Uses syscall.Statfs (Linux/macOS). Uses Frsize for correct block size
// and Bavail for space available to non-root users.
func getDiskUsage(path string) (*DiskUsageInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}

	total := stat.Blocks * uint64(stat.Frsize)
	free := stat.Bfree * uint64(stat.Frsize)
	available := stat.Bavail * uint64(stat.Frsize)
	used := total - free

	var percent float64
	if total > 0 {
		percent = float64(used) / float64(total) * DiskPercent100
	}

	return &DiskUsageInfo{
		TotalBytes:     total,
		UsedBytes:      used,
		AvailableBytes: available,
		UsedPercent:    percent,
	}, nil
}

// MetricsHandler returns the Prometheus metrics handler.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
