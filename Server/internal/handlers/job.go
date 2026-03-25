package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"hashtasker-go/internal/database"
	"hashtasker-go/internal/models"
	wordlistpkg "hashtasker-go/internal/wordlist"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type JobHandler struct {
	uploadPath    string
	wordlistsPath string
	rulesetsPath  string
	hashmodesPath string
}

// normalizeNewlines converts all newline variations (\r\n, \r, \n) to Unix-style \n
func normalizeNewlines(text string) string {
	// First replace Windows CRLF with LF
	text = strings.ReplaceAll(text, "\r\n", "\n")
	// Then replace any remaining Mac CR with LF
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

// splitLines splits text by normalized newlines and returns non-empty lines
func splitLines(text string) []string {
	normalized := normalizeNewlines(text)
	lines := strings.Split(normalized, "\n")

	// Filter out empty lines
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

// processHashContent processes hash content by stripping whitespace and rejoining with newlines
func processHashContent(content string) string {
	lines := splitLines(content)
	return strings.Join(lines, "\n")
}

func NewJobHandler(config models.ServerConfig) *JobHandler {
	os.MkdirAll(config.UploadPath, 0755)
	return &JobHandler{
		uploadPath:    config.UploadPath,
		wordlistsPath: config.WordlistsPath,
		rulesetsPath:  config.RulesetsPath,
		hashmodesPath: config.HashmodesPath,
	}
}

func (h *JobHandler) JobsPage(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	c.HTML(http.StatusOK, "base", gin.H{
		"title":    "Jobs - HashTasker",
		"user":     u,
		"template": "jobs",
	})
}

func (h *JobHandler) CreateJobPage(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	c.HTML(http.StatusOK, "base", gin.H{
		"title":    "Create Job - HashTasker",
		"user":     u,
		"template": "create_job",
	})
}

func (h *JobHandler) JobDetailsPage(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	jobUID := c.Param("uid")

	var job models.Job
	result := database.DB.Preload("User").Where("uid = ?", jobUID).First(&job)
	if result.Error != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{
			"title": "Job Not Found",
			"error": "Job not found",
		})
		return
	}

	if !u.IsAdmin && job.UserID != u.ID {
		c.HTML(http.StatusForbidden, "error.html", gin.H{
			"title": "Access Denied",
			"error": "You don't have permission to view this job",
		})
		return
	}

	c.HTML(http.StatusOK, "base", gin.H{
		"title":    fmt.Sprintf("Job %s - HashTasker", job.Name),
		"user":     u,
		"job":      job,
		"template": "job_details",
	})
}

func (h *JobHandler) GetJobs(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	var jobs []models.Job
	query := database.DB.Preload("User")

	// Default to current user's jobs only
	// Admins can pass ?show_all=true to see everyone's jobs
	showAll := c.Query("show_all") == "true" && u.IsAdmin
	if !showAll {
		query = query.Where("user_id = ?", u.ID)
	}

	if err := query.Order("created_at DESC").Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch jobs"})
		return
	}

	// Enrich jobs with progress data
	type JobWithProgress struct {
		models.Job
		WorkerProgress   string `json:"worker_progress"`
		ActiveWorkers    int    `json:"active_workers"`
		TotalWorkers     int    `json:"total_workers"`
		TimeRemaining    int64  `json:"time_remaining"`
		TimeRemainingStr string `json:"time_remaining_str"`
	}

	var enrichedJobs []JobWithProgress
	for _, job := range jobs {
		var progress []models.JobProgress
		database.DB.Where("job_id = ?", job.ID).Find(&progress)

		activeWorkers := 0
		totalWorkers := len(progress)
		var totalTimeRemaining int64 = 0

		for _, p := range progress {
			if p.Status == "running" {
				activeWorkers++
				if p.TimeRemaining > 0 {
					totalTimeRemaining += p.TimeRemaining
				}
			}
		}

		// Calculate average time remaining from active workers only
		var avgTimeRemaining int64 = 0
		var timeRemainingStr string
		if activeWorkers > 0 && totalTimeRemaining > 0 {
			avgTimeRemaining = totalTimeRemaining / int64(activeWorkers)
			timeRemainingStr = formatDuration(avgTimeRemaining)
		} else if job.Status == "completed" {
			timeRemainingStr = "Complete"
		} else if job.Status == "failed" {
			timeRemainingStr = "Failed"
		} else if job.Status == "cancelled" {
			timeRemainingStr = "Cancelled"
		} else if job.Status == "queued" {
			timeRemainingStr = "Queued"
		} else {
			timeRemainingStr = "Calculating..."
		}

		var progressText string
		if totalWorkers == 0 {
			progressText = "No workers assigned"
		} else {
			// Always show active worker count
			progressText = fmt.Sprintf("%d of %d workers active", activeWorkers, totalWorkers)
		}

		enrichedJobs = append(enrichedJobs, JobWithProgress{
			Job:              job,
			WorkerProgress:   progressText,
			ActiveWorkers:    activeWorkers,
			TotalWorkers:     totalWorkers,
			TimeRemaining:    avgTimeRemaining,
			TimeRemainingStr: timeRemainingStr,
		})
	}

	c.JSON(http.StatusOK, enrichedJobs)
}

// formatDuration formats seconds into a human readable duration
func formatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	} else if seconds < 3600 {
		return fmt.Sprintf("%dm", seconds/60)
	} else if seconds < 86400 {
		return fmt.Sprintf("%dh", seconds/3600)
	} else {
		return fmt.Sprintf("%dd", seconds/86400)
	}
}

func (h *JobHandler) CreateJob(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	name := c.PostForm("name")
	wordlist := c.PostForm("wordlist")
	rulesets := c.PostFormArray("rulesets") // Support multiple rulesets
	hashModeStr := c.PostForm("hash_mode")
	hashesText := c.PostForm("hashes_text")
	disablePotfileStr := c.PostForm("disable_potfile")
	usernameModeStr := c.PostForm("username_mode")
	webhookURL := strings.TrimSpace(c.PostForm("webhook_url"))

	if name == "" || hashModeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required fields"})
		return
	}

	// Either wordlist or custom wordlist file must be provided
	if wordlist == "" {
		if _, err := c.FormFile("wordlist_file"); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Either wordlist selection or custom wordlist file is required"})
			return
		}
	}

	hashMode, err := strconv.Atoi(hashModeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid hash mode"})
		return
	}

	disablePotfile := disablePotfileStr == "true" || disablePotfileStr == "on"

	jobUID := generateJobUID()

	// Handle hashes file upload - read and process into HashesText instead of saving as file
	file, err := c.FormFile("hashes_file")
	if err == nil {
		// Read uploaded file content
		fileContent, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read uploaded hashes file"})
			return
		}
		defer fileContent.Close()

		content, err := io.ReadAll(fileContent)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read uploaded hashes file content"})
			return
		}

		// Process the content to strip whitespace from each line and normalize newlines
		processedContent := processHashContent(string(content))

		// Set hashesText to the processed content - treat it the same as pasted hashes
		hashesText = processedContent
		log.Printf("Processed uploaded hash file: %d hashes", len(splitLines(processedContent)))
	}

	// Handle custom wordlist upload
	var wordlistFile string
	customWordlistFile, err := c.FormFile("wordlist_file")
	if err == nil {
		filename := fmt.Sprintf("%s_wordlist_%s", jobUID, customWordlistFile.Filename)
		wordlistFile = filepath.Join(h.uploadPath, filename)

		if err := c.SaveUploadedFile(customWordlistFile, wordlistFile); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded wordlist file"})
			return
		}
		// Use the uploaded file as the wordlist source
		wordlist = wordlistFile
	}

	// Process hashesText to normalize newlines and strip whitespace
	if hashesText != "" {
		hashesText = processHashContent(hashesText)
	}
	
	totalHashes := 0
	if hashesText != "" {
		totalHashes = len(splitLines(hashesText))
	}

	// Convert rulesets array to JSON string
	rulesetsJSON := "[]"
	if len(rulesets) > 0 {
		if data, err := json.Marshal(rulesets); err == nil {
			rulesetsJSON = string(data)
		}
	}

	// Get active workers to distribute the job
	var activeWorkers []models.Worker
	if err := database.DB.Where("is_online = ?", true).Find(&activeWorkers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get active workers"})
		return
	}

	if len(activeWorkers) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active workers available"})
		return
	}

	// Flag as NTDS if hash mode is 1000 (NTLM) and over 300 hashes
	isNTDS := hashMode == 1000 && totalHashes > 300

	job := models.Job{
		UID:            jobUID,
		Name:           name,
		UserID:         u.ID,
		Status:         models.JobStatusQueued,
		HashMode:       hashMode,
		Wordlist:       wordlist,
		WordlistFile:   wordlistFile,
		Rulesets:       rulesetsJSON,
		DisablePotfile: disablePotfile,
		UsernameMode:   usernameModeStr == "true" || usernameModeStr == "on",
		IsNTDS:         isNTDS,
		WebhookURL:     webhookURL,
		HashesFile:     "", // No longer storing hash files, all hashes go into HashesText
		HashesText:     hashesText,
		TotalHashes:    totalHashes,
	}

	// Create job in database first
	if err := database.DB.Create(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create job"})
		return
	}

	// Split wordlist across available workers
	splitter := wordlistpkg.NewWordlistSplitter(h.uploadPath)
	chunks, err := splitter.SplitWordlist(wordlist, jobUID, len(activeWorkers))
	if err != nil {
		// Clean up job if wordlist splitting fails
		database.DB.Delete(&job)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to split wordlist: %v", err)})
		return
	}

	// Create job chunks for each worker
	for i, chunk := range chunks {
		if i >= len(activeWorkers) {
			break // Safety check
		}

		jobChunk := models.JobChunk{
			JobID:      job.ID,
			WorkerID:   activeWorkers[i].ID,
			ChunkIndex: chunk.ChunkIndex,
			ChunkPath:  chunk.FilePath,
			StartLine:  chunk.StartLine,
			EndLine:    chunk.EndLine,
			Status:     models.JobStatusQueued,
		}

		if err := database.DB.Create(&jobChunk).Error; err != nil {
			// Clean up on error
			database.DB.Where("job_id = ?", job.ID).Delete(&models.JobChunk{})
			database.DB.Delete(&job)
			splitter.CleanupChunks(jobUID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create job chunks"})
			return
		}
	}

	// Record analytics for job creation
	hashModeName := fmt.Sprintf("Mode %d", hashMode)
	hashModes, err := h.loadHashModes()
	if err == nil {
		for _, hm := range hashModes {
			if hm.Mode == hashMode {
				hashModeName = hm.Name
				break
			}
		}
	}
	RecordJobCreation(&job, u, hashModeName)

	SendJobWebhook(&job, fmt.Sprintf("Job '%s' (%s) created — %d hashes, %d workers assigned", job.Name, job.UID, totalHashes, len(activeWorkers)))

	c.JSON(http.StatusOK, gin.H{
		"message":      "Job created successfully",
		"job_uid":      jobUID,
		"workers_used": len(activeWorkers),
		"chunks":       len(chunks),
	})
}

func (h *JobHandler) GetJobDetails(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	jobUID := c.Param("uid")

	var job models.Job
	result := database.DB.Preload("User").Where("uid = ?", jobUID).First(&job)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if !u.IsAdmin && job.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var progress []models.JobProgress
	database.DB.Where("job_id = ?", job.ID).Find(&progress)

	// Sort progress by worker name alphabetically
	sort.Slice(progress, func(i, j int) bool {
		return progress[i].WorkerName < progress[j].WorkerName
	})

	// Get pagination parameters
	page := 1
	pageSize := 50 // Default page size
	
	if pageParam := c.Query("page"); pageParam != "" {
		if p, err := strconv.Atoi(pageParam); err == nil && p > 0 {
			page = p
		}
	}
	
	if sizeParam := c.Query("page_size"); sizeParam != "" {
		if size, err := strconv.Atoi(sizeParam); err == nil && size > 0 && size <= 200 {
			pageSize = size
		}
	}
	
	// Get total count of cracked hashes (unique constraint ensures no duplicates)
	var totalCrackedHashes int64
	database.DB.Model(&models.CrackedHash{}).Where("job_id = ?", job.ID).Count(&totalCrackedHashes)

	// Get paginated cracked hashes
	offset := (page - 1) * pageSize
	var crackedHashes []models.CrackedHash
	database.DB.Where("job_id = ?", job.ID).
		Order("cracked_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&crackedHashes)
	
	// Calculate pagination info
	totalPages := int((totalCrackedHashes + int64(pageSize) - 1) / int64(pageSize))

	c.JSON(http.StatusOK, gin.H{
		"job":      job,
		"progress": progress,
		"cracked_hashes": gin.H{
			"data":         crackedHashes,
			"current_page": page,
			"page_size":    pageSize,
			"total_count":  totalCrackedHashes,
			"total_pages":  totalPages,
		},
	})
}

func (h *JobHandler) GetJobCrackedHashes(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	jobUID := c.Param("uid")

	var job models.Job
	result := database.DB.Preload("User").Where("uid = ?", jobUID).First(&job)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if !u.IsAdmin && job.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Get pagination parameters
	page := 1
	pageSize := 50 // Default page size
	
	if pageParam := c.Query("page"); pageParam != "" {
		if p, err := strconv.Atoi(pageParam); err == nil && p > 0 {
			page = p
		}
	}
	
	if sizeParam := c.Query("page_size"); sizeParam != "" {
		if size, err := strconv.Atoi(sizeParam); err == nil && size > 0 && size <= 200 {
			pageSize = size
		}
	}

	// Get search parameter
	search := c.Query("search")

	// Build query
	query := database.DB.Model(&models.CrackedHash{}).Where("job_id = ?", job.ID)
	
	// Add search filter if provided
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("hash LIKE ? OR plaintext LIKE ?", searchPattern, searchPattern)
	}
	
	// Get total count
	var totalCount int64
	query.Count(&totalCount)
	
	// Get paginated cracked hashes
	offset := (page - 1) * pageSize
	var crackedHashes []models.CrackedHash
	if err := query.Order("cracked_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&crackedHashes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get cracked hashes"})
		return
	}
	
	// Calculate pagination info
	totalPages := int((totalCount + int64(pageSize) - 1) / int64(pageSize))

	c.JSON(http.StatusOK, gin.H{
		"hashes":       crackedHashes,
		"current_page": page,
		"page_size":    pageSize,
		"total":        totalCount,
		"total_pages":  totalPages,
	})
}

func (h *JobHandler) CancelJob(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	jobUID := c.Param("uid")

	var job models.Job
	result := database.DB.Where("uid = ?", jobUID).First(&job)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if !u.IsAdmin && job.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	if job.Status == models.JobStatusCompleted || job.Status == models.JobStatusCancelled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Job cannot be cancelled"})
		return
	}

	job.Status = models.JobStatusCancelled
	now := time.Now()
	job.FinishedAt = &now

	if err := database.DB.Save(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel job"})
		return
	}

	SendJobWebhook(&job, fmt.Sprintf("Job '%s' (%s) was cancelled", job.Name, job.UID))

	// Cancel all job chunks that are not yet completed
	database.DB.Model(&models.JobChunk{}).
		Where("job_id = ? AND status IN (?)", job.ID, []models.JobStatus{
			models.JobStatusQueued, 
			models.JobStatusRunning,
		}).
		Updates(map[string]interface{}{
			"status":      models.JobStatusCancelled,
			"finished_at": &now,
		})

	// Cancel all job progress records that are still running
	database.DB.Model(&models.JobProgress{}).
		Where("job_id = ? AND status IN (?)", job.ID, []string{"running", "queued"}).
		Updates(map[string]interface{}{
			"status":      "cancelled",
			"finished_at": &now,
		})

	// Clean up all job files when cancelled
	splitter := wordlistpkg.NewWordlistSplitter(h.uploadPath)
	go func() {
		// Clean up wordlist chunks
		if err := splitter.CleanupChunks(job.UID); err != nil {
			log.Printf("Warning: Failed to cleanup wordlist chunks for cancelled job %s: %v", job.UID, err)
		}

		// Clean up custom uploaded wordlist
		if job.WordlistFile != "" {
			if err := os.Remove(job.WordlistFile); err != nil && !os.IsNotExist(err) {
				log.Printf("Warning: Failed to remove custom wordlist for cancelled job %s: %v", job.UID, err)
			}
		}

		// Clean up potfile
		potfilePath := filepath.Join(h.uploadPath, fmt.Sprintf("%s.pot", job.UID))
		os.Remove(potfilePath)

		log.Printf("File cleanup completed for cancelled job %s", job.UID)
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Job cancelled successfully"})
}

func (h *JobHandler) DeleteJob(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	jobUID := c.Param("uid")

	var job models.Job
	result := database.DB.Where("uid = ?", jobUID).First(&job)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if !u.IsAdmin && job.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	if err := h.purgeJob(&job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Job deleted successfully"})
}

// purgeJob fully removes a job: all related DB records and all files on disk.
// Used by both manual delete and auto-delete.
func (h *JobHandler) purgeJob(job *models.Job) error {
	// Clean up related data
	database.DB.Where("job_id = ?", job.ID).Delete(&models.JobProgress{})
	database.DB.Where("job_id = ?", job.ID).Delete(&models.CrackedHash{})
	database.DB.Where("job_id = ?", job.ID).Delete(&models.JobChunk{})

	// Clean up files
	if job.WordlistFile != "" {
		os.Remove(job.WordlistFile)
	}

	// Clean up generated potfile if it exists
	potfilePath := filepath.Join(h.uploadPath, fmt.Sprintf("%s.pot", job.UID))
	os.Remove(potfilePath)

	// Clean up wordlist chunks
	splitter := wordlistpkg.NewWordlistSplitter(h.uploadPath)
	splitter.CleanupChunks(job.UID)

	if err := database.DB.Delete(job).Error; err != nil {
		return err
	}

	return nil
}

// AutoDeleteOldJobs deletes all jobs older than 45 days along with all
// associated DB records and files. This mirrors the manual delete behavior.
func (h *JobHandler) AutoDeleteOldJobs() {
	cutoff := time.Now().AddDate(0, 0, -45)

	var oldJobs []models.Job
	if err := database.DB.Where("created_at < ?", cutoff).Find(&oldJobs).Error; err != nil {
		log.Printf("Auto-delete: failed to query old jobs: %v", err)
		return
	}

	if len(oldJobs) == 0 {
		log.Printf("Auto-delete: no jobs older than 45 days")
		return
	}

	log.Printf("Auto-delete: found %d jobs older than 45 days, deleting...", len(oldJobs))

	deleted := 0
	for i := range oldJobs {
		job := &oldJobs[i]
		if err := h.purgeJob(job); err != nil {
			log.Printf("Auto-delete: failed to delete job %s (%s): %v", job.UID, job.Name, err)
			continue
		}
		log.Printf("Auto-delete: deleted job %s (%s, created %s)", job.UID, job.Name, job.CreatedAt.Format("2006-01-02"))
		deleted++
	}

	log.Printf("Auto-delete: completed, deleted %d/%d jobs", deleted, len(oldJobs))
}

func (h *JobHandler) CopyJob(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	jobUID := c.Param("uid")

	var job models.Job
	result := database.DB.Where("uid = ?", jobUID).First(&job)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if !u.IsAdmin && job.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Process hashes_text to strip whitespace before copying
	processedHashesText := ""
	if job.HashesText != "" {
		processedHashesText = processHashContent(job.HashesText)
	}

	// Return job data for copying
	c.JSON(http.StatusOK, gin.H{
		"name":            job.Name + " - copy",
		"hash_mode":       job.HashMode,
		"wordlist":        job.Wordlist,
		"wordlist_file":   job.WordlistFile,
		"rulesets":        job.Rulesets,
		"disable_potfile": job.DisablePotfile,
		"username_mode":   job.UsernameMode,
		"hashes_text":     processedHashesText,
		"webhook_url":     job.WebhookURL,
	})
}

func (h *JobHandler) GetWordlists(c *gin.Context) {
	wordlists, err := h.loadWordlists()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load wordlists"})
		return
	}

	c.JSON(http.StatusOK, wordlists)
}

func (h *JobHandler) GetRulesets(c *gin.Context) {
	rulesets, err := h.loadRulesets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load rulesets"})
		return
	}

	c.JSON(http.StatusOK, rulesets)
}

func (h *JobHandler) GetHashModes(c *gin.Context) {
	hashModes, err := h.loadHashModes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load hash modes"})
		return
	}

	c.JSON(http.StatusOK, hashModes)
}

func (h *JobHandler) loadWordlists() ([]models.WordlistOption, error) {
	// Check if wordlists directory exists
	if _, err := os.Stat(h.wordlistsPath); os.IsNotExist(err) {
		return []models.WordlistOption{}, nil
	}

	files, err := os.ReadDir(h.wordlistsPath)
	if err != nil {
		return []models.WordlistOption{}, err
	}

	var wordlists []models.WordlistOption
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Common wordlist file extensions
		name := file.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".txt" && ext != ".wordlist" && ext != ".lst" && ext != "" {
			continue
		}

		fullPath := filepath.Join(h.wordlistsPath, name)
		info, err := file.Info()
		if err != nil {
			continue
		}

		wordlists = append(wordlists, models.WordlistOption{
			Name:     name,
			Path:     fullPath,
			Size:     info.Size(),
			Modified: info.ModTime().Format("2006-01-02"),
		})
	}

	// Sort by size descending (largest first)
	sort.Slice(wordlists, func(i, j int) bool {
		return wordlists[i].Size > wordlists[j].Size
	})

	return wordlists, nil
}

func (h *JobHandler) loadRulesets() ([]models.RulesetOption, error) {
	// Check if rulesets directory exists
	if _, err := os.Stat(h.rulesetsPath); os.IsNotExist(err) {
		return []models.RulesetOption{}, nil
	}

	files, err := os.ReadDir(h.rulesetsPath)
	if err != nil {
		return []models.RulesetOption{}, err
	}

	var rulesets []models.RulesetOption
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Common rule file extensions
		name := file.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".rule" && ext != ".rules" && ext != ".txt" && ext != "" {
			continue
		}

		fullPath := filepath.Join(h.rulesetsPath, name)
		info, err := file.Info()
		if err != nil {
			continue
		}

		// Count rules in file (lines that don't start with # and aren't empty)
		ruleCount := h.countRulesInFile(fullPath)

		rulesets = append(rulesets, models.RulesetOption{
			Name:     name,
			Path:     fullPath,
			Size:     info.Size(),
			Rules:    ruleCount,
			Modified: info.ModTime().Format("2006-01-02"),
		})
	}

	// Sort by size descending (largest first)
	sort.Slice(rulesets, func(i, j int) bool {
		return rulesets[i].Size > rulesets[j].Size
	})

	return rulesets, nil
}

func (h *JobHandler) countRulesInFile(filePath string) int {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}

	lines := splitLines(string(content))
	count := 0
	for _, line := range lines {
		if !strings.HasPrefix(line, "#") {
			count++
		}
	}

	return count
}

func (h *JobHandler) loadHashModes() ([]models.HashModeOption, error) {
	// Fallback hash modes if file doesn't exist or fails to load
	fallbackHashModes := []models.HashModeOption{
		{Mode: 0, Name: "MD5", Category: "Raw Hash", Description: "MD5"},
		{Mode: 100, Name: "SHA1", Category: "Raw Hash", Description: "SHA1"},
		{Mode: 1400, Name: "SHA256", Category: "Raw Hash", Description: "SHA2-256"},
		{Mode: 1700, Name: "SHA512", Category: "Raw Hash", Description: "SHA2-512"},
		{Mode: 1000, Name: "NTLM", Category: "Operating System", Description: "NTLM"},
		{Mode: 3200, Name: "bcrypt", Category: "Generic KDF", Description: "bcrypt $2*$, Blowfish (Unix)"},
		{Mode: 1800, Name: "sha512crypt", Category: "Operating System", Description: "sha512crypt $6$, SHA512 (Unix)"},
	}

	// Check if hash modes file exists
	if _, err := os.Stat(h.hashmodesPath); os.IsNotExist(err) {
		return fallbackHashModes, nil
	}

	// Read the hash modes file
	content, err := os.ReadFile(h.hashmodesPath)
	if err != nil {
		return fallbackHashModes, nil
	}

	var hashModes []models.HashModeOption
	lines := splitLines(string(content))

	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			continue // Skip comments
		}

		// Split on first colon only - everything after first colon is the name
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		modeStr := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])

		mode, err := strconv.Atoi(modeStr)
		if err != nil {
			continue // Skip lines with invalid mode numbers
		}

		hashModes = append(hashModes, models.HashModeOption{
			Mode:        mode,
			Name:        name,
			Category:    "Hash Mode",
			Description: name,
		})
	}

	// Return fallback if no valid modes were parsed
	if len(hashModes) == 0 {
		return fallbackHashModes, nil
	}

	return hashModes, nil
}

func (h *JobHandler) ExportCrackedHashesCSV(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	jobUID := c.Param("uid")

	var job models.Job
	result := database.DB.Where("uid = ?", jobUID).First(&job)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if !u.IsAdmin && job.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Get all cracked hashes
	var crackedHashes []models.CrackedHash
	database.DB.Where("job_id = ?", job.ID).Order("cracked_at ASC").Find(&crackedHashes)

	// Set headers for CSV download
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_cracked.csv\"", job.UID))

	// Write CSV header
	c.Writer.Write([]byte("Hash,Password,Recovery Time\n"))

	// Write cracked hashes
	for _, hash := range crackedHashes {
		// Escape CSV fields that contain commas or quotes
		hashField := escapeCsvField(hash.Hash)
		plaintextField := escapeCsvField(hash.Plaintext)
		recoveryTime := fmt.Sprintf("%d", hash.RecoveryTime)

		c.Writer.Write([]byte(fmt.Sprintf("%s,%s,%s\n", hashField, plaintextField, recoveryTime)))
	}
}

func (h *JobHandler) ExportAllHashesCSV(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	jobUID := c.Param("uid")

	var job models.Job
	result := database.DB.Where("uid = ?", jobUID).First(&job)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if !u.IsAdmin && job.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Get all cracked hashes
	var crackedHashes []models.CrackedHash
	database.DB.Where("job_id = ?", job.ID).Order("cracked_at ASC").Find(&crackedHashes)

	// Create a map for quick lookup of cracked hashes
	crackedMap := make(map[string]models.CrackedHash)
	for _, hash := range crackedHashes {
		crackedMap[hash.Hash] = hash
	}

	// Set headers for CSV download
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_all.csv\"", job.UID))

	// Write CSV header
	c.Writer.Write([]byte("Hash,Password,Recovery Time\n"))

	// Get all hashes from job
	if job.HashesText != "" {
		hashes := splitLines(job.HashesText)
		for _, hash := range hashes {
			hash = strings.TrimSpace(hash)
			if hash == "" {
				continue
			}

			if crackedHash, found := crackedMap[hash]; found {
				// Hash was cracked
				hashField := escapeCsvField(crackedHash.Hash)
				plaintextField := escapeCsvField(crackedHash.Plaintext)
				recoveryTime := fmt.Sprintf("%d", crackedHash.RecoveryTime)
				c.Writer.Write([]byte(fmt.Sprintf("%s,%s,%s\n", hashField, plaintextField, recoveryTime)))
			} else {
				// Hash not cracked
				hashField := escapeCsvField(hash)
				c.Writer.Write([]byte(fmt.Sprintf("%s,,\n", hashField)))
			}
		}
	}
}

func (h *JobHandler) ExportPotfile(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	jobUID := c.Param("uid")

	var job models.Job
	result := database.DB.Where("uid = ?", jobUID).First(&job)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if !u.IsAdmin && job.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Only allow potfile export for NTLM (mode 1000)
	if job.HashMode != 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Potfile export is only available for NTLM (mode 1000) jobs"})
		return
	}

	// Get all cracked hashes
	var crackedHashes []models.CrackedHash
	database.DB.Where("job_id = ?", job.ID).Order("cracked_at ASC").Find(&crackedHashes)

	// Generate potfile in uploads directory
	potfilePath := filepath.Join(h.uploadPath, fmt.Sprintf("%s.pot", job.UID))
	potfile, err := os.Create(potfilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create potfile"})
		return
	}
	defer potfile.Close()

	// Write cracked hashes in hash:password format
	for _, hash := range crackedHashes {
		potfile.WriteString(fmt.Sprintf("%s:%s\n", hash.Hash, hash.Plaintext))
	}

	// Set headers for potfile download
	c.Header("Content-Type", "text/plain")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.pot\"", job.UID))

	// Serve the file
	c.File(potfilePath)
}

// escapeCsvField escapes CSV fields containing commas, quotes, or newlines
func escapeCsvField(field string) string {
	// If field contains comma, quote, or newline, wrap in quotes and escape internal quotes
	if strings.ContainsAny(field, ",\"\n\r") {
		field = strings.ReplaceAll(field, "\"", "\"\"")
		return "\"" + field + "\""
	}
	return field
}

func generateJobUID() string {
	id := uuid.New().String()
	return strings.ReplaceAll(id, "-", "")[:10]
}
