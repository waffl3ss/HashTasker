package models

import (
	"time"

	"gorm.io/gorm"
)

type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusRunning    JobStatus = "running"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusCancelled  JobStatus = "cancelled"
	JobStatusFailed     JobStatus = "failed"
	JobStatusOverloaded JobStatus = "overloaded"
)

type Job struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	UID            string         `gorm:"unique;not null;size:10" json:"uid"`
	Name           string         `gorm:"not null" json:"name"`
	UserID         uint           `gorm:"not null;index" json:"user_id"`
	User           User           `gorm:"foreignKey:UserID" json:"user"`
	Status         JobStatus      `gorm:"not null;default:'queued'" json:"status"`
	HashMode       int            `gorm:"not null" json:"hash_mode"`
	Wordlist       string         `gorm:"not null" json:"wordlist"`
	WordlistFile   string         `json:"wordlist_file"` // For custom uploaded wordlists
	Rulesets       string         `gorm:"type:text" json:"rulesets"` // JSON array of ruleset paths
	DisablePotfile bool           `gorm:"default:false" json:"disable_potfile"`
	HashesFile     string         `json:"hashes_file"`
	HashesText     string         `gorm:"type:longtext" json:"hashes_text"`
	TotalHashes    int            `gorm:"default:0" json:"total_hashes"`
	CrackedCount   int            `gorm:"default:0" json:"cracked_count"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	FinishedAt     *time.Time     `json:"finished_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

type JobProgress struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	JobID         uint       `gorm:"not null;index" json:"job_id"`
	Job           Job        `gorm:"foreignKey:JobID" json:"job"`
	WorkerName    string     `gorm:"not null" json:"worker_name"`
	Progress      float64    `gorm:"default:0" json:"progress"`
	Status        string     `gorm:"not null;default:'queued'" json:"status"`
	KeyspaceSize  int64      `gorm:"default:0" json:"keyspace_size"`
	TimeElapsed   int64      `gorm:"default:0" json:"time_elapsed"`
	TimeRemaining int64      `gorm:"default:0" json:"time_remaining"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	FinishedAt    *time.Time `json:"finished_at"`
}

type CrackedHash struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	JobID        uint      `gorm:"not null;index" json:"job_id"`
	Job          Job       `gorm:"foreignKey:JobID" json:"job"`
	Hash         string    `gorm:"not null" json:"hash"`
	Plaintext    string    `gorm:"not null" json:"plaintext"`
	RecoveryTime int64     `gorm:"default:0" json:"recovery_time"`
	CrackedAt    time.Time `json:"cracked_at"`
}

type JobChunk struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	JobID       uint      `gorm:"not null;index" json:"job_id"`
	Job         Job       `gorm:"foreignKey:JobID" json:"job"`
	WorkerID    uint      `gorm:"not null;index" json:"worker_id"`
	Worker      Worker    `gorm:"foreignKey:WorkerID" json:"worker"`
	ChunkIndex  int       `gorm:"not null" json:"chunk_index"`
	ChunkPath   string    `gorm:"not null" json:"chunk_path"`
	StartLine   int64     `gorm:"not null" json:"start_line"`
	EndLine     int64     `gorm:"not null" json:"end_line"`
	Status      JobStatus `gorm:"not null;default:'queued'" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}
