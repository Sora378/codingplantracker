package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

type Window struct {
	UsedPercent        float64 `json:"usedPercent"`
	WindowDurationMins *int    `json:"windowDurationMins"`
	ResetsAt           *int64  `json:"resetsAt"`
}

type Snapshot struct {
	LimitID              *string `json:"limitId"`
	LimitName            *string `json:"limitName"`
	Primary              *Window `json:"primary"`
	Secondary            *Window `json:"secondary"`
	PlanType             *string `json:"planType"`
	RateLimitReachedType *string `json:"rateLimitReachedType"`
}

type RateLimits struct {
	RateLimits          Snapshot            `json:"rateLimits"`
	RateLimitsByLimitID map[string]Snapshot `json:"rateLimitsByLimitId"`
}

type Usage struct {
	PlanType  string
	Primary   *Window
	Secondary *Window
	Reached   string
}

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func ReadUsage(ctx context.Context) (*Usage, error) {
	return ReadUsageWithEnv(ctx, nil)
}

func ReadUsageWithEnv(ctx context.Context, env []string) (*Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	binary, err := BinaryPath()
	if err != nil {
		return nil, fmt.Errorf("codex executable not found; install Codex CLI or set CPQ_CODEX_BIN: %w", err)
	}

	cmd := exec.CommandContext(ctx, binary, "app-server", "--listen", "stdio://")
	if len(env) > 0 {
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	responses := make(chan rpcResponse, 4)
	readErr := make(chan error, 1)
	go readResponses(stdout, responses, readErr)
	go io.Copy(io.Discard, stderr)

	if _, err := fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"coplanage","title":"Coplanage","version":"0.1.0"},"capabilities":{"experimentalApi":true}}}`); err != nil {
		return nil, err
	}
	if _, err := waitForID(ctx, responses, readErr, 1); err != nil {
		return nil, err
	}

	if _, err := fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":2,"method":"account/rateLimits/read","params":null}`); err != nil {
		return nil, err
	}
	resp, err := waitForID(ctx, responses, readErr, 2)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, errors.New(resp.Error.Message)
	}

	var limits RateLimits
	if err := json.Unmarshal(resp.Result, &limits); err != nil {
		return nil, err
	}

	snapshot := limits.RateLimits
	if byID, ok := limits.RateLimitsByLimitID["codex"]; ok {
		snapshot = byID
	}
	if snapshot.Primary == nil && snapshot.Secondary == nil {
		return nil, errors.New("codex rate limit response had no windows")
	}

	usage := &Usage{
		Primary:   snapshot.Primary,
		Secondary: snapshot.Secondary,
	}
	if snapshot.PlanType != nil {
		usage.PlanType = *snapshot.PlanType
	}
	if snapshot.RateLimitReachedType != nil {
		usage.Reached = *snapshot.RateLimitReachedType
	}
	return usage, nil
}

func readResponses(stdout io.Reader, responses chan<- rpcResponse, readErr chan<- error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var resp rpcResponse
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil || resp.ID == 0 {
			continue
		}
		responses <- resp
	}
	if err := scanner.Err(); err != nil {
		readErr <- err
	}
}

func waitForID(ctx context.Context, responses <-chan rpcResponse, readErr <-chan error, id int) (rpcResponse, error) {
	for {
		select {
		case <-ctx.Done():
			return rpcResponse{}, ctx.Err()
		case err := <-readErr:
			return rpcResponse{}, err
		case resp := <-responses:
			if resp.ID == id {
				if resp.Error != nil {
					return resp, errors.New(resp.Error.Message)
				}
				return resp, nil
			}
		}
	}
}
