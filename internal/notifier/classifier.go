package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
)

func runClassifier(ctx context.Context, job ClaimedJob, opts WorkerOptions) (ClassifierOutput, string, string, string, error) {
	commandPath, err := ExpandPath(opts.ClassifierCommand)
	if err != nil {
		return ClassifierOutput{}, "", "", "", err
	}
	target := classifierTarget(job)
	args := []string{target}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	jobCtx, cancel, err := classifierContext(ctx, job)
	if err != nil {
		return ClassifierOutput{}, "", "", "", err
	}
	defer cancel()

	stdout, stderr, err := runClassifierCommand(jobCtx, commandPath, args)
	if err != nil {
		if errors.Is(jobCtx.Err(), context.DeadlineExceeded) {
			return ClassifierOutput{}, "", "", "", fmt.Errorf("classifier timed out before lease expiry")
		}
		return ClassifierOutput{}, "", "", "", fmt.Errorf("classifier failed: %w stderr=%s", err, strings.TrimSpace(stderr))
	}
	outputJSON := strings.TrimSpace(stdout)
	var output ClassifierOutput
	if err := json.Unmarshal([]byte(outputJSON), &output); err != nil {
		return ClassifierOutput{}, "", "", "", fmt.Errorf("classifier returned invalid JSON: %w stdout=%s stderr=%s", err, outputJSON, strings.TrimSpace(stderr))
	}
	return output, outputJSON, parseStderrPath(stderr, "prompt"), parseStderrPath(stderr, "session"), nil
}

func classifierTarget(job ClaimedJob) string {
	target := deref(job.Item.SourceURL)
	if target != "" {
		return target
	}
	target = deref(job.Item.Ref)
	if target != "" {
		return target
	}
	return job.Item.SourceRef
}

func classifierContext(ctx context.Context, job ClaimedJob) (context.Context, context.CancelFunc, error) {
	if job.LeasedUntil == nil {
		return ctx, func() {}, nil
	}
	timeout := time.Until(*job.LeasedUntil)
	if timeout <= 0 {
		return nil, nil, fmt.Errorf("classifier lease already expired")
	}
	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	return jobCtx, cancel, nil
}

func runClassifierCommand(ctx context.Context, commandPath string, args []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, commandPath, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func parseStderrPath(stderr string, label string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(label) + `:\s*(.+)$`)
	match := re.FindStringSubmatch(stderr)
	if len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}
