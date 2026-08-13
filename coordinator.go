package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Assignment struct {
	Work      Work
	StartedAt time.Time
}

var (
	workQueue  = make(chan Work, 10_000)
	inProgress = make(map[string]Assignment)
	jobs       = make(map[string]*Job)
	mu         sync.Mutex
)

func startCoordinator() {
	if os.Getenv("SUBMIT_KEY") == "" {
		fmt.Println("SUBMIT_KEY is required")
		return
	}
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	http.HandleFunc("/jobs", createJob)
	http.HandleFunc("/jobs/", getJob)
	http.HandleFunc("/work", giveWork)
	http.HandleFunc("/script/", giveScript)
	http.HandleFunc("/result", receiveResult)
	go retryAbandonedWork()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("coordinator running on :" + port)
	fmt.Println(http.ListenAndServe(":"+port, nil))
}

func createJob(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(key), []byte(os.Getenv("SUBMIT_KEY"))) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	module, err := os.CreateTemp("", "distry-*.wasm")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer os.Remove(module.Name())
	defer module.Close()
	if _, err = module.ReadFrom(http.MaxBytesReader(w, r.Body, 10<<20)); err != nil {
		http.Error(w, "invalid module", 400)
		return
	}
	data, _ := os.ReadFile(module.Name())
	if len(data) < 4 || string(data[:4]) != "\x00asm" {
		http.Error(w, "not a wasm module", 400)
		return
	}
	output, err := runModule(module.Name(), "split")
	var chunks [][]string
	if err != nil || json.Unmarshal([]byte(output), &chunks) != nil || len(chunks) > cap(workQueue) {
		http.Error(w, "split failed: "+output, 400)
		return
	}
	idBytes := make([]byte, 8)
	rand.Read(idBytes)
	id := hex.EncodeToString(idBytes)
	mu.Lock()
	jobs[id] = &Job{Module: data, Total: len(chunks), Results: make(map[int]string)}
	mu.Unlock()
	go func() {
		for i, args := range chunks {
			workQueue <- Work{JobID: id, ID: i, Args: args}
		}
	}()
	writeJSON(w, JobStatus{ID: id, Total: len(chunks)})
}

func getJob(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	id := strings.TrimPrefix(r.URL.Path, "/jobs/")
	job := jobs[id]
	if job == nil {
		http.NotFound(w, r)
		return
	}
	status := JobStatus{ID: id, Completed: len(job.Results), Total: job.Total, Done: len(job.Results) == job.Total}
	if status.Done {
		status.Results = make([]string, job.Total)
		for i := range status.Results {
			status.Results[i] = job.Results[i]
		}
	}
	writeJSON(w, status)
}

func giveWork(w http.ResponseWriter, _ *http.Request) {
	select {
	case work := <-workQueue:
		mu.Lock()
		inProgress[work.JobID+"/"+strconv.Itoa(work.ID)] = Assignment{work, time.Now()}
		mu.Unlock()
		writeJSON(w, work)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func giveScript(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	job := jobs[strings.TrimPrefix(r.URL.Path, "/script/")]
	mu.Unlock()
	if job == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/wasm")
	w.Write(job.Module)
}

func receiveResult(w http.ResponseWriter, r *http.Request) {
	var result Result
	if json.NewDecoder(r.Body).Decode(&result) != nil {
		http.Error(w, "invalid result", 400)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	job := jobs[result.JobID]
	if job == nil || result.WorkID < 0 || result.WorkID >= job.Total {
		http.Error(w, "unknown work", 404)
		return
	}
	delete(inProgress, result.JobID+"/"+strconv.Itoa(result.WorkID))
	job.Results[result.WorkID] = result.Output
}

func retryAbandonedWork() {
	for range time.Tick(5 * time.Second) {
		mu.Lock()
		for key, assignment := range inProgress {
			if time.Since(assignment.StartedAt) > 10*time.Minute {
				delete(inProgress, key)
				workQueue <- assignment.Work
			}
		}
		mu.Unlock()
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}
