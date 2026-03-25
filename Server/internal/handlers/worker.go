package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hashtasker-go/internal/database"
	"hashtasker-go/internal/models"
	wordlistpkg "hashtasker-go/internal/wordlist"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type WorkerHandler struct {
	uploadPath string
}

func NewWorkerHandler(uploadPath string) *WorkerHandler {
	return &WorkerHandler{
		uploadPath: uploadPath,
	}
}

func (h *WorkerHandler) RegisterWorker(c *gin.Context) {
	var stats models.WorkerStats
	if err := c.ShouldBindJSON(&stats); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}
	
	var worker models.Worker
	result := database.DB.Where("hostname = ?", stats.Hostname).First(&worker)
	
	if result.Error == gorm.ErrRecordNotFound {
		worker = models.Worker{
			Hostname:     stats.Hostname,
			CPUUsage:     stats.CPUUsage,
			MemoryUsed:   stats.MemoryUsed,
			MemoryTotal:  stats.MemoryTotal,
			DiskUsage:    stats.DiskUsage,
			HashcatProcs: stats.HashcatProcs,
			LastCheckin:  time.Now(),
			IsOnline:     true,
		}
		
		if err := database.DB.Create(&worker).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register worker"})
			return
		}
	} else {
		worker.CPUUsage = stats.CPUUsage
		worker.MemoryUsed = stats.MemoryUsed
		worker.MemoryTotal = stats.MemoryTotal
		worker.DiskUsage = stats.DiskUsage
		worker.HashcatProcs = stats.HashcatProcs
		worker.LastCheckin = time.Now()
		worker.IsOnline = true
		
		database.DB.Save(&worker)
	}
	
	database.DB.Where("worker_id = ?", worker.ID).Delete(&models.GPU{})
	
	for _, gpu := range stats.GPUs {
		newGPU := models.GPU{
			WorkerID: worker.ID,
			Index:    gpu.Index,
			Name:     gpu.Name,
			Usage:    gpu.Usage,
			Temp:     gpu.Temp,
			MemUsed:  gpu.MemUsed,
			MemTotal: gpu.MemTotal,
		}
		database.DB.Create(&newGPU)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"worker_id": worker.ID,
	})
}

func (h *WorkerHandler) UpdateWorkerStats(c *gin.Context) {
	var stats models.WorkerStats
	if err := c.ShouldBindJSON(&stats); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}
	
	var worker models.Worker
	result := database.DB.Where("hostname = ?", stats.Hostname).First(&worker)
	
	if result.Error == gorm.ErrRecordNotFound {
		// Worker doesn't exist, create it
		worker = models.Worker{
			Hostname:     stats.Hostname,
			CPUUsage:     stats.CPUUsage,
			MemoryUsed:   stats.MemoryUsed,
			MemoryTotal:  stats.MemoryTotal,
			DiskUsage:    stats.DiskUsage,
			HashcatProcs: stats.HashcatProcs,
			LastCheckin:  time.Now(),
			IsOnline:     true,
		}
		
		if err := database.DB.Create(&worker).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register worker"})
			return
		}
		
		log.Printf("New worker registered: %s", stats.Hostname)
	} else if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	} else {
		// Worker exists, update it
		worker.CPUUsage = stats.CPUUsage
		worker.MemoryUsed = stats.MemoryUsed
		worker.MemoryTotal = stats.MemoryTotal
		worker.DiskUsage = stats.DiskUsage
		worker.HashcatProcs = stats.HashcatProcs
		worker.LastCheckin = time.Now()
		worker.IsOnline = true
		
		database.DB.Save(&worker)
	}
	
	// Update GPU information
	database.DB.Where("worker_id = ?", worker.ID).Delete(&models.GPU{})
	
	for _, gpu := range stats.GPUs {
		newGPU := models.GPU{
			WorkerID: worker.ID,
			Index:    gpu.Index,
			Name:     gpu.Name,
			Usage:    gpu.Usage,
			Temp:     gpu.Temp,
			MemUsed:  gpu.MemUsed,
			MemTotal: gpu.MemTotal,
		}
		database.DB.Create(&newGPU)
	}
	
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *WorkerHandler) ReportHashcatProgress(c *gin.Context) {
	var progress models.HashcatProgress
	if err := c.ShouldBindJSON(&progress); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}
	
	hostname := c.GetHeader("X-Worker-Hostname")
	if hostname == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Worker hostname header required"})
		return
	}
	
	var job models.Job
	result := database.DB.Where("uid = ?", progress.JobUID).First(&job)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}
	
	var jobProgress models.JobProgress
	progressResult := database.DB.Where("job_id = ? AND worker_name = ?", job.ID, hostname).First(&jobProgress)
	
	// Treat "exhausted" as "completed" since it means the work is done
	progressStatus := progress.Status
	if progressStatus == "exhausted" {
		progressStatus = "completed"
	}
	
	if progressResult.Error == gorm.ErrRecordNotFound {
		jobProgress = models.JobProgress{
			JobID:         job.ID,
			WorkerName:    hostname,
			Progress:      progress.Progress,
			Status:        progressStatus,
			KeyspaceSize:  progress.KeyspaceSize,
			TimeElapsed:   progress.TimeElapsed,
			TimeRemaining: progress.TimeRemaining,
		}
		
		if progressStatus == "completed" || progressStatus == "failed" {
			now := time.Now()
			jobProgress.FinishedAt = &now
		}
		
		database.DB.Create(&jobProgress)
	} else {
		jobProgress.Progress = progress.Progress
		jobProgress.Status = progressStatus
		jobProgress.KeyspaceSize = progress.KeyspaceSize
		jobProgress.TimeElapsed = progress.TimeElapsed
		jobProgress.TimeRemaining = progress.TimeRemaining
		
		if progressStatus == "completed" || progressStatus == "failed" {
			now := time.Now()
			jobProgress.FinishedAt = &now
		}
		
		database.DB.Save(&jobProgress)
	}
	
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *WorkerHandler) ReportCrackedHash(c *gin.Context) {
	var crackedHash models.CrackedHash
	if err := c.ShouldBindJSON(&crackedHash); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	jobUID := c.Param("job_uid")

	var job models.Job
	result := database.DB.Where("uid = ?", jobUID).First(&job)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	crackedHash.JobID = job.ID
	crackedHash.CrackedAt = time.Now()

	// Check if this hash was already cracked for this job (deduplication)
	var existingHash models.CrackedHash
	err := database.DB.Where("job_id = ? AND hash = ?", job.ID, crackedHash.Hash).First(&existingHash).Error

	if err == nil {
		// Hash already exists - skip inserting duplicate but return success
		log.Printf("Hash already cracked for job %s, skipping duplicate: %s", jobUID, crackedHash.Hash)
		c.JSON(http.StatusOK, gin.H{"status": "success", "duplicate": true})
		return
	}

	// Create new cracked hash record
	if err := database.DB.Create(&crackedHash).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save cracked hash"})
		return
	}

	// Record analytics for cracked hash
	RecordCrackedHashes(1)

	// Update cracked count using DISTINCT to avoid counting duplicates
	var crackedCount int64
	database.DB.Model(&models.CrackedHash{}).Where("job_id = ?", job.ID).Distinct("hash").Count(&crackedCount)

	job.CrackedCount = int(crackedCount)
	database.DB.Save(&job)

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *WorkerHandler) MarkWorkersOffline() {
	cutoff := time.Now().Add(-60 * time.Second)
	database.DB.Model(&models.Worker{}).Where("last_checkin < ?", cutoff).Update("is_online", false)
}

// GetJobAssignments returns queued job chunks for a specific worker
func (h *WorkerHandler) GetJobAssignments(c *gin.Context) {
	hostname := c.GetHeader("X-Worker-Hostname")
	if hostname == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Worker hostname header required"})
		return
	}

	// Find worker by hostname
	var worker models.Worker
	if err := database.DB.Where("hostname = ?", hostname).First(&worker).Error; err != nil {
		// Worker not found, return empty assignments (worker will be created when it reports stats)
		c.JSON(http.StatusOK, gin.H{"assignments": []gin.H{}})
		return
	}

	// Find queued and running job chunks for this worker (overloaded chunks handled locally)
	var jobChunks []models.JobChunk
	if err := database.DB.Preload("Job").Where("worker_id = ? AND status IN ?", worker.ID, []models.JobStatus{models.JobStatusQueued, models.JobStatusRunning}).Find(&jobChunks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get job assignments"})
		return
	}

	// Convert to response format
	var assignments []gin.H
	for _, chunk := range jobChunks {
		// Skip chunks with empty job UIDs (corrupted data)
		if chunk.Job.UID == "" {
			log.Printf("Warning: Found job chunk %d with empty job UID, skipping", chunk.ID)
			continue
		}

		assignments = append(assignments, gin.H{
			"job_uid":         chunk.Job.UID,
			"chunk_index":     chunk.ChunkIndex,
			"job_name":        chunk.Job.Name,
			"hash_mode":       chunk.Job.HashMode,
			"rulesets":        chunk.Job.Rulesets,
			"disable_potfile": chunk.Job.DisablePotfile,
			"username_mode":   chunk.Job.UsernameMode,
			"hashes_text":     chunk.Job.HashesText,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"assignments": assignments,
	})
}

// GetWordlistChunk serves a wordlist chunk file to a worker
func (h *WorkerHandler) GetWordlistChunk(c *gin.Context) {
	hostname := c.GetHeader("X-Worker-Hostname")
	if hostname == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Worker hostname header required"})
		return
	}

	jobUID := c.Param("job_uid")
	chunkIndexStr := c.Param("chunk_index")

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid chunk index"})
		return
	}

	// Find worker by hostname
	var worker models.Worker
	if err := database.DB.Where("hostname = ?", hostname).First(&worker).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Worker not found"})
		return
	}

	// Find the job chunk
	var jobChunk models.JobChunk
	if err := database.DB.Preload("Job").Where("worker_id = ? AND chunk_index = ?", worker.ID, chunkIndex).
		Joins("JOIN jobs ON job_chunks.job_id = jobs.id").
		Where("jobs.uid = ?", jobUID).First(&jobChunk).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job chunk not found"})
		return
	}

	// Check if chunk file exists
	if _, err := os.Stat(jobChunk.ChunkPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wordlist chunk file not found"})
		return
	}

	// Serve the file
	file, err := os.Open(jobChunk.ChunkPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open wordlist chunk"})
		return
	}
	defer file.Close()

	// Set headers
	c.Header("Content-Type", "text/plain")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_chunk_%d.wordlist\"", jobUID, chunkIndex))

	// Stream file content
	_, err = io.Copy(c.Writer, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send wordlist chunk"})
		return
	}
}

// UpdateChunkStatus updates the status of a job chunk
func (h *WorkerHandler) UpdateChunkStatus(c *gin.Context) {
	hostname := c.GetHeader("X-Worker-Hostname")
	if hostname == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Worker hostname header required"})
		return
	}

	jobUID := c.Param("job_uid")
	chunkIndexStr := c.Param("chunk_index")

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid chunk index"})
		return
	}

	var statusUpdate struct {
		Status string `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&statusUpdate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Find worker by hostname
	var worker models.Worker
	if err := database.DB.Where("hostname = ?", hostname).First(&worker).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Worker not found"})
		return
	}

	// Find and update the job chunk
	var jobChunk models.JobChunk
	if err := database.DB.Preload("Job").Where("worker_id = ? AND chunk_index = ?", worker.ID, chunkIndex).
		Joins("JOIN jobs ON job_chunks.job_id = jobs.id").
		Where("jobs.uid = ?", jobUID).First(&jobChunk).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job chunk not found"})
		return
	}

	// Update chunk status
	// Treat "exhausted" as "completed" since it means the work is done
	chunkStatus := statusUpdate.Status
	if chunkStatus == "exhausted" {
		chunkStatus = "completed"
	}
	
	jobChunk.Status = models.JobStatus(chunkStatus)
	if chunkStatus == "completed" || chunkStatus == "failed" || chunkStatus == "cancelled" {
		now := time.Now()
		jobChunk.FinishedAt = &now
	}

	if err := database.DB.Save(&jobChunk).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update chunk status"})
		return
	}

	// Update overall job status based on chunk statuses
	h.updateOverallJobStatus(jobChunk.JobID)

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// updateOverallJobStatus updates the job status based on its chunks
func (h *WorkerHandler) updateOverallJobStatus(jobID uint) {
	var chunks []models.JobChunk
	database.DB.Where("job_id = ?", jobID).Find(&chunks)
	
	if len(chunks) == 0 {
		return
	}
	
	// Count chunk statuses
	queued := 0
	running := 0
	completed := 0
	failed := 0
	cancelled := 0
	overloaded := 0
	
	for _, chunk := range chunks {
		switch chunk.Status {
		case models.JobStatusQueued:
			queued++
		case models.JobStatusRunning:
			running++
		case models.JobStatusCompleted:
			completed++
		case models.JobStatusFailed:
			failed++
		case models.JobStatusCancelled:
			cancelled++
		case models.JobStatusOverloaded:
			overloaded++
		}
	}
	
	// Determine overall job status
	var newStatus models.JobStatus
	if running > 0 {
		newStatus = models.JobStatusRunning
	} else if completed > 0 && (failed == 0 && cancelled == 0 && running == 0 && queued == 0 && overloaded == 0) {
		// All chunks completed successfully
		newStatus = models.JobStatusCompleted
	} else if completed > 0 && (failed > 0 || cancelled > 0) && running == 0 && queued == 0 && overloaded == 0 {
		// Some chunks completed, some failed/cancelled - consider partially completed
		newStatus = models.JobStatusCompleted
	} else if failed > 0 && completed == 0 && running == 0 && queued == 0 && overloaded == 0 {
		// All chunks failed
		newStatus = models.JobStatusFailed
	} else if cancelled > 0 && completed == 0 && failed == 0 && running == 0 && queued == 0 && overloaded == 0 {
		// All chunks cancelled
		newStatus = models.JobStatusCancelled
	} else {
		// Still have queued, overloaded chunks or mixed status
		newStatus = models.JobStatusQueued
	}
	
	// Update job status
	var job models.Job
	if err := database.DB.First(&job, jobID).Error; err != nil {
		return
	}
	
	if job.Status != newStatus {
		oldStatus := job.Status
		job.Status = newStatus
		if newStatus == models.JobStatusCompleted || newStatus == models.JobStatusFailed {
			now := time.Now()
			job.FinishedAt = &now
		}
		database.DB.Save(&job)

		// Send webhook for major status transitions
		switch newStatus {
		case models.JobStatusRunning:
			if oldStatus == models.JobStatusQueued {
				SendJobWebhook(&job, fmt.Sprintf("Job '%s' (%s) is now running", job.Name, job.UID))
			}
		case models.JobStatusCompleted:
			SendJobWebhook(&job, fmt.Sprintf("Job '%s' (%s) completed — %d/%d hashes cracked", job.Name, job.UID, job.CrackedCount, job.TotalHashes))
		case models.JobStatusFailed:
			SendJobWebhook(&job, fmt.Sprintf("Job '%s' (%s) failed", job.Name, job.UID))
		}

		// Clean up all job files when job reaches a final state
		if newStatus == models.JobStatusCompleted || newStatus == models.JobStatusFailed || newStatus == models.JobStatusCancelled {
			go h.cleanupAllJobFiles(&job)
		}
	}
}

// CleanupOrphanedFiles removes files in the upload directory that belong to jobs
// that no longer exist in the database. This handles crashes, bugs, or other
// situations that leave stale files behind.
func (h *WorkerHandler) CleanupOrphanedFiles() {
	log.Printf("Running orphaned file cleanup in %s", h.uploadPath)

	entries, err := os.ReadDir(h.uploadPath)
	if err != nil {
		log.Printf("Warning: Failed to read upload directory for orphan cleanup: %v", err)
		return
	}

	// Build a set of valid job UIDs from the database
	var jobs []models.Job
	database.DB.Select("uid").Find(&jobs)
	validUIDs := make(map[string]bool)
	for _, job := range jobs {
		validUIDs[job.UID] = true
	}

	removedCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Extract jobUID from known file patterns:
		// {jobUID}_chunk_{i}.wordlist
		// {jobUID}_wordlist_{filename}
		// {jobUID}.pot
		var uid string
		if idx := strings.Index(name, "_chunk_"); idx > 0 {
			uid = name[:idx]
		} else if idx := strings.Index(name, "_wordlist_"); idx > 0 {
			uid = name[:idx]
		} else if strings.HasSuffix(name, ".pot") {
			uid = strings.TrimSuffix(name, ".pot")
		} else {
			continue // Not a recognized job file pattern
		}

		if uid == "" || validUIDs[uid] {
			continue // Job still exists, keep the file
		}

		// Job doesn't exist in DB — orphaned file
		filePath := filepath.Join(h.uploadPath, name)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Failed to remove orphaned file %s: %v", filePath, err)
		} else if err == nil {
			removedCount++
		}
	}

	if removedCount > 0 {
		log.Printf("Orphaned file cleanup: removed %d files", removedCount)
	} else {
		log.Printf("Orphaned file cleanup: no orphaned files found")
	}
}

// cleanupAllJobFiles removes all server-side files for a finished job:
// wordlist chunks, custom uploaded wordlist, and potfile
func (h *WorkerHandler) cleanupAllJobFiles(job *models.Job) {
	log.Printf("Cleaning up all files for finished job %s (status: %s)", job.UID, job.Status)

	// 1. Clean up wordlist chunks
	splitter := wordlistpkg.NewWordlistSplitter(h.uploadPath)
	if err := splitter.CleanupChunks(job.UID); err != nil {
		log.Printf("Warning: Failed to cleanup wordlist chunks for job %s: %v", job.UID, err)
	}

	// 2. Clean up custom uploaded wordlist file
	if job.WordlistFile != "" {
		if err := os.Remove(job.WordlistFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Failed to remove custom wordlist for job %s: %v", job.UID, err)
		} else if err == nil {
			log.Printf("Cleaned up custom wordlist for job %s: %s", job.UID, job.WordlistFile)
		}
	}

	// 3. Clean up potfile if it exists
	potfilePath := filepath.Join(h.uploadPath, fmt.Sprintf("%s.pot", job.UID))
	if err := os.Remove(potfilePath); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: Failed to remove potfile for job %s: %v", job.UID, err)
	}

	log.Printf("File cleanup completed for job %s", job.UID)
}