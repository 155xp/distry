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

var (
	workQueue  = make(chan Work, 10_000)
	inProgress = make(map[string]Work)
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
	output, err := runModule(module.Name(), nil, "split")
	var chunks [][]string
	if err != nil || json.Unmarshal([]byte(output), &chunks) != nil || len(chunks) > cap(workQueue) {
		http.Error(w, "split failed: "+output, 400)
		return
	}
	id := newID()
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
		work.LeaseID, work.LeaseUntil, work.Attempt = newID(), time.Now().Add(11*time.Minute), work.Attempt+1
		mu.Lock()
		inProgress[work.JobID+"/"+strconv.Itoa(work.ID)] = work
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
	key := result.JobID + "/" + strconv.Itoa(result.WorkID)
	work, ok := inProgress[key]
	if !ok || result.LeaseID != work.LeaseID || time.Now().After(work.LeaseUntil) {
		http.Error(w, "invalid or expired lease", http.StatusConflict)
		return
	}
	delete(inProgress, key)
	if result.Error != "" {
		workQueue <- work
		w.WriteHeader(http.StatusNoContent)
		return
	}
	job.Results[result.WorkID] = result.Output
	w.WriteHeader(http.StatusNoContent)
}

func retryAbandonedWork() {
	for range time.Tick(5 * time.Second) {
		mu.Lock()
		for key, work := range inProgress {
			if time.Now().After(work.LeaseUntil) {
				delete(inProgress, key)
				workQueue <- work
			}
		}
		mu.Unlock()
	}
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}
