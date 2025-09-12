package models

import "time"

type Worker struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Hostname     string    `gorm:"unique;not null" json:"hostname"`
	CPUUsage     float64   `gorm:"default:0" json:"cpu_usage"`
	MemoryUsed   int64     `gorm:"default:0" json:"memory_used"`
	MemoryTotal  int64     `gorm:"default:0" json:"memory_total"`
	DiskUsage    float64   `gorm:"default:0" json:"disk_usage"`
	LastCheckin  time.Time `json:"last_checkin"`
	IsOnline     bool      `gorm:"default:false" json:"is_online"`
	HashcatProcs int       `gorm:"default:0" json:"hashcat_procs"`
	GPU          []GPU     `gorm:"foreignKey:WorkerID" json:"gpus"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type GPU struct {
	ID       uint    `gorm:"primaryKey" json:"id"`
	WorkerID uint    `gorm:"not null;index" json:"worker_id"`
	Worker   Worker  `gorm:"foreignKey:WorkerID" json:"worker"`
	Index    int     `gorm:"not null" json:"index"`
	Name     string  `json:"name"`
	Usage    float64 `gorm:"default:0" json:"usage"`
	Temp     float64 `gorm:"default:0" json:"temperature"`
	MemUsed  int64   `gorm:"default:0" json:"mem_used"`
	MemTotal int64   `gorm:"default:0" json:"mem_total"`
}

type WorkerStats struct {
	Hostname     string     `json:"hostname"`
	CPUUsage     float64    `json:"cpu_usage"`
	MemoryUsed   int64      `json:"memory_used"`
	MemoryTotal  int64      `json:"memory_total"`
	DiskUsage    float64    `json:"disk_usage"`
	HashcatProcs int        `json:"hashcat_procs"`
	GPUs         []GPUStats `json:"gpus"`
}

type GPUStats struct {
	Index    int     `json:"index"`
	Name     string  `json:"name"`
	Usage    float64 `json:"usage"`
	Temp     float64 `json:"temperature"`
	MemUsed  int64   `json:"mem_used"`
	MemTotal int64   `json:"mem_total"`
}

type HashcatProgress struct {
	JobUID        string  `json:"job_uid"`
	Progress      float64 `json:"progress"`
	KeyspaceSize  int64   `json:"keyspace_size"`
	TimeElapsed   int64   `json:"time_elapsed"`
	TimeRemaining int64   `json:"time_remaining"`
	Status        string  `json:"status"`
}
