package system

import (
	"os"
	"runtime"
	"time"
)

type Status struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Kernel   string `json:"kernel"`
	Uptime   string `json:"uptime"`
}
type Inspector struct{ startedAt time.Time }

func NewInspector(startedAt time.Time) Inspector { return Inspector{startedAt: startedAt} }
func (i Inspector) Status() Status {
	host, _ := os.Hostname()
	return Status{Hostname: host, OS: runtime.GOOS, Kernel: runtime.GOARCH, Uptime: time.Since(i.startedAt).Round(time.Second).String()}
}
func LinuxOnly() bool { return runtime.GOOS == "linux" }
