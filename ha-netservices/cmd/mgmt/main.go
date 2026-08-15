package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const appVersion = "0.1.0"

type commandSpec struct {
	Name string
	Args []string
}

type actionDefinition struct {
	Precheck *commandSpec
	Command  *commandSpec
	Signal   *signalSpec
}

type signalSpec struct {
	Mode        string
	PidFilePath string
	ProcessName string
	Signal      syscall.Signal
}

type serviceDefinition struct {
	Actions map[string]actionDefinition
}

type actionResponse struct {
	Success    bool   `json:"success"`
	Service    string `json:"service"`
	Action     string `json:"action"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration_ms"`
	Timestamp  string `json:"timestamp"`
}

type commandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

var services = map[string]serviceDefinition{
	"haproxy": {
		Actions: map[string]actionDefinition{
			"check": {
				Command: &commandSpec{
					Name: "/usr/sbin/haproxy",
					Args: []string{"-c", "-f", "/config/ha-netservices/haproxy/haproxy.cfg"},
				},
			},
			"reload": {
				Precheck: &commandSpec{
					Name: "/usr/sbin/haproxy",
					Args: []string{"-c", "-f", "/config/ha-netservices/haproxy/haproxy.cfg"},
				},
				Signal: &signalSpec{
					Mode:        "pidfile",
					PidFilePath: "/run/haproxy.pid",
					Signal:      syscall.SIGUSR2,
				},
			},
			"status": {
				Command: &commandSpec{
					Name: "/bin/cat",
					Args: []string{"/run/haproxy.pid"},
				},
			},
		},
	},
	"bind": {
		Actions: map[string]actionDefinition{
			"check": {
				Command: &commandSpec{
					Name: "/usr/bin/named-checkconf",
					Args: []string{"/config/ha-netservices/bind9/named.conf"},
				},
			},
			"reload": {
				Signal: &signalSpec{
					Mode:        "pidof",
					ProcessName: "named",
					Signal:      syscall.SIGHUP,
				},
			},
			"status": {
				Command: &commandSpec{
					Name: "/bin/pidof",
					Args: []string{"named"},
				},
			},
		},
	},
	"radiusd": {
		Actions: map[string]actionDefinition{
			"check": {
				Command: &commandSpec{
					Name: "/usr/sbin/radiusd",
					Args: []string{"-C", "-d", "/config/ha-netservices/radiusd"},
				},
			},
			"reload": {
				Signal: &signalSpec{
					Mode:        "pidof",
					ProcessName: "radiusd",
					Signal:      syscall.SIGTERM,
				},
			},
			"status": {
				Command: &commandSpec{
					Name: "/bin/pidof",
					Args: []string{"radiusd"},
				},
			},
		},
	},
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	showHelp := flag.Bool("help", false, "print usage and exit")
	listenAddr := flag.String("listen", envOrDefault("MGMT_ADDR", "127.0.0.1:8099"), "management listen address")
	flag.Parse()

	if *showHelp {
		flag.Usage()
		return
	}

	if *showVersion {
		fmt.Println(appVersion)
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/mgmt/", mgmtHandler)

	server := &http.Server{
		Addr:              *listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("mgmt API listening on %s", *listenAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("mgmt API failed: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func mgmtHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service, action, ok := parseMgmtPath(r.URL.Path)
	if !ok {
		http.Error(w, "invalid path; expected /mgmt/{service}/{check,reload,status}", http.StatusNotFound)
		return
	}

	serviceDef, found := services[service]
	if !found {
		http.Error(w, "unknown service", http.StatusNotFound)
		return
	}

	actionDef, found := serviceDef.Actions[action]
	if !found {
		http.Error(w, "unknown action", http.StatusNotFound)
		return
	}

	started := time.Now()
	resp := actionResponse{
		Service:   service,
		Action:    action,
		Timestamp: started.UTC().Format(time.RFC3339),
	}

	result, err := executeAction(r.Context(), actionDef)
	resp.DurationMS = time.Since(started).Milliseconds()

	if result != nil {
		resp.ExitCode = result.ExitCode
		resp.Stdout = result.Stdout
		resp.Stderr = result.Stderr
	}

	if err != nil {
		resp.Success = false
		if resp.ExitCode == 0 {
			resp.ExitCode = 1
		}
		if resp.Stderr == "" {
			resp.Stderr = err.Error()
		}
		writeJSON(w, http.StatusBadRequest, resp)
		return
	}

	resp.Success = true
	writeJSON(w, http.StatusOK, resp)
}

func parseMgmtPath(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "mgmt" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func executeAction(ctx context.Context, def actionDefinition) (*commandResult, error) {
	if def.Precheck != nil {
		preResult, err := runCommand(ctx, *def.Precheck)
		if err != nil {
			return preResult, fmt.Errorf("precheck failed: %w", err)
		}
	}

	if def.Command != nil {
		cmdResult, err := runCommand(ctx, *def.Command)
		if err != nil {
			return cmdResult, err
		}
		return cmdResult, nil
	}

	if def.Signal != nil {
		sigResult, err := runSignalAction(*def.Signal)
		if err != nil {
			return sigResult, err
		}
		return sigResult, nil
	}

	return nil, errors.New("action is not defined")
}

func runSignalAction(spec signalSpec) (*commandResult, error) {
	var pid int
	var err error

	switch spec.Mode {
	case "pidfile":
		pid, err = pidFromFile(spec.PidFilePath)
		if err != nil {
			return &commandResult{ExitCode: 1, Stderr: err.Error()}, err
		}
	case "pidof":
		pid, err = pidFromPidof(spec.ProcessName)
		if err != nil {
			return &commandResult{ExitCode: 1, Stderr: err.Error()}, err
		}
	default:
		err = fmt.Errorf("unsupported signal mode: %s", spec.Mode)
		return &commandResult{ExitCode: 1, Stderr: err.Error()}, err
	}

	if err := syscall.Kill(pid, spec.Signal); err != nil {
		msg := fmt.Sprintf("failed to signal pid %d: %v", pid, err)
		return &commandResult{ExitCode: 1, Stderr: msg}, err
	}

	if spec.Mode == "pidof" && spec.Signal == syscall.SIGTERM {
		restartedPID, restartErr := waitForDifferentPID(spec.ProcessName, pid, 5*time.Second)
		if restartErr != nil {
			msg := fmt.Sprintf("sent signal %d to pid %d but restart was not observed: %v", spec.Signal, pid, restartErr)
			return &commandResult{ExitCode: 1, Stdout: msg, Stderr: restartErr.Error()}, restartErr
		}

		stdout := fmt.Sprintf("sent signal %d to pid %d; restarted as pid %d", spec.Signal, pid, restartedPID)
		return &commandResult{ExitCode: 0, Stdout: stdout}, nil
	}

	stdout := fmt.Sprintf("sent signal %d to pid %d", spec.Signal, pid)
	return &commandResult{ExitCode: 0, Stdout: stdout}, nil
}

func waitForDifferentPID(processName string, oldPID int, timeout time.Duration) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("timeout waiting for %s to restart", processName)
		case <-ticker.C:
			pid, err := pidFromPidof(processName)
			if err == nil && pid != oldPID {
				return pid, nil
			}
		}
	}
}

func pidFromFile(path string) (int, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("unable to read pid file %s: %w", path, err)
	}

	pidStr := strings.TrimSpace(string(contents))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid pid in %s: %w", path, err)
	}
	return pid, nil
}

func pidFromPidof(processName string) (int, error) {
	cmd := exec.Command("/bin/pidof", processName)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("unable to find process %s: %w", processName, err)
	}

	pids := strings.Fields(strings.TrimSpace(string(output)))
	if len(pids) == 0 {
		return 0, fmt.Errorf("no pid found for process %s", processName)
	}

	pidStr := pids[0]
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid pid from pidof for %s: %w", processName, err)
	}
	return pid, nil
}

func runCommand(ctx context.Context, spec commandSpec) (*commandResult, error) {
	cmd := exec.CommandContext(ctx, spec.Name, spec.Args...)
	output, err := cmd.CombinedOutput()

	result := &commandResult{
		Stdout: strings.TrimSpace(string(output)),
	}

	if err == nil {
		result.ExitCode = 0
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else {
		result.ExitCode = 1
	}
	result.Stderr = err.Error()
	return result, err
}

func writeJSON(w http.ResponseWriter, status int, data actionResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
