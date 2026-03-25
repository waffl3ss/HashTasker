package models

import (
	"time"
)

// AnalyticsSummary stores overall totals
type AnalyticsSummary struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TotalJobs    int64     `json:"total_jobs"`
	TotalHashes  int64     `json:"total_hashes"`
	TotalCracked int64     `json:"total_cracked"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AnalyticsJobsByPeriod stores job counts by time period
type AnalyticsJobsByPeriod struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Period    string    `json:"period" gorm:"index"`                            // "daily", "weekly", "monthly", "yearly"
	PeriodKey string    `json:"period_key" gorm:"uniqueIndex:idx_period_key"`   // e.g., "2025-01-15", "2025-W03", "2025-01", "2025"
	JobCount  int64     `json:"job_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AnalyticsWordlistUsage tracks wordlist usage
type AnalyticsWordlistUsage struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	WordlistName string    `json:"wordlist_name" gorm:"uniqueIndex"`
	UsageCount   int64     `json:"usage_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AnalyticsRulesetUsage tracks ruleset usage
type AnalyticsRulesetUsage struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	RulesetName string    `json:"ruleset_name" gorm:"uniqueIndex"`
	UsageCount  int64     `json:"usage_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AnalyticsJobsByUser tracks jobs per user
type AnalyticsJobsByUser struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `json:"user_id" gorm:"uniqueIndex"`
	Username  string    `json:"username"`
	JobCount  int64     `json:"job_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AnalyticsHashModeUsage tracks hash mode usage
type AnalyticsHashModeUsage struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	HashMode     int       `json:"hash_mode" gorm:"uniqueIndex"`
	HashModeName string    `json:"hash_mode_name"`
	UsageCount   int64     `json:"usage_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
