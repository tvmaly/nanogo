package helper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

var ErrUnavailable = errors.New("apple voice helper unavailable")

type Client struct {
	Path string
	Args []string
}

func ResolveBinary(envName, binaryName string) (string, error) {
	if p := os.Getenv(envName); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%w: %s=%s is not executable: %v", ErrUnavailable, envName, p, err)
		}
		return p, nil
	}
	if p, err := exec.LookPath(binaryName); err == nil {
		return p, nil
	}
	local := filepath.Join("ext", "voice", "providers", "apple", "helpers", "bin", binaryName)
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}
	return "", fmt.Errorf("%w: build %s with `make apple-voice-helpers` or set %s", ErrUnavailable, binaryName, envName)
}

func (c Client) Start(ctx context.Context) (*Process, error) {
	if c.Path == "" {
		return nil, fmt.Errorf("%w: helper path is empty", ErrUnavailable)
	}
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("apple voice helper stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("apple voice helper stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: start %s: %v", ErrUnavailable, c.Path, err)
	}
	p := &Process{cmd: cmd, stdin: stdin, events: make(chan Event, 16), done: make(chan error, 1)}
	go p.scan(stdout)
	return p, nil
}

type Process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	events chan Event
	done   chan error
	mu     sync.Mutex
	closed bool
}

func (p *Process) Events() <-chan Event { return p.events }

func (p *Process) Send(req Request) error {
	line, err := EncodeRequest(req)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("apple voice helper process closed")
	}
	if _, err := p.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write apple voice helper request: %w", err)
	}
	return nil
}

func (p *Process) Close(context.Context) error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		_ = p.stdin.Close()
	}
	p.mu.Unlock()
	err := <-p.done
	return err
}

func (p *Process) scan(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		event, err := DecodeEvent(sc.Bytes())
		if err != nil {
			p.events <- Event{Type: "error", Error: err.Error()}
			continue
		}
		p.events <- event
	}
	close(p.events)
	waitErr := p.cmd.Wait()
	if scanErr := sc.Err(); scanErr != nil {
		waitErr = errors.Join(waitErr, fmt.Errorf("scan apple voice helper stdout: %w", scanErr))
	}
	p.done <- waitErr
	close(p.done)
}
