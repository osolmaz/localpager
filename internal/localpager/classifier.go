package localpager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	jobCtx, cancel, err := classifierContext(ctx, job)
	if err != nil {
		return ClassifierOutput{}, "", "", "", err
	}
	defer cancel()
	args := classifierCommandArgs(target, opts)
	if opts.ClassifierSchema != "" {
		args = append(args, "--schema", opts.ClassifierSchema)
	}
	if opts.ClassifierPromptTemplate != "" {
		args = append(args, "--prompt-template", opts.ClassifierPromptTemplate)
	}
	if opts.ClassifierTopicTaxonomy != "" {
		args = append(args, "--topic-taxonomy", opts.ClassifierTopicTaxonomy)
	}
	contextPath, err := writeClassifierContextFile(jobCtx, job.Item, opts.ClassifierContext)
	if err != nil {
		return ClassifierOutput{}, "", "", "", err
	}
	if contextPath != "" {
		args = append(args, "--github-context-file", contextPath)
	}

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

func classifierCommandArgs(target string, opts WorkerOptions) []string {
	args := []string{target}
	args = appendStringFlag(args, "--model", opts.Model)
	args = appendStringFlag(args, "--base-url", opts.AgentBaseURL)
	args = appendIntFlag(args, "--context-window", opts.AgentContextWindow)
	args = appendIntFlag(args, "--max-tokens", opts.AgentMaxTokens)
	args = appendIntFlag(args, "--timeout-ms", opts.AgentTimeoutMS)
	return args
}

func appendStringFlag(args []string, flag string, value string) []string {
	if value == "" {
		return args
	}
	return append(args, flag, value)
}

func appendIntFlag(args []string, flag string, value int) []string {
	if value <= 0 {
		return args
	}
	return append(args, flag, fmt.Sprintf("%d", value))
}

func writeClassifierContextFile(ctx context.Context, item Item, opts ClassifierContextOptions) (string, error) {
	body := renderClassifierContext(ctx, item, opts)
	if strings.TrimSpace(body) == "" {
		return "", nil
	}
	dir, err := classifierContextDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create classifier context dir: %w", err)
	}
	file, err := os.CreateTemp(dir, "context-*.md")
	if err != nil {
		return "", fmt.Errorf("create classifier context file: %w", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(body); err != nil {
		return "", fmt.Errorf("write classifier context file: %w", err)
	}
	return file.Name(), nil
}

func classifierContextDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local/state/localpager/classifier/contexts"), nil
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
