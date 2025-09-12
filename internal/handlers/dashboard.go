package handlers

import (
	"net/http"
	"os"
	"runtime"
	"time"

	"hashtasker-go/internal/database"
	"hashtasker-go/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

type DashboardHandler struct{}

func NewDashboardHandler() *DashboardHandler {
	return &DashboardHandler{}
}

func (h *DashboardHandler) Dashboard(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	c.HTML(http.StatusOK, "base", gin.H{
		"title":    "Dashboard - HashTasker",
		"user":     u,
		"template": "dashboard",
	})
}

func (h *DashboardHandler) GetDashboardStats(c *gin.Context) {
	var runningJobs int64
	var totalJobs int64
	var activeWorkers int64
	var totalHashcatProcs int

	database.DB.Model(&models.Job{}).Where("status IN ?", []models.JobStatus{
		models.JobStatusQueued,
		models.JobStatusRunning,
	}).Count(&runningJobs)

	database.DB.Model(&models.Job{}).Count(&totalJobs)

	database.DB.Model(&models.Worker{}).Where("is_online = ?", true).Count(&activeWorkers)

	var workers []models.Worker
	database.DB.Where("is_online = ?", true).Find(&workers)
	for _, worker := range workers {
		totalHashcatProcs += worker.HashcatProcs
	}

	c.JSON(http.StatusOK, gin.H{
		"running_jobs":        runningJobs,
		"total_jobs":          totalJobs,
		"active_workers":      activeWorkers,
		"total_hashcat_procs": totalHashcatProcs,
	})
}

func (h *DashboardHandler) SystemStats(c *gin.Context) {
	user, _ := c.Get("user")
	u := user.(*models.User)

	c.HTML(http.StatusOK, "base", gin.H{
		"title":    "System Stats - HashTasker",
		"user":     u,
		"template": "system_stats",
	})
}

func (h *DashboardHandler) GetSystemStats(c *gin.Context) {
	var workers []models.Worker
	database.DB.Preload("GPU").Where("is_online = ?", true).Find(&workers)

	// Get server stats
	serverStats, err := h.getServerStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get server stats"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workers": workers,
		"server":  serverStats,
	})
}

func (h *DashboardHandler) getServerStats() (map[string]interface{}, error) {
	hostname, _ := os.Hostname()

	// CPU usage
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		return nil, err
	}
	var cpuUsage float64
	if len(cpuPercent) > 0 {
		cpuUsage = cpuPercent[0]
	}

	// Memory usage
	memStats, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	// Disk usage
	diskStats, err := disk.Usage("/")
	if err != nil {
		return nil, err
	}

	// Host info
	hostInfo, _ := host.Info()

	// Runtime stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"hostname":     hostname,
		"cpu_usage":    cpuUsage,
		"memory_used":  memStats.Used,
		"memory_total": memStats.Total,
		"disk_used":    diskStats.Used,
		"disk_total":   diskStats.Total,
		"uptime":       hostInfo.Uptime,
		"is_server":    true,
		"last_checkin": time.Now(),
	}, nil
}
