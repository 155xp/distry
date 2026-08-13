package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, errors.New("output exceeds 1 MB")
	}
	return b.Buffer.Write(p)
}

type Work struct {
	JobID string   `json:"job_id"`
	ID    int      `json:"id"`
	Args  []string `json:"args"`
}

type Result struct {
	JobID  string `json:"job_id"`
	WorkID int    `json:"work_id"`
	Output string `json:"output"`
}

type Job struct {
	Module  []byte
	Total   int
	Results map[int]string
}

type JobStatus struct {
	ID        string   `json:"id"`
	Done      bool     `json:"done"`
	Completed int      `json:"completed"`
	Total     int      `json:"total"`
	Results   []string `json:"results,omitempty"`
}

func runModule(module string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var output cappedBuffer
	cmd := exec.CommandContext(ctx, "wasmtime", append([]string{"run", "-W", "max-memory-size=268435456", "-W", "timeout=600s", module}, args...)...)
	cmd.Env = append(os.Environ(), "HOME=/tmp")
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	return strings.TrimSpace(output.String()), err
}
