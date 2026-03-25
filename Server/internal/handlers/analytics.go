package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"hashtasker-go/internal/database"
	"hashtasker-go/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AnalyticsHandler struct{}

func NewAnalyticsHandler() *AnalyticsHandler {
	return &AnalyticsHandler{}
}

// AnalyticsPage renders the analytics page (admin only)
func (h *AnalyticsHandler) AnalyticsPage(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	if !u.IsAdmin {
		c.HTML(http.StatusForbidden, "error.html", gin.H{
			"title": "Access Denied",
			"error": "You don't have permission to view this page",
		})
		return
	}

	c.HTML(http.StatusOK, "base", gin.H{
		"title":    "Analytics - HashTasker",
		"user":     u,
		"template": "analytics",
	})
}

// GetAnalytics returns analytics data computed from raw job data.
// Supports ?exclude_ntds=true (default) to filter out NTDS jobs.
func (h *AnalyticsHandler) GetAnalytics(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	if !u.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Default to excluding NTDS jobs
	excludeNTDS := c.Query("exclude_ntds") != "false"

	// Base query scope for filtering
	jobScope := func(db *gorm.DB) *gorm.DB {
		if excludeNTDS {
			return db.Where("is_ntds = ?", false)
		}
		return db
	}

	// --- Summary ---
	var totalJobs int64
	var totalHashes, totalCracked int64

	jobScope(database.DB.Model(&models.Job{})).Count(&totalJobs)

	// Use struct to scan aggregate results
	type SumResult struct {
		TotalHashes  int64
		TotalCracked int64
	}
	var sums SumResult
	jobScope(database.DB.Model(&models.Job{})).
		Select("COALESCE(SUM(total_hashes), 0) as total_hashes, COALESCE(SUM(cracked_count), 0) as total_cracked").
		Scan(&sums)
	totalHashes = sums.TotalHashes
	totalCracked = sums.TotalCracked

	summary := gin.H{
		"total_jobs":    totalJobs,
		"total_hashes":  totalHashes,
		"total_cracked": totalCracked,
	}

	// --- Jobs by period (computed from raw job created_at) ---
	var jobs []models.Job
	jobScope(database.DB.Model(&models.Job{})).Select("created_at").Find(&jobs)

	dailyCounts := map[string]int64{}
	weeklyCounts := map[string]int64{}
	monthlyCounts := map[string]int64{}
	yearlyCounts := map[string]int64{}

	for _, job := range jobs {
		t := job.CreatedAt.UTC()
		dailyCounts[t.Format("2006-01-02")]++
		weeklyCounts[t.Format("2006-W")+getWeekNumber(t)]++
		monthlyCounts[t.Format("2006-01")]++
		yearlyCounts[t.Format("2006")]++
	}

	jobsByDay := mapToSortedPeriods("daily", dailyCounts, 30)
	jobsByWeek := mapToSortedPeriods("weekly", weeklyCounts, 12)
	jobsByMonth := mapToSortedPeriods("monthly", monthlyCounts, 12)
	jobsByYear := mapToSortedPeriods("yearly", yearlyCounts, 5)

	// --- Wordlist usage ---
	type UsageRow struct {
		Name  string
		Count int64
	}

	var wordlistRows []UsageRow
	jobScope(database.DB.Model(&models.Job{})).
		Select("wordlist as name, COUNT(*) as count").
		Group("wordlist").
		Order("count DESC").
		Limit(20).
		Find(&wordlistRows)

	var wordlistUsage []gin.H
	for _, row := range wordlistRows {
		wordlistUsage = append(wordlistUsage, gin.H{
			"wordlist_name": getWordlistDisplayName(row.Name),
			"usage_count":   row.Count,
		})
	}

	// --- Ruleset usage (need to unmarshal JSON arrays from each job) ---
	var jobsWithRulesets []models.Job
	jobScope(database.DB.Model(&models.Job{})).
		Select("rulesets").
		Where("rulesets != '' AND rulesets != '[]'").
		Find(&jobsWithRulesets)

	rulesetCounts := map[string]int64{}
	for _, job := range jobsWithRulesets {
		var rulesets []string
		if err := json.Unmarshal([]byte(job.Rulesets), &rulesets); err == nil {
			for _, ruleset := range rulesets {
				if ruleset != "" {
					name := getRulesetDisplayName(ruleset)
					rulesetCounts[name]++
				}
			}
		}
	}

	var rulesetUsage []gin.H
	for name, count := range rulesetCounts {
		rulesetUsage = append(rulesetUsage, gin.H{
			"ruleset_name": name,
			"usage_count":  count,
		})
	}
	// Sort by count descending
	sort.Slice(rulesetUsage, func(i, j int) bool {
		return rulesetUsage[i]["usage_count"].(int64) > rulesetUsage[j]["usage_count"].(int64)
	})
	if len(rulesetUsage) > 20 {
		rulesetUsage = rulesetUsage[:20]
	}

	// --- Jobs by user ---
	type UserJobRow struct {
		UserID   uint
		Username string
		Count    int64
	}
	var userRows []UserJobRow
	jobScope(database.DB.Model(&models.Job{})).
		Select("jobs.user_id, users.username, COUNT(*) as count").
		Joins("JOIN users ON users.id = jobs.user_id").
		Group("jobs.user_id, users.username").
		Order("count DESC").
		Limit(20).
		Find(&userRows)

	var jobsByUser []gin.H
	for _, row := range userRows {
		jobsByUser = append(jobsByUser, gin.H{
			"user_id":   row.UserID,
			"username":  row.Username,
			"job_count": row.Count,
		})
	}

	// --- Hash mode usage ---
	type HashModeRow struct {
		HashMode int
		Count    int64
	}
	var hashModeRows []HashModeRow
	jobScope(database.DB.Model(&models.Job{})).
		Select("hash_mode, COUNT(*) as count").
		Group("hash_mode").
		Order("count DESC").
		Limit(20).
		Find(&hashModeRows)

	var hashModeUsage []gin.H
	for _, row := range hashModeRows {
		hashModeUsage = append(hashModeUsage, gin.H{
			"hash_mode":      row.HashMode,
			"hash_mode_name": fmt.Sprintf("Mode %d", row.HashMode),
			"usage_count":    row.Count,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"summary":       summary,
		"exclude_ntds":  excludeNTDS,
		"jobs_by_period": gin.H{
			"daily":   jobsByDay,
			"weekly":  jobsByWeek,
			"monthly": jobsByMonth,
			"yearly":  jobsByYear,
		},
		"wordlist_usage":  wordlistUsage,
		"ruleset_usage":   rulesetUsage,
		"jobs_by_user":    jobsByUser,
		"hash_mode_usage": hashModeUsage,
	})
}

// mapToSortedPeriods converts a map of period_key->count into a sorted slice, most recent first
func mapToSortedPeriods(period string, counts map[string]int64, limit int) []gin.H {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	if len(keys) > limit {
		keys = keys[:limit]
	}

	result := make([]gin.H, len(keys))
	for i, key := range keys {
		result[i] = gin.H{
			"period":     period,
			"period_key": key,
			"job_count":  counts[key],
		}
	}
	return result
}

// RecordJobCreation updates analytics when a new job is created
func RecordJobCreation(job *models.Job, user *models.User, hashModeName string) {
	now := time.Now().UTC()

	// Update summary totals
	database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"total_jobs":   gorm.Expr("total_jobs + 1"),
			"total_hashes": gorm.Expr("total_hashes + ?", job.TotalHashes),
			"updated_at":   now,
		}),
	}).Create(&models.AnalyticsSummary{
		ID:          1,
		TotalJobs:   1,
		TotalHashes: int64(job.TotalHashes),
		UpdatedAt:   now,
	})

	// Update jobs by period (daily, weekly, monthly, yearly)
	periods := map[string]string{
		"daily":   now.Format("2006-01-02"),
		"weekly":  now.Format("2006-W") + getWeekNumber(now),
		"monthly": now.Format("2006-01"),
		"yearly":  now.Format("2006"),
	}

	for period, periodKey := range periods {
		database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "period_key"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"job_count":  gorm.Expr("job_count + 1"),
				"updated_at": now,
			}),
		}).Create(&models.AnalyticsJobsByPeriod{
			Period:    period,
			PeriodKey: periodKey,
			JobCount:  1,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	// Update wordlist usage
	wordlistName := getWordlistDisplayName(job.Wordlist)
	database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "wordlist_name"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"usage_count": gorm.Expr("usage_count + 1"),
			"updated_at":  now,
		}),
	}).Create(&models.AnalyticsWordlistUsage{
		WordlistName: wordlistName,
		UsageCount:   1,
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	// Update ruleset usage
	if job.Rulesets != "" && job.Rulesets != "[]" {
		var rulesets []string
		if err := json.Unmarshal([]byte(job.Rulesets), &rulesets); err == nil {
			for _, ruleset := range rulesets {
				if ruleset == "" {
					continue
				}
				rulesetName := getRulesetDisplayName(ruleset)
				database.DB.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "ruleset_name"}},
					DoUpdates: clause.Assignments(map[string]interface{}{
						"usage_count": gorm.Expr("usage_count + 1"),
						"updated_at":  now,
					}),
				}).Create(&models.AnalyticsRulesetUsage{
					RulesetName: rulesetName,
					UsageCount:  1,
					CreatedAt:   now,
					UpdatedAt:   now,
				})
			}
		}
	}

	// Update jobs by user
	database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"username":   user.Username,
			"job_count":  gorm.Expr("job_count + 1"),
			"updated_at": now,
		}),
	}).Create(&models.AnalyticsJobsByUser{
		UserID:    user.ID,
		Username:  user.Username,
		JobCount:  1,
		CreatedAt: now,
		UpdatedAt: now,
	})

	// Update hash mode usage
	database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "hash_mode"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"hash_mode_name": hashModeName,
			"usage_count":    gorm.Expr("usage_count + 1"),
			"updated_at":     now,
		}),
	}).Create(&models.AnalyticsHashModeUsage{
		HashMode:     job.HashMode,
		HashModeName: hashModeName,
		UsageCount:   1,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
}

// ClearAnalytics deletes all analytics data (admin only)
func (h *AnalyticsHandler) ClearAnalytics(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	if !u.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Delete all analytics data
	database.DB.Exec("DELETE FROM analytics_summaries")
	database.DB.Exec("DELETE FROM analytics_jobs_by_periods")
	database.DB.Exec("DELETE FROM analytics_wordlist_usages")
	database.DB.Exec("DELETE FROM analytics_ruleset_usages")
	database.DB.Exec("DELETE FROM analytics_jobs_by_users")
	database.DB.Exec("DELETE FROM analytics_hash_mode_usages")

	c.JSON(http.StatusOK, gin.H{"message": "Analytics data cleared successfully"})
}

// RecordCrackedHashes updates the total cracked count
func RecordCrackedHashes(count int) {
	if count <= 0 {
		return
	}

	now := time.Now().UTC()
	database.DB.Model(&models.AnalyticsSummary{}).Where("id = ?", 1).Updates(map[string]interface{}{
		"total_cracked": gorm.Expr("total_cracked + ?", count),
		"updated_at":    now,
	})
}

// Helper functions

func getWeekNumber(t time.Time) string {
	_, week := t.ISOWeek()
	return fmt.Sprintf("%02d", week)
}

func getWordlistDisplayName(wordlistPath string) string {
	if wordlistPath == "" {
		return "None"
	}

	// Check if it's a custom uploaded wordlist (contains job UID pattern)
	filename := filepath.Base(wordlistPath)
	if strings.Contains(filename, "_wordlist_") {
		return "Custom"
	}

	return filename
}

func getRulesetDisplayName(rulesetPath string) string {
	if rulesetPath == "" {
		return "None"
	}

	// Custom rulesets would be in uploads directory - but rulesets aren't uploaded currently
	// Just return the filename
	return filepath.Base(rulesetPath)
}
