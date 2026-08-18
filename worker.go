package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func startWorker(server string) {
	server = strings.TrimRight(server, "/")
	modules := make(map[string]string)
	defer func() {
		for _, path := range modules {
			os.Remove(path)
		}
	}()

	for {
		resp, err := http.Get(server + "/work")
		if err != nil {
			fmt.Println("coordinator unavailable:", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if resp.StatusCode == http.StatusNoContent {
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			continue
		}
		var work Work
		err = json.NewDecoder(resp.Body).Decode(&work)
		resp.Body.Close()
		if err != nil {
			fmt.Println("invalid work:", err)
			continue
		}

		module := modules[work.JobID]
		if module == "" {
			module, err = downloadModule(server, work.JobID)
			if err == nil {
				modules[work.JobID] = module
			}
		}
		fmt.Printf("job %s chunk %d\n", work.JobID, work.ID)
		var output string
		if err == nil {
			output, err = runModule(module, nil, append([]string{"run"}, work.Args...)...)
		}
		result := Result{JobID: work.JobID, WorkID: work.ID, LeaseID: work.LeaseID, Output: output}
		if err != nil {
			fmt.Println("job failed:", err)
			result.Error = err.Error()
		}
		body, _ := json.Marshal(result)
		resp, err = http.Post(server+"/result", "application/json", bytes.NewReader(body))
		if err == nil {
			resp.Body.Close()
		}
	}
}

func downloadModule(server, id string) (string, error) {
	resp, err := http.Get(server + "/script/" + id)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %s", resp.Status)
	}
	file, err := os.CreateTemp("", "distry-*.wasm")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(file, io.LimitReader(resp.Body, 10<<20))
	file.Close()
	return file.Name(), err
}
