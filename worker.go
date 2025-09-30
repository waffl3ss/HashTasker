package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"hashtasker-go/internal/models"
)

type WorkerConfig struct {
	ServerURL       string `json:"server_url"`
	Hostname        string `json:"hostname"`
	CheckinInterval string `json:"checkin_interval"`
	HashcatPath     string `json:"hashcat_path"`
	RulesetsPath    string `json:"rulesets_path"`
	TempPath        string `json:"temp_path"`
}

type HashcatManager struct {
	config         *WorkerConfig
	processes      map[string]*HashcatProcess
	processedJobs  map[string]bool // Track completed/failed jobs to prevent reprocessing
	overloadedJobs map[string]*OverloadedJob // Track locally overloaded jobs for retry
	httpClient     *http.Client
	maxProcesses   int // Maximum concurrent hashcat processes (from config or auto-detect)
}

type HashcatProcess struct {
	JobUID     string
	ChunkIndex int
	Cmd        *exec.Cmd
	Context    context.Context
	Cancel     context.CancelFunc
	Status     string
	StartTime  time.Time
	Cancelled  bool // Track if this process was manually cancelled
}

type OverloadedJob struct {
	Assignment      JobAssignment
	WordlistPath    string
	HashFile        string
	LastRetry       time.Time
	RetryCount      int
	OverloadThreshold int // The hashcat process count when this job was overloaded
}

type JobAssignment struct {
	JobUID         string `json:"job_uid"`
	ChunkIndex     int    `json:"chunk_index"`
	JobName        string `json:"job_name"`
	HashMode       int    `json:"hash_mode"`
	Rulesets       string `json:"rulesets"`
	DisablePotfile bool   `json:"disable_potfile"`
	HashesText     string `json:"hashes_text"`
	HashesFile     string `json:"hashes_file"`
}

func main() {
	var configPath = flag.String("config", "worker.json", "Path to worker configuration file")
	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	manager := NewHashcatManager(config)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go manager.Run()

	<-sigChan
	log.Println("Shutting down worker...")
	manager.Shutdown()
}

func loadConfig(path string) (*WorkerConfig, error) {
	// Check if config file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file '%s' not found - please create one from worker.json.example", path)
	}

	// Read config file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	// Parse config
	var config WorkerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Set hostname if not provided
	if config.Hostname == "" {
		config.Hostname = getHostname()
	}

	// Validate required fields
	if config.ServerURL == "" {
		return nil, fmt.Errorf("server_url must be set in config file")
	}
	if config.HashcatPath == "" {
		return nil, fmt.Errorf("hashcat_path must be set in config file")
	}
	if config.TempPath == "" {
		return nil, fmt.Errorf("temp_path must be set in config file")
	}

	// Set defaults for optional fields
	if config.CheckinInterval == "" {
		config.CheckinInterval = "30s"
	}
	if config.RulesetsPath == "" {
		config.RulesetsPath = "/opt/rules"
	}

	// Create temp directory
	if err := os.MkdirAll(config.TempPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %v", err)
	}

	return &config, nil
}

func getHostname() string {
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return "unknown-worker"
}

func NewHashcatManager(config *WorkerConfig) *HashcatManager {
	// Auto-detect max processes based on system (default to 4, but can be configured)
	maxProcs := 4 // Conservative default
	if cpuCount := runtime.NumCPU(); cpuCount > 1 {
		maxProcs = cpuCount / 2 // Half the CPU cores as a reasonable default
	}
	
	manager := &HashcatManager{
		config:         config,
		processes:      make(map[string]*HashcatProcess),
		processedJobs:  make(map[string]bool),
		overloadedJobs: make(map[string]*OverloadedJob),
		httpClient:     &http.Client{Timeout: 10 * time.Minute}, // Increased timeout for large wordlist downloads
		maxProcesses:   maxProcs,
	}

	// Start cleanup routine to prevent memory leaks
	go manager.startCleanupRoutine()

	return manager
}

func (hm *HashcatManager) Run() {
	log.Printf("Starting HashTasker worker on %s", hm.config.Hostname)
	log.Printf("Server URL: %s", hm.config.ServerURL)

	// Parse checkin interval
	checkinInterval, err := time.ParseDuration(hm.config.CheckinInterval)
	if err != nil {
		log.Printf("Invalid checkin interval '%s', using default 30s: %v", hm.config.CheckinInterval, err)
		checkinInterval = 30 * time.Second
	}

	ticker := time.NewTicker(checkinInterval)
	defer ticker.Stop()

	// Start overloaded job retry checker
	retryTicker := time.NewTicker(30 * time.Second) // Check for retries every 30 seconds
	defer retryTicker.Stop()

	// Initial checkin
	hm.checkin()

	for {
		select {
		case <-ticker.C:
			hm.checkin()
			hm.checkForJobCancellations()
			hm.checkForNewJobs()
		case <-retryTicker.C:
			hm.checkOverloadedJobs()
		}
	}
}

func (hm *HashcatManager) checkin() {
	stats := hm.collectSystemStats()

	data, err := json.Marshal(stats)
	if err != nil {
		log.Printf("Error marshaling stats: %v", err)
		return
	}

	resp, err := hm.httpClient.Post(
		hm.config.ServerURL+"/api/worker/stats",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		log.Printf("Error sending stats: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Server responded with status: %d", resp.StatusCode)
	}
}

func (hm *HashcatManager) collectSystemStats() *models.WorkerStats {
	stats := &models.WorkerStats{
		Hostname:     hm.config.Hostname,
		CPUUsage:     getCPUUsage(),
		MemoryUsed:   getMemoryUsed(),
		MemoryTotal:  getMemoryTotal(),
		DiskUsage:    getDiskUsage(),
		HashcatProcs: getHashcatProcessCount(),
		GPUs:         getGPUStats(),
	}

	return stats
}

func getCPUUsage() float64 {
	// Simple CPU usage calculation using /proc/stat
	cmd := exec.Command("sh", "-c", `
		grep 'cpu ' /proc/stat | awk '{usage=($2+$4)*100/($2+$3+$4+$5)} END {print usage}'
	`)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	usage, _ := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	return usage
}

func getMemoryUsed() int64 {
	cmd := exec.Command("sh", "-c", `
		free -b | grep 'Mem:' | awk '{print $3}'
	`)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	used, _ := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	return used
}

func getMemoryTotal() int64 {
	cmd := exec.Command("sh", "-c", `
		free -b | grep 'Mem:' | awk '{print $2}'
	`)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	total, _ := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	return total
}

func getDiskUsage() float64 {
	cmd := exec.Command("sh", "-c", `
		df / | tail -1 | awk '{print $5}' | sed 's/%//'
	`)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	usage, _ := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	return usage
}

func getHashcatProcessCount() int {
	// Count running hashcat.bin processes specifically
	// Use grep -v to exclude grep itself
	cmd := exec.Command("sh", "-c", `
		ps aux | grep 'hashcat\.bin' | grep -v grep | wc -l
	`)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return count
}

// getSystemHashcatProcessCount returns the total system-wide hashcat.bin process count
func (hm *HashcatManager) getSystemHashcatProcessCount() int {
	return getHashcatProcessCount()
}

// hasCapacityForNewProcess checks if we can start a new hashcat process
// This is now only used for initial job assignment - overload handling uses dynamic thresholds
func (hm *HashcatManager) hasCapacityForNewProcess() bool {
	systemCount := hm.getSystemHashcatProcessCount()
	log.Printf("System hashcat processes: %d/%d", systemCount, hm.maxProcesses)
	return systemCount < hm.maxProcesses
}

func getGPUStats() []models.GPUStats {
	// Try to get GPU stats using nvidia-smi
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=index,name,utilization.gpu,temperature.gpu,memory.used,memory.total",
		"--format=csv,noheader,nounits",
	)

	output, err := cmd.Output()
	if err != nil {
		return []models.GPUStats{}
	}

	var gpus []models.GPUStats
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ", ")
		if len(parts) < 6 {
			continue
		}

		index, _ := strconv.Atoi(parts[0])
		usage, _ := strconv.ParseFloat(parts[2], 64)
		temp, _ := strconv.ParseFloat(parts[3], 64)
		memUsed, _ := strconv.ParseInt(parts[4], 10, 64)
		memTotal, _ := strconv.ParseInt(parts[5], 10, 64)

		gpu := models.GPUStats{
			Index:    index,
			Name:     parts[1],
			Usage:    usage,
			Temp:     temp,
			MemUsed:  memUsed * 1024 * 1024, // Convert MB to bytes
			MemTotal: memTotal * 1024 * 1024,
		}

		gpus = append(gpus, gpu)
	}

	return gpus
}

// splitHashOutput splits hashcat output on the last colon to separate hash from plaintext
func splitHashOutput(line string) (string, string, bool) {
	// Find the last colon in the line
	lastColon := strings.LastIndex(line, ":")
	if lastColon == -1 {
		return "", "", false
	}
	
	hash := line[:lastColon]
	plaintext := line[lastColon+1:]
	
	return hash, plaintext, true
}

// isHashcatError checks if a line contains hashcat error messages that shouldn't be treated as cracked hashes
func isHashcatError(line string) bool {
	// Common hashcat error patterns
	errorPatterns := []string{
		"Separator unmatched",
		"Token length exception", 
		"hashfile",
		"Hashfile",
		"on line",
		"No hashes loaded",
		"Hash-file",
	}
	
	for _, pattern := range errorPatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	
	return false
}

// runHashcatShow runs hashcat --show to get previously cracked hashes from potfile
func (hm *HashcatManager) runHashcatShow(assignment JobAssignment, hashFile string) {
	log.Printf("Running hashcat --show to get existing cracked hashes for job %s", assignment.JobUID)
	
	// Build hashcat --show command
	showArgs := []string{
		"--show",
		"-m", fmt.Sprintf("%d", assignment.HashMode),
		"--outfile-format=1,2",
		hashFile,
	}
	
	// Add rulesets if any (needed for proper matching)
	if assignment.Rulesets != "" && assignment.Rulesets != "[]" {
		var rulesets []string
		if err := json.Unmarshal([]byte(assignment.Rulesets), &rulesets); err == nil {
			for _, ruleset := range rulesets {
				if ruleset != "" {
					rulesetPath := filepath.Join(hm.config.RulesetsPath, filepath.Base(ruleset))
					showArgs = append(showArgs, "-r", rulesetPath)
				}
			}
		}
	}
	
	cmd := exec.Command(hm.config.HashcatPath, showArgs...)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Hashcat --show command failed (this is normal if no hashes were previously cracked): %v", err)
		return
	}
	
	// Process the output to report existing cracked hashes (normalize newlines first)
	normalizedOutput := strings.ReplaceAll(string(output), "\r\n", "\n")
	normalizedOutput = strings.ReplaceAll(normalizedOutput, "\r", "\n")
	lines := strings.Split(normalizedOutput, "\n")
	existingCracks := 0
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Skip error messages
		if isHashcatError(line) {
			log.Printf("Skipping hashcat error message: %s", line)
			continue
		}
		
		hash, plaintext, ok := splitHashOutput(line)
		if !ok {
			continue
		}
		
		// Report existing cracked hash
		crackedHash := models.CrackedHash{
			Hash:         hash,
			Plaintext:    plaintext,
			RecoveryTime: 0, // Pre-existing, so 0 recovery time
		}
		
		data, _ := json.Marshal(crackedHash)
		resp, err := hm.httpClient.Post(
			hm.config.ServerURL+"/api/worker/cracked/"+assignment.JobUID,
			"application/json",
			bytes.NewBuffer(data),
		)
		if err != nil {
			log.Printf("Failed to report existing cracked hash: %v", err)
		} else {
			resp.Body.Close()
			existingCracks++
		}
	}
	
	if existingCracks > 0 {
		log.Printf("Reported %d existing cracked hashes from potfile", existingCracks)
	}
}

// checkForJobCancellations checks if any running jobs have been cancelled
func (hm *HashcatManager) checkForJobCancellations() {
	// Check each running process individually
	for jobKey, process := range hm.processes {
		// Use the worker assignment endpoint to check if job is still assigned
		// If not assigned, it means the job was cancelled
		req, err := http.NewRequest("GET", hm.config.ServerURL+"/api/worker/assignments", nil)
		if err != nil {
			log.Printf("Error creating assignments request: %v", err)
			continue
		}
		
		req.Header.Set("X-Worker-Hostname", hm.config.Hostname)
		
		resp, err := hm.httpClient.Do(req)
		if err != nil {
			log.Printf("Error getting assignments: %v", err)
			continue
		}
		
		if resp.StatusCode == http.StatusOK {
			var response struct {
				Assignments []JobAssignment `json:"assignments"`
			}
			
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				log.Printf("Error decoding assignments response: %v", err)
				resp.Body.Close()
				continue
			}
			
			// Check if this process's job/chunk is still in assignments
			stillAssigned := false
			for _, assignment := range response.Assignments {
				if assignment.JobUID == process.JobUID && assignment.ChunkIndex == process.ChunkIndex {
					stillAssigned = true
					break
				}
			}
			
			// If not assigned anymore, it means the job was cancelled
			if !stillAssigned {
				log.Printf("Job %s chunk %d is no longer assigned, stopping process (likely cancelled)", process.JobUID, process.ChunkIndex)
				
				// Mark as cancelled before killing the process
				process.Cancelled = true

				// Cancel the specific process
				process.Cancel()

				// Update chunk status to cancelled
				hm.updateChunkStatus(process.JobUID, process.ChunkIndex, "cancelled")

				// Clean up chunk files for cancelled job
				go hm.cleanupCancelledJobFiles(process.JobUID, process.ChunkIndex)
				
				// Report progress status as cancelled
				hm.reportProgress(&models.HashcatProgress{
					JobUID:        process.JobUID,
					Progress:      0,
					KeyspaceSize:  0,
					TimeElapsed:   0,
					TimeRemaining: 0,
					Status:        "cancelled",
				})
				
				// Remove from processes map
				delete(hm.processes, jobKey)
			}
		}
		resp.Body.Close()
	}
}

// checkForNewJobs polls the server for new job assignments
func (hm *HashcatManager) checkForNewJobs() {
	req, err := http.NewRequest("GET", hm.config.ServerURL+"/api/worker/assignments", nil)
	if err != nil {
		log.Printf("Error creating job assignments request: %v", err)
		return
	}
	
	req.Header.Set("X-Worker-Hostname", hm.config.Hostname)
	
	resp, err := hm.httpClient.Do(req)
	if err != nil {
		log.Printf("Error getting job assignments: %v", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		log.Printf("Server responded with status %d when getting assignments", resp.StatusCode)
		return
	}
	
	var response struct {
		Assignments []JobAssignment `json:"assignments"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Error decoding job assignments response: %v", err)
		return
	}
	
	log.Printf("Received %d job assignments", len(response.Assignments))
	
	for _, assignment := range response.Assignments {
		// Skip assignments with empty job UIDs
		if assignment.JobUID == "" {
			log.Printf("Warning: Received assignment with empty JobUID, skipping chunk %d", assignment.ChunkIndex)
			// Mark this invalid assignment as processed to prevent repeated warnings
			invalidKey := fmt.Sprintf("INVALID_%d", assignment.ChunkIndex)
			hm.processedJobs[invalidKey] = true
			continue
		}

		jobKey := fmt.Sprintf("%s_%d", assignment.JobUID, assignment.ChunkIndex)

		// Skip if already running
		if _, exists := hm.processes[jobKey]; exists {
			log.Printf("Skipping job %s - already running", jobKey)
			continue
		}
		
		// Skip if already processed (completed/failed)
		if hm.processedJobs[jobKey] {
			log.Printf("Skipping job %s - already processed", jobKey)
			continue
		}
		
		// Check if we have capacity before starting
		if !hm.hasCapacityForNewProcess() {
			currentProcs := hm.getSystemHashcatProcessCount()
			log.Printf("Worker overloaded (%d hashcat processes), queuing job %s chunk %d for retry", currentProcs, assignment.JobUID, assignment.ChunkIndex)
			hm.queueOverloadedJob(assignment, "", currentProcs)
			continue
		}
		
		log.Printf("Starting new job assignment: %s chunk %d", assignment.JobUID, assignment.ChunkIndex)
		hm.processedJobs[jobKey] = true // Mark as processing immediately
		go hm.startJobChunk(assignment)
	}
}

// startJobChunk starts a hashcat process for a specific job chunk
func (hm *HashcatManager) startJobChunk(assignment JobAssignment) {
	jobKey := fmt.Sprintf("%s_%d", assignment.JobUID, assignment.ChunkIndex)
	
	// Download wordlist chunk
	log.Printf("Attempting to download wordlist chunk for job %s chunk %d", assignment.JobUID, assignment.ChunkIndex)
	wordlistPath, err := hm.downloadWordlistChunk(assignment.JobUID, assignment.ChunkIndex)
	if err != nil {
		log.Printf("Failed to download wordlist chunk for job %s: %v", assignment.JobUID, err)
		hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
		return
	}
	log.Printf("Successfully downloaded wordlist chunk to: %s", wordlistPath)
	
	// Update chunk status to running
	hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "running")
	
	// Prepare hashcat command
	ctx, cancel := context.WithCancel(context.Background())
	
	sessionID := fmt.Sprintf("%s_%d_%d", assignment.JobUID, assignment.ChunkIndex, time.Now().Unix())
	
	// Create hash file - create a temporary copy for this chunk
	hashFile := filepath.Join(hm.config.TempPath, fmt.Sprintf("%s_%d.hashes", assignment.JobUID, assignment.ChunkIndex))
	log.Printf("Creating hash file: %s", hashFile)
	
	if assignment.HashesText != "" {
		log.Printf("Writing %d characters of hash text to file", len(assignment.HashesText))
		// Ensure Unix line endings for consistency (hashesText should already be normalized from server)
		normalizedHashes := strings.ReplaceAll(assignment.HashesText, "\r\n", "\n")
		normalizedHashes = strings.ReplaceAll(normalizedHashes, "\r", "\n")
		err := os.WriteFile(hashFile, []byte(normalizedHashes), 0644)
		if err != nil {
			log.Printf("Failed to write hash file %s: %v", hashFile, err)
			hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
			return
		}
	} else if assignment.HashesFile != "" {
		log.Printf("Copying hash file from: %s", assignment.HashesFile)
		// Copy the original hash file to a temporary file for this chunk
		originalContent, err := os.ReadFile(assignment.HashesFile)
		if err != nil {
			log.Printf("Failed to read original hash file %s: %v", assignment.HashesFile, err)
			hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
			return
		}
		log.Printf("Read %d bytes from original hash file", len(originalContent))
		// Normalize newlines in the copied file content
		normalizedContent := strings.ReplaceAll(string(originalContent), "\r\n", "\n")
		normalizedContent = strings.ReplaceAll(normalizedContent, "\r", "\n")
		err = os.WriteFile(hashFile, []byte(normalizedContent), 0644)
		if err != nil {
			log.Printf("Failed to write hash file %s: %v", hashFile, err)
			hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
			return
		}
	} else {
		log.Printf("No hashes provided in assignment - neither HashesText nor HashesFile")
		hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
		return
	}
	
	log.Printf("Successfully created hash file: %s", hashFile)
	
	// Add output file for cracked hashes
	potFile := filepath.Join(hm.config.TempPath, fmt.Sprintf("%s_%d.pot", assignment.JobUID, assignment.ChunkIndex))
	
	// Build hashcat arguments in correct order
	args := []string{
		"--quiet",
		"-a", "0", // Dictionary attack
		"-w", "4", // Workload profile 4
		"-m", fmt.Sprintf("%d", assignment.HashMode), // Hash mode
		"--outfile-format=1,2", // Format: hash:plain
		"-o", potFile, // Output file
		"--machine-readable",
		"--status",
		"--restore-disable",
		"--status-timer=5",
		"-O", // Optimize for 32 characters or less passwords
		"--session", sessionID,
	}
	
	// Add potfile disable option if requested
	if assignment.DisablePotfile {
		args = append(args, "--potfile-disable")
	}
	
	// Handle multiple rulesets
	if assignment.Rulesets != "" && assignment.Rulesets != "[]" {
		var rulesets []string
		if err := json.Unmarshal([]byte(assignment.Rulesets), &rulesets); err == nil {
			for _, ruleset := range rulesets {
				if ruleset != "" {
					rulesetPath := filepath.Join(hm.config.RulesetsPath, filepath.Base(ruleset))
					args = append(args, "-r", rulesetPath)
				}
			}
		}
	}
	
	// Verify files exist before running hashcat
	if _, err := os.Stat(hashFile); os.IsNotExist(err) {
		log.Printf("Hash file does not exist: %s", hashFile)
		hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
		return
	}
	if _, err := os.Stat(wordlistPath); os.IsNotExist(err) {
		log.Printf("Wordlist file does not exist: %s", wordlistPath)
		hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
		return
	}
	
	// Add hash file and wordlist at the end (correct hashcat argument order)
	args = append(args, hashFile, wordlistPath)
	
	cmd := exec.CommandContext(ctx, hm.config.HashcatPath, args...)
	
	// Log the full hashcat command for debugging
	fullCmd := append([]string{hm.config.HashcatPath}, args...)
	log.Printf("Running Hashcat Command: %s", strings.Join(fullCmd, " "))
	
	process := &HashcatProcess{
		JobUID:     assignment.JobUID,
		ChunkIndex: assignment.ChunkIndex,
		Cmd:        cmd,
		Context:    ctx,
		Cancel:     cancel,
		Status:     "running",
		StartTime:  time.Now(),
	}
	
	hm.processes[jobKey] = process
	
	// Start the process
	go func() {
		defer cancel()
		defer func() {
			delete(hm.processes, jobKey)
		}()
		
		log.Printf("Starting hashcat job %s chunk %d", assignment.JobUID, assignment.ChunkIndex)
		
		// Set up pipes to capture hashcat output
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("Failed to create stdout pipe: %v", err)
			hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
			return
		}
		
		cmd.Stderr = os.Stderr
		
		// Start the command
		err = cmd.Start()
		if err != nil {
			log.Printf("Failed to start hashcat: %v", err)
			hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
			return
		}
		
		// If potfile is enabled, run --show command first to get existing cracks
		if !assignment.DisablePotfile {
			hm.runHashcatShow(assignment, hashFile)
		}
		
		// Monitor hashcat output for progress and cracked hashes
		go hm.monitorHashcatOutput(assignment.JobUID, assignment.ChunkIndex, stdout, potFile)
		
		// Monitor potfile for real-time cracked hashes
		go hm.monitorPotFile(ctx, assignment.JobUID, assignment.ChunkIndex, potFile, time.Now())
		
		// Wait for completion
		err = cmd.Wait()
		
		if err != nil {
			// If the process was cancelled, keep the cancelled status
			if process.Cancelled {
				log.Printf("Hashcat job %s chunk %d was cancelled and terminated", assignment.JobUID, assignment.ChunkIndex)
				process.Status = "cancelled"
				// Status was already updated when cancellation was detected
			} else {
				// Handle specific hashcat exit codes
				if exitError, ok := err.(*exec.ExitError); ok {
					exitCode := exitError.ExitCode()
					log.Printf("Hashcat job %s chunk %d exited with code %d", assignment.JobUID, assignment.ChunkIndex, exitCode)
					
					switch exitCode {
					case 1:
						// Exhausted - all passwords tried, none found
						log.Printf("Hashcat job %s chunk %d exhausted (no more passwords to try)", assignment.JobUID, assignment.ChunkIndex)
						process.Status = "exhausted"
						hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "exhausted")
					case 2:
						// Aborted via checkpoint
						log.Printf("Hashcat job %s chunk %d aborted", assignment.JobUID, assignment.ChunkIndex)
						process.Status = "aborted"
						hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "aborted")
					case 252:
						// Worker overloaded - too many jobs running
						currentProcs := hm.getSystemHashcatProcessCount()
						log.Printf("Hashcat job %s chunk %d not started - worker overloaded (exit 252, %d processes), queuing for retry", assignment.JobUID, assignment.ChunkIndex, currentProcs)
						process.Status = "overloaded"
						hm.queueOverloadedJob(assignment, wordlistPath, currentProcs)
						// Don't report as overloaded to server, handle locally
					case 255:
						// Invalid parameters or other error
						log.Printf("Hashcat job %s chunk %d failed with invalid parameters (exit 255)", assignment.JobUID, assignment.ChunkIndex)
						process.Status = "failed"
						hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
					default:
						log.Printf("Hashcat job %s chunk %d failed: %v", assignment.JobUID, assignment.ChunkIndex, err)
						process.Status = "failed"
						hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
					}
				} else {
					log.Printf("Hashcat job %s chunk %d failed: %v", assignment.JobUID, assignment.ChunkIndex, err)
					process.Status = "failed"
					hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "failed")
				}
			}
		} else {
			log.Printf("Hashcat job %s chunk %d completed successfully", assignment.JobUID, assignment.ChunkIndex)
			process.Status = "completed"
			hm.updateChunkStatus(assignment.JobUID, assignment.ChunkIndex, "completed")
			
			// Final potfile check is handled by the real-time monitoring
			log.Printf("Hashcat job %s chunk %d completed successfully", assignment.JobUID, assignment.ChunkIndex)
		}
		
		// Clean up local chunk files
		hm.cleanupJobChunkFiles(assignment.JobUID, assignment.ChunkIndex, potFile, wordlistPath, hashFile)
	}()
}

// monitorHashcatOutput monitors hashcat's stdout for progress updates
func (hm *HashcatManager) monitorHashcatOutput(jobUID string, chunkIndex int, stdout io.ReadCloser, potFile string) {
	defer stdout.Close()
	
	startTime := time.Now()
	scanner := bufio.NewScanner(stdout)
	
	for scanner.Scan() {
		line := scanner.Text()
		
		if strings.HasPrefix(line, "STATUS") {
			progress := hm.parseHashcatStatus(line, startTime)
			if progress != nil {
				progress.JobUID = jobUID
				log.Printf("Progress update: %.1f%% complete, %ds remaining", progress.Progress, progress.TimeRemaining)
				hm.reportProgress(progress)
			}
		}
		
		// Also output to console for debugging
		fmt.Println(line)
	}
	
	log.Printf("Hashcat output monitoring finished for job %s chunk %d", jobUID, chunkIndex)
}

// parseHashcatStatus parses hashcat STATUS output line with time calculations
func (hm *HashcatManager) parseHashcatStatus(line string, startTime time.Time) *models.HashcatProgress {
	parts := strings.Split(strings.TrimSpace(line), "\t")
	if len(parts) < 4 {
		return nil
	}
	
	progress := &models.HashcatProgress{}
	elapsed := time.Since(startTime)
	progress.TimeElapsed = int64(elapsed.Seconds())
	
	// Parse status code
	if len(parts) > 1 {
		if statusCode, err := strconv.Atoi(parts[1]); err == nil {
			statusMap := map[int]string{
				3: "running",
				5: "exhausted", 
				6: "cracked",
				7: "aborted",
				8: "quit",
			}
			progress.Status = statusMap[statusCode]
			if progress.Status == "" {
				progress.Status = "unknown"
			}
		}
	}
	
	// Find PROGRESS section to calculate progress and time remaining
	for i, part := range parts {
		if part == "PROGRESS" && i+2 < len(parts) {
			if done, err := strconv.ParseInt(parts[i+1], 10, 64); err == nil {
				if total, err := strconv.ParseInt(parts[i+2], 10, 64); err == nil {
					progress.KeyspaceSize = total
					if total > 0 {
						progress.Progress = float64(done) / float64(total) * 100
						
						// Calculate estimated time remaining
						if done > 0 && progress.Progress > 0 {
							// Time per percent = elapsed / progress_percent
							timePerPercent := elapsed.Seconds() / progress.Progress
							remainingPercent := 100.0 - progress.Progress
							progress.TimeRemaining = int64(timePerPercent * remainingPercent)
						}
					}
				}
			}
			break
		}
	}
	
	return progress
}

// monitorPotFile monitors the potfile for new cracked hashes in real-time
func (hm *HashcatManager) monitorPotFile(ctx context.Context, jobUID string, chunkIndex int, potFile string, startTime time.Time) {
	log.Printf("Starting potfile monitoring for %s", potFile)
	
	var lastSize int64 = 0
	reportedHashes := make(map[string]bool) // Track already reported hashes
	
	// Monitor the potfile every 2 seconds
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Printf("Potfile monitoring stopped for job %s chunk %d", jobUID, chunkIndex)
			return
		case <-ticker.C:
			// Check if file exists
			fileInfo, err := os.Stat(potFile)
			if err != nil {
				continue // File doesn't exist yet
			}
			
			// If file size hasn't changed, skip
			if fileInfo.Size() == lastSize {
				continue
			}
			
			// File has grown, read new content
			content, err := os.ReadFile(potFile)
			if err != nil {
				log.Printf("Error reading potfile %s: %v", potFile, err)
				continue
			}
			
			// Normalize newlines before processing potfile content
			normalizedContent := strings.ReplaceAll(string(content), "\r\n", "\n")
			normalizedContent = strings.ReplaceAll(normalizedContent, "\r", "\n")
			lines := strings.Split(normalizedContent, "\n")
			newHashes := 0
			
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				
				// Skip if we've already reported this hash
				if reportedHashes[line] {
					continue
				}
				
				// Skip error messages
				if isHashcatError(line) {
					log.Printf("Skipping hashcat error message: %s", line)
					continue
				}
				
				hash, plaintext, ok := splitHashOutput(line)
				if !ok {
					continue
				}
				
				// Mark as reported
				reportedHashes[line] = true
				
				// Calculate recovery time from job start
				recoveryTime := int64(time.Since(startTime).Seconds())
				
				// Report the cracked hash immediately
				crackedHash := models.CrackedHash{
					Hash:         hash,
					Plaintext:    plaintext,
					RecoveryTime: recoveryTime,
				}
				
				data, _ := json.Marshal(crackedHash)
				resp, err := hm.httpClient.Post(
					hm.config.ServerURL+"/api/worker/cracked/"+jobUID,
					"application/json",
					bytes.NewBuffer(data),
				)
				if err != nil {
					log.Printf("Failed to report cracked hash: %v", err)
				} else {
					resp.Body.Close()
					newHashes++
					log.Printf("Reported newly cracked hash: %s (cracked after %ds)", hash, recoveryTime)
				}
			}
			
			if newHashes > 0 {
				log.Printf("Reported %d new cracked hashes from potfile", newHashes)
			}
			
			lastSize = fileInfo.Size()
		}
	}
}

// reportProgress sends progress update to server
func (hm *HashcatManager) reportProgress(progress *models.HashcatProgress) {
	data, err := json.Marshal(progress)
	if err != nil {
		return
	}
	
	req, err := http.NewRequest("POST",
		hm.config.ServerURL+"/api/worker/progress",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Worker-Hostname", hm.config.Hostname)
	
	hm.httpClient.Do(req)
}

// queueOverloadedJob adds a job to the local overloaded queue for retry
func (hm *HashcatManager) queueOverloadedJob(assignment JobAssignment, wordlistPath string, overloadThreshold int) {
	jobKey := fmt.Sprintf("%s_%d", assignment.JobUID, assignment.ChunkIndex)
	
	overloadedJob := &OverloadedJob{
		Assignment:        assignment,
		WordlistPath:      wordlistPath,
		LastRetry:         time.Now(),
		RetryCount:        0,
		OverloadThreshold: overloadThreshold,
	}
	
	hm.overloadedJobs[jobKey] = overloadedJob
	log.Printf("Queued overloaded job %s for local retry (threshold: %d processes)", jobKey, overloadThreshold)
}

// checkOverloadedJobs checks if we can retry any overloaded jobs
func (hm *HashcatManager) checkOverloadedJobs() {
	if len(hm.overloadedJobs) == 0 {
		return
	}
	
	currentProcs := hm.getSystemHashcatProcessCount()
	log.Printf("Checking %d overloaded jobs for retry (current: %d processes)", len(hm.overloadedJobs), currentProcs)
	
	// Try to retry overloaded jobs
	for jobKey, overloadedJob := range hm.overloadedJobs {
		// Check if job is still valid (not cancelled/deleted) by checking server
		if hm.isJobCancelled(overloadedJob.Assignment) {
			log.Printf("Overloaded job %s was cancelled, removing from retry queue", jobKey)
			delete(hm.overloadedJobs, jobKey)
			continue
		}
		
		// Check if process count has dropped below the overload threshold for this specific job
		if currentProcs >= overloadedJob.OverloadThreshold {
			log.Printf("Job %s still waiting: current=%d, needs<%d", jobKey, currentProcs, overloadedJob.OverloadThreshold)
			continue
		}
		
		// Capacity is now available for this job
		log.Printf("Retrying overloaded job %s (attempt %d): processes dropped from %d to %d", 
			jobKey, overloadedJob.RetryCount+1, overloadedJob.OverloadThreshold, currentProcs)
		
		overloadedJob.RetryCount++
		overloadedJob.LastRetry = time.Now()
		
		// Remove from overloaded queue and start normally
		delete(hm.overloadedJobs, jobKey)
		hm.processedJobs[jobKey] = true
		
		go hm.startJobChunk(overloadedJob.Assignment)
		
		// Only retry one job at a time to avoid overwhelming the system again
		break
	}
}

// isJobCancelled checks if a job has been cancelled or deleted on the server
func (hm *HashcatManager) isJobCancelled(assignment JobAssignment) bool {
	req, err := http.NewRequest("GET", hm.config.ServerURL+"/api/worker/assignments", nil)
	if err != nil {
		log.Printf("Error creating assignments request for cancellation check: %v", err)
		return false // Assume not cancelled on error
	}
	
	req.Header.Set("X-Worker-Hostname", hm.config.Hostname)
	
	resp, err := hm.httpClient.Do(req)
	if err != nil {
		log.Printf("Error checking assignments for cancellation: %v", err)
		return false // Assume not cancelled on error
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return false // Assume not cancelled on error
	}
	
	var response struct {
		Assignments []JobAssignment `json:"assignments"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Error decoding assignments response for cancellation check: %v", err)
		return false
	}
	
	// Check if this job is still in the assignments
	for _, serverAssignment := range response.Assignments {
		if serverAssignment.JobUID == assignment.JobUID && serverAssignment.ChunkIndex == assignment.ChunkIndex {
			return false // Job still exists, not cancelled
		}
	}
	
	return true // Job not found in assignments, likely cancelled
}

// downloadWordlistChunk downloads a wordlist chunk from the server
func (hm *HashcatManager) downloadWordlistChunk(jobUID string, chunkIndex int) (string, error) {
	url := fmt.Sprintf("%s/api/worker/wordlist/%s/%d", hm.config.ServerURL, jobUID, chunkIndex)
	log.Printf("Downloading wordlist chunk from: %s", url)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	
	req.Header.Set("X-Worker-Hostname", hm.config.Hostname)
	
	resp, err := hm.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		// Try to read error response
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server responded with status %d: %s", resp.StatusCode, string(body))
	}
	
	// Create local wordlist file
	wordlistPath := filepath.Join(hm.config.TempPath, fmt.Sprintf("%s_%d.wordlist", jobUID, chunkIndex))
	file, err := os.Create(wordlistPath)
	if err != nil {
		return "", fmt.Errorf("failed to create wordlist file %s: %v", wordlistPath, err)
	}
	defer file.Close()
	
	// Copy response body to file
	bytesWritten, err := io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(wordlistPath)
		return "", fmt.Errorf("failed to write wordlist file: %v", err)
	}
	
	log.Printf("Successfully downloaded %d bytes to wordlist file: %s", bytesWritten, wordlistPath)
	return wordlistPath, nil
}

// updateChunkStatus updates the status of a job chunk on the server
func (hm *HashcatManager) updateChunkStatus(jobUID string, chunkIndex int, status string) {
	url := fmt.Sprintf("%s/api/worker/chunk/%s/%d/status", hm.config.ServerURL, jobUID, chunkIndex)
	
	statusData := map[string]string{"status": status}
	data, _ := json.Marshal(statusData)
	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Error creating chunk status request: %v", err)
		return
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Worker-Hostname", hm.config.Hostname)
	
	resp, err := hm.httpClient.Do(req)
	if err != nil {
		log.Printf("Error updating chunk status: %v", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		log.Printf("Server responded with status %d when updating chunk status", resp.StatusCode)
	}
}

func (hm *HashcatManager) Shutdown() {
	log.Println("Stopping all running jobs...")

	for jobKey := range hm.processes {
		parts := strings.Split(jobKey, "_")
		if len(parts) >= 2 {
			jobUID := strings.Join(parts[:len(parts)-1], "_")
			chunkIndex, _ := strconv.Atoi(parts[len(parts)-1])
			hm.updateChunkStatus(jobUID, chunkIndex, "cancelled")
		}
		hm.processes[jobKey].Cancel()
	}

	// Wait a bit for processes to stop gracefully
	time.Sleep(5 * time.Second)

	log.Println("Worker shutdown complete")
}

// startCleanupRoutine periodically cleans up processed jobs to prevent memory leaks
func (hm *HashcatManager) startCleanupRoutine() {
	ticker := time.NewTicker(30 * time.Minute) // Clean up every 30 minutes
	defer ticker.Stop()

	for range ticker.C {
		// Clean up processed jobs map - keep only recent entries
		// This prevents the map from growing infinitely over time
		if len(hm.processedJobs) > 1000 { // Only clean if map is large
			log.Printf("Cleaning up processed jobs map (current size: %d)", len(hm.processedJobs))

			// Keep only running jobs and recently processed invalid jobs
			newProcessedJobs := make(map[string]bool)

			// Keep all currently running jobs
			for jobKey := range hm.processes {
				newProcessedJobs[jobKey] = true
			}

			// Keep INVALID entries (these are smaller in number)
			for jobKey, processed := range hm.processedJobs {
				if strings.HasPrefix(jobKey, "INVALID_") && processed {
					newProcessedJobs[jobKey] = true
				}
			}

			hm.processedJobs = newProcessedJobs
			log.Printf("Processed jobs map cleaned up (new size: %d)", len(hm.processedJobs))
		}
	}
}

// cleanupJobChunkFiles removes temporary files for a completed/failed job chunk
func (hm *HashcatManager) cleanupJobChunkFiles(jobUID string, chunkIndex int, potFile, wordlistPath, hashFile string) {
	log.Printf("Cleaning up files for job %s chunk %d", jobUID, chunkIndex)

	// Remove temporary files
	filesToClean := []string{potFile, wordlistPath, hashFile}
	for _, file := range filesToClean {
		if file != "" {
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				log.Printf("Warning: Failed to remove file %s: %v", file, err)
			} else if err == nil {
				log.Printf("Cleaned up file: %s", file)
			}
		}
	}

	// Also clean up any other temporary files for this job chunk
	tempPattern := filepath.Join(hm.config.TempPath, fmt.Sprintf("%s_%d.*", jobUID, chunkIndex))
	matches, err := filepath.Glob(tempPattern)
	if err == nil {
		for _, match := range matches {
			if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
				log.Printf("Warning: Failed to remove temp file %s: %v", match, err)
			} else if err == nil {
				log.Printf("Cleaned up temp file: %s", match)
			}
		}
	}

	log.Printf("File cleanup completed for job %s chunk %d", jobUID, chunkIndex)
}

// cleanupCancelledJobFiles removes temporary files for a cancelled job chunk
func (hm *HashcatManager) cleanupCancelledJobFiles(jobUID string, chunkIndex int) {
	log.Printf("Cleaning up files for cancelled job %s chunk %d", jobUID, chunkIndex)

	// Find and remove all files for this job chunk
	tempPattern := filepath.Join(hm.config.TempPath, fmt.Sprintf("%s_%d.*", jobUID, chunkIndex))
	matches, err := filepath.Glob(tempPattern)
	if err == nil {
		for _, match := range matches {
			if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
				log.Printf("Warning: Failed to remove cancelled job file %s: %v", match, err)
			} else if err == nil {
				log.Printf("Cleaned up cancelled job file: %s", match)
			}
		}
	}

	log.Printf("Cancelled job file cleanup completed for job %s chunk %d", jobUID, chunkIndex)
}
