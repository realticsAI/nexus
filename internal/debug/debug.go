package debug

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

var enabled bool

func init() {
	v := os.Getenv("NEXUS_DEBUG")
	enabled = v == "1" || strings.EqualFold(v, "true")
}

func Enabled() bool { return enabled }

func Log(component, format string, args ...interface{}) {
	if !enabled {
		return
	}
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[DEBUG:%s] %s\n", component, msg)
}

func MemAlloc() uint64 {
	if !enabled {
		return 0
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func FormatBytes(b uint64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func FileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

func Since(t time.Time) string {
	return time.Since(t).Round(time.Millisecond).String()
}
