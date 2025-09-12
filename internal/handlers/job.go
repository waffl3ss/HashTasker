package handlers

import (
	"encoding/json"
	"fmt"
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
	"github.com/google/uuid"
)

type JobHandler struct {
	uploadPath    string
	wordlistsPath string
	rulesetsPath  string
	hashmodesPath string
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

	if !u.IsAdmin {
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
		
		// Calculate average time remaining
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
		} else if activeWorkers == 0 && job.Status == "running" {
			progressText = "Preparing workers..."
		} else if activeWorkers == 0 {
			progressText = "No active workers"
		} else {
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

	// Handle hashes file upload
	var hashesFile string
	file, err := c.FormFile("hashes_file")
	if err == nil {
		filename := fmt.Sprintf("%s_%s", jobUID, file.Filename)
		hashesFile = filepath.Join(h.uploadPath, filename)

		if err := c.SaveUploadedFile(file, hashesFile); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded hashes file"})
			return
		}
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

	totalHashes := 0
	if hashesText != "" {
		totalHashes = len(strings.Split(strings.TrimSpace(hashesText), "\n"))
	} else if hashesFile != "" {
		if content, err := os.ReadFile(hashesFile); err == nil {
			totalHashes = len(strings.Split(strings.TrimSpace(string(content)), "\n"))
		}
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
		HashesFile:     hashesFile,
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
	
	// Get total count of cracked hashes
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

	// Clean up related data
	database.DB.Where("job_id = ?", job.ID).Delete(&models.JobProgress{})
	database.DB.Where("job_id = ?", job.ID).Delete(&models.CrackedHash{})
	database.DB.Where("job_id = ?", job.ID).Delete(&models.JobChunk{})

	// Clean up files
	if job.HashesFile != "" {
		os.Remove(job.HashesFile)
	}
	if job.WordlistFile != "" {
		os.Remove(job.WordlistFile)
	}

	// Clean up wordlist chunks
	splitter := wordlistpkg.NewWordlistSplitter(h.uploadPath)
	splitter.CleanupChunks(job.UID)

	if err := database.DB.Delete(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Job deleted successfully"})
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

	// Return job data for copying
	c.JSON(http.StatusOK, gin.H{
		"name":            job.Name + " - copy",
		"hash_mode":       job.HashMode,
		"wordlist":        job.Wordlist,
		"wordlist_file":   job.WordlistFile,
		"rulesets":        job.Rulesets,
		"disable_potfile": job.DisablePotfile,
		"hashes_text":     job.HashesText,
		"hashes_file":     job.HashesFile,
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
			Rules:    ruleCount,
			Modified: info.ModTime().Format("2006-01-02"),
		})
	}

	return rulesets, nil
}

func (h *JobHandler) countRulesInFile(filePath string) int {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(content), "\n")
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
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
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
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

func generateJobUID() string {
	id := uuid.New().String()
	return strings.ReplaceAll(id, "-", "")[:10]
}
