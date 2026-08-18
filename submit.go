package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func submit(server, source string) error {
	key := os.Getenv("SUBMIT_KEY")
	if key == "" {
		return fmt.Errorf("SUBMIT_KEY is required")
	}
	file, err := os.CreateTemp("", "distry-*.wasm")
	if err != nil {
		return err
	}
	file.Close()
	defer os.Remove(file.Name())
	cmd := exec.Command("go", "build", "-o", file.Name(), source)
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build failed: %s", output)
	}
	module, _ := os.ReadFile(file.Name())
	req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(server, "/")+"/jobs", bytes.NewReader(module))
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coordinator returned %s: %s", resp.Status, message)
	}
	var status JobStatus
	json.NewDecoder(resp.Body).Decode(&status)
	fmt.Println("job:", status.ID)
	for !status.Done {
		time.Sleep(2 * time.Second)
		resp, err = http.Get(strings.TrimRight(server, "/") + "/jobs/" + status.ID)
		if err != nil {
			return err
		}
		err = json.NewDecoder(resp.Body).Decode(&status)
		resp.Body.Close()
		if err != nil {
			return err
		}
		fmt.Printf("progress: %d/%d\r", status.Completed, status.Total)
	}
	fmt.Println()
	results, _ := json.Marshal(status.Results)
	output, err := runModule(file.Name(), bytes.NewReader(results), "merge")
	if err != nil {
		return fmt.Errorf("merge failed: %s", output)
	}
	fmt.Println(output)
	return nil
}
