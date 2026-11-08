// SPDX-License-Identifier: AGPL-3.0-only
package diagnostics

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultProcesses    = 25
	MaxProcesses        = 100
	maxScannedProcesses = 4096
	maxProcFileBytes    = 64 << 10
)

var ErrProcessScanBusy = errors.New("a process scan is already running")

type Process struct {
	PID           int     `json:"pid"`
	Command       string  `json:"command"`
	CPUPercent    float64 `json:"cpuPct"`
	RSSBytes      int64   `json:"rssBytes"`
	User          string  `json:"user"`
	UID           uint64  `json:"uid"`
	State         string  `json:"state"`
	UptimeSeconds float64 `json:"uptimeSeconds"`
	ContainerID   string  `json:"containerId,omitempty"`
	// Live-view detail, all from the same /proc/<pid>/stat read.
	Threads        int64    `json:"threads"`
	Nice           int64    `json:"nice"`
	VirtBytes      int64    `json:"virtBytes"`
	CPUTimeSeconds float64  `json:"cpuTimeSeconds"`
	MemPercent     *float64 `json:"memPct,omitempty"`
}

// TaskCounts summarises every process seen by a scan, not only the returned
// rows: R is running; S and I (idle kernel threads) are sleeping; D, Z, T and
// the rest are other.
type TaskCounts struct {
	Total    int   `json:"total"`
	Threads  int64 `json:"threads"`
	Running  int   `json:"running"`
	Sleeping int   `json:"sleeping"`
	Other    int   `json:"other"`
}

// ProcessScan is one finished scan: the busiest processes and the task
// counts, as of SampledAt.
type ProcessScan struct {
	Processes []Process  `json:"processes"`
	Tasks     TaskCounts `json:"tasks"`
	SampledAt time.Time  `json:"sampledAt"`
}

type ProcessScanner struct {
	ProcRoot, PasswdPath string
	SampleInterval       time.Duration
	// Redact runs over every returned command line, normally the log
	// redactor, so secrets passed as arguments never reach the UI.
	Redact func(string) string
	// ReuseFor is how long a finished scan answers later requests: a view
	// that refreshes every few seconds, open in several tabs, still costs at
	// most one /proc walk per ReuseFor (ADR 017). Zero disables reuse.
	ReuseFor time.Duration
	sem      chan struct{}
	now      func() time.Time
	mu       sync.Mutex
	last     ProcessScan
}

func NewProcessScanner(procRoot, passwdPath string) *ProcessScanner {
	return &ProcessScanner{ProcRoot: procRoot, PasswdPath: passwdPath, SampleInterval: 200 * time.Millisecond, ReuseFor: 4 * time.Second, sem: make(chan struct{}, 1)}
}

func (s *ProcessScanner) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

type processSample struct {
	PID                         int
	Command, State, ContainerID string
	Ticks                       uint64
	StartTicks                  uint64
	RSSBytes                    int64
	UID                         uint64
	Threads, Nice, VirtBytes    int64
}

// Scan returns the busiest processes; see ScanHost.
func (s *ProcessScanner) Scan(ctx context.Context, limit int) ([]Process, error) {
	result, err := s.ScanHost(ctx, limit)
	return result.Processes, err
}

// ScanHost samples /proc twice and returns up to limit processes, busiest
// first, with counts over every process seen. A scan finished less than
// ReuseFor ago is returned again instead of walking /proc once more.
func (s *ProcessScanner) ScanHost(ctx context.Context, limit int) (ProcessScan, error) {
	if limit == 0 {
		limit = DefaultProcesses
	}
	if limit < 1 || limit > MaxProcesses {
		return ProcessScan{}, fmt.Errorf("process limit must be between 1 and %d", MaxProcesses)
	}
	if recent, ok := s.recent(limit); ok {
		return recent, nil
	}
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		return ProcessScan{}, ErrProcessScanBusy
	}
	// A scan that finished while this one waited for the semaphore serves it.
	if recent, ok := s.recent(limit); ok {
		return recent, nil
	}
	result, err := s.scan(ctx)
	if err != nil {
		return ProcessScan{}, err
	}
	if s.ReuseFor > 0 {
		s.mu.Lock()
		s.last = result
		s.mu.Unlock()
	}
	return truncateScan(result, limit), nil
}

// recent returns the last scan when it is younger than ReuseFor.
func (s *ProcessScanner) recent(limit int) (ProcessScan, bool) {
	if s.ReuseFor <= 0 {
		return ProcessScan{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last.SampledAt.IsZero() || s.clock().Sub(s.last.SampledAt) >= s.ReuseFor {
		return ProcessScan{}, false
	}
	return truncateScan(s.last, limit), true
}

func truncateScan(result ProcessScan, limit int) ProcessScan {
	if len(result.Processes) > limit {
		result.Processes = result.Processes[:limit:limit]
	}
	return result
}

// scan does the two-sample walk and keeps the MaxProcesses busiest, so one
// result can answer any limit.
func (s *ProcessScanner) scan(ctx context.Context) (ProcessScan, error) {
	firstTotal, first, err := s.sample(ctx)
	if err != nil {
		return ProcessScan{}, err
	}
	timer := time.NewTimer(s.SampleInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ProcessScan{}, ctx.Err()
	case <-timer.C:
	}
	secondTotal, second, err := s.sample(ctx)
	if err != nil {
		return ProcessScan{}, err
	}
	deltaTotal := secondTotal - firstTotal
	if secondTotal < firstTotal || deltaTotal == 0 {
		deltaTotal = 1
	}
	users := readPasswd(s.PasswdPath)
	uptime := readUptime(s.ProcRoot)
	memTotal := readMemTotal(s.ProcRoot)
	var tasks TaskCounts
	for _, current := range second {
		tasks.Total++
		tasks.Threads += current.Threads
		switch current.State {
		case "R":
			tasks.Running++
		case "S", "I":
			tasks.Sleeping++
		default:
			tasks.Other++
		}
	}
	values := make([]Process, 0, len(second))
	for pid, current := range second {
		previous, ok := first[pid]
		if !ok || current.StartTicks != previous.StartTicks {
			continue
		}
		delta := current.Ticks - previous.Ticks
		if current.Ticks < previous.Ticks {
			delta = 0
		}
		user := users[current.UID]
		if user == "" {
			user = strconv.FormatUint(current.UID, 10)
		}
		age := uptime - float64(current.StartTicks)/100
		if age < 0 {
			age = 0
		}
		value := Process{PID: pid, Command: current.Command, CPUPercent: float64(delta) * 100 / float64(deltaTotal), RSSBytes: current.RSSBytes, User: user, UID: current.UID, State: current.State, UptimeSeconds: age, ContainerID: current.ContainerID,
			Threads: current.Threads, Nice: current.Nice, VirtBytes: current.VirtBytes, CPUTimeSeconds: float64(current.Ticks) / 100}
		if memTotal > 0 {
			pct := float64(current.RSSBytes) * 100 / float64(memTotal)
			value.MemPercent = &pct
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].CPUPercent == values[j].CPUPercent {
			return values[i].RSSBytes > values[j].RSSBytes
		}
		return values[i].CPUPercent > values[j].CPUPercent
	})
	if len(values) > MaxProcesses {
		values = values[:MaxProcesses]
	}
	for i := range values {
		values[i].Command = s.redactCommand(values[i].Command)
	}
	return ProcessScan{Processes: values, Tasks: tasks, SampledAt: s.clock()}, nil
}

// inlineSecretFlags lists clients that take a secret glued to a short flag,
// such as "mysql -pSECRET" or "redis-cli -aSECRET", keyed by the executable
// name prefix. Long "--password=" forms are already covered by the redactor.
var inlineSecretFlags = []struct{ program, flag string }{{"mysql", "-p"}, {"mariadb", "-p"}, {"redis-cli", "-a"}}

func (s *ProcessScanner) redactCommand(command string) string {
	fields := strings.Fields(command)
	if len(fields) > 1 {
		program := fields[0]
		if i := strings.LastIndexByte(program, '/'); i >= 0 {
			program = program[i+1:]
		}
		for _, rule := range inlineSecretFlags {
			if !strings.HasPrefix(program, rule.program) {
				continue
			}
			for i, field := range fields[1:] {
				if strings.HasPrefix(field, rule.flag) && len(field) > len(rule.flag) {
					fields[i+1] = rule.flag + "[REDACTED]"
				}
			}
			command = strings.Join(fields, " ")
		}
	}
	if s.Redact != nil {
		command = s.Redact(command)
	}
	return command
}

func (s *ProcessScanner) sample(ctx context.Context) (uint64, map[int]processSample, error) {
	total, err := readCPUTotal(filepath.Join(s.ProcRoot, "stat"))
	if err != nil {
		return 0, nil, err
	}
	entries, err := os.ReadDir(s.ProcRoot)
	if err != nil {
		return 0, nil, err
	}
	values := map[int]processSample{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}
		if len(values) >= maxScannedProcesses {
			break
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 || !entry.IsDir() {
			continue
		}
		value, err := readProcess(s.ProcRoot, pid)
		if err == nil {
			values[pid] = value
		}
	}
	return total, values, nil
}

func readProcess(root string, pid int) (processSample, error) {
	dir := filepath.Join(root, strconv.Itoa(pid))
	stat, err := readBounded(filepath.Join(dir, "stat"))
	if err != nil {
		return processSample{}, err
	}
	open, close := strings.IndexByte(stat, '('), strings.LastIndex(stat, ") ")
	if open < 0 || close < open {
		return processSample{}, errors.New("malformed process stat")
	}
	fields := strings.Fields(stat[close+2:])
	if len(fields) < 22 {
		return processSample{}, errors.New("short process stat")
	}
	utime, e1 := strconv.ParseUint(fields[11], 10, 64)
	stime, e2 := strconv.ParseUint(fields[12], 10, 64)
	start, e3 := strconv.ParseUint(fields[19], 10, 64)
	rss, e4 := strconv.ParseInt(fields[21], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return processSample{}, errors.New("invalid process counters")
	}
	nice := displayInt(fields[16])
	threads := displayInt(fields[17])
	virt := displayInt(fields[20])
	command := strings.TrimSpace(stat[open+1 : close])
	if raw, err := readBounded(filepath.Join(dir, "cmdline")); err == nil {
		if value := strings.TrimSpace(strings.ReplaceAll(raw, "\x00", " ")); value != "" {
			command = value
		}
	}
	uid := uint64(0)
	if status, err := readBounded(filepath.Join(dir, "status")); err == nil {
		for _, line := range strings.Split(status, "\n") {
			if strings.HasPrefix(line, "Uid:") {
				parts := strings.Fields(line)
				if len(parts) > 1 {
					// A uid that does not parse would otherwise read as root.
					parsed, err := strconv.ParseUint(parts[1], 10, 64)
					if err != nil {
						return processSample{}, fmt.Errorf("invalid process uid: %w", err)
					}
					uid = parsed
				}
				break
			}
		}
	}
	container := ""
	if cgroup, err := readBounded(filepath.Join(dir, "cgroup")); err == nil {
		container = containerFromCgroup(cgroup)
	}
	return processSample{PID: pid, Command: command, State: fields[0], Ticks: utime + stime, StartTicks: start, RSSBytes: max(0, rss) * int64(os.Getpagesize()), UID: uid, ContainerID: container,
		Threads: max(0, threads), Nice: nice, VirtBytes: max(0, virt)}, nil
}

func readCPUTotal(path string) (uint64, error) {
	raw, err := readBounded(path)
	if err != nil {
		return 0, err
	}
	line, _, _ := strings.Cut(raw, "\n")
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "cpu" {
		return 0, errors.New("malformed host cpu stat")
	}
	var total uint64
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, err
		}
		total += value
	}
	return total, nil
}

// displayInt parses a display-only stat field; one that does not parse reads
// as zero rather than dropping the whole process from the scan.
func displayInt(field string) int64 {
	value, err := strconv.ParseInt(field, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// readMemTotal returns the host's MemTotal in bytes, or 0 when unreadable.
func readMemTotal(root string) int64 {
	raw, err := readBounded(filepath.Join(root, "meminfo"))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return 0
			}
			return kb * 1024
		}
	}
	return 0
}
func readUptime(root string) float64 {
	raw, err := readBounded(filepath.Join(root, "uptime"))
	if err != nil {
		return 0
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		// Same as an unreadable uptime: process ages are not computed.
		return 0
	}
	return value
}
func readBounded(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), maxProcFileBytes)
	var b strings.Builder
	for scanner.Scan() {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(scanner.Text())
	}
	return b.String(), scanner.Err()
}
func readPasswd(path string) map[uint64]string {
	users := map[uint64]string{}
	raw, err := readBounded(path)
	if err != nil {
		return users
	}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 3 {
			continue
		}
		uid, err := strconv.ParseUint(fields[2], 10, 64)
		if err == nil {
			users[uid] = fields[0]
		}
	}
	return users
}
func containerFromCgroup(raw string) string {
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool { return r == '/' || r == ':' || r == '\n' || r == '-' }) {
		field = strings.TrimSuffix(field, ".scope")
		if len(field) >= 64 {
			candidate := field[len(field)-64:]
			if allHex(candidate) {
				return candidate
			}
		}
	}
	return ""
}
func allHex(value string) bool {
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}
