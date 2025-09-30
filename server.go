package main

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"hashtasker-go/internal/auth"
	"hashtasker-go/internal/database"
	"hashtasker-go/internal/handlers"
	"hashtasker-go/internal/middleware"
	"hashtasker-go/internal/models"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// Embed the web directory
//
//go:embed web/templates/*.html
var templatesFS embed.FS

//go:embed web/static
var staticFS embed.FS

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Try SQLite first for development
	if err := database.ConnectSQLite("hashtasker.db"); err != nil {
		// Fallback to MySQL if SQLite fails
		if err := database.Connect(config.Database); err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
	}

	if err := database.Migrate(); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	hashedPassword, _ := auth.HashPassword(config.Admin.Password)
	config.Admin.Password = hashedPassword

	if err := database.CreateDefaultAdmin(config.Admin); err != nil {
		log.Fatalf("Failed to create default admin: %v", err)
	}

	authService := auth.NewAuthService(config.LDAP)

	r := setupRouter(authService, config.Server)

	addr := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)
	log.Printf("Starting server on %s", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func loadConfig() (*models.Config, error) {
	// Check if config file exists
	if _, err := os.Stat("config.yaml"); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file 'config.yaml' not found - please create one from config.yaml.example")
	}

	// Read config file
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	// Parse config
	var config models.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Validate required fields
	if config.Server.SessionKey == "" || config.Server.SessionKey == "change-this-to-a-secure-random-string" {
		return nil, fmt.Errorf("server.session_key must be set to a secure random string in config.yaml")
	}
	if config.Database.Password == "" || config.Database.Password == "your-secure-password" {
		return nil, fmt.Errorf("database.password must be set in config.yaml")
	}
	if config.Admin.Password == "" || config.Admin.Password == "changeme" {
		return nil, fmt.Errorf("admin.password must be set in config.yaml")
	}

	return &config, nil
}

func setupRouter(authService *auth.AuthService, serverConfig models.ServerConfig) *gin.Engine {
	r := gin.Default()

	// Load templates from embedded filesystem
	tmpl := template.Must(template.New("").ParseFS(templatesFS, "web/templates/*.html"))
	r.SetHTMLTemplate(tmpl)

	// Serve static files from embedded filesystem
	staticSubFS, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		log.Fatalf("Failed to create static subfilesystem: %v", err)
	}
	r.StaticFS("/static", http.FS(staticSubFS))

	authHandler := handlers.NewAuthHandler(authService)
	dashboardHandler := handlers.NewDashboardHandler()
	jobHandler := handlers.NewJobHandler(serverConfig)
	workerHandler := handlers.NewWorkerHandler(serverConfig.UploadPath)
	userHandler := handlers.NewUserHandler()
	contentHandler := handlers.NewContentHandler()

	r.GET("/login", authHandler.LoginPage)
	r.POST("/login", authHandler.Login)
	r.POST("/logout", authHandler.Logout)

	// Worker API endpoints (no auth required)
	workerAPI := r.Group("/api/worker")
	{
		workerAPI.POST("/stats", workerHandler.UpdateWorkerStats)
		workerAPI.POST("/progress", workerHandler.ReportHashcatProgress)
		workerAPI.POST("/cracked/:job_uid", workerHandler.ReportCrackedHash)
		workerAPI.GET("/assignments", workerHandler.GetJobAssignments)
		workerAPI.GET("/wordlist/:job_uid/:chunk_index", workerHandler.GetWordlistChunk)
		workerAPI.POST("/chunk/:job_uid/:chunk_index/status", workerHandler.UpdateChunkStatus)
	}

	// Authenticated routes
	auth := r.Group("/")
	auth.Use(middleware.AuthRequired(authService))
	{
		auth.GET("/", dashboardHandler.Dashboard)
		auth.GET("/system-stats", dashboardHandler.SystemStats)
		auth.GET("/jobs", jobHandler.JobsPage)
		auth.GET("/jobs/create", jobHandler.CreateJobPage)
		auth.GET("/jobs/:uid", jobHandler.JobDetailsPage)
		auth.GET("/profile", userHandler.ProfilePage)
		auth.GET("/admin/users", userHandler.UserManagementPage)
		auth.GET("/troubleshooting", contentHandler.TroubleshootingPage)
		auth.GET("/changelog", contentHandler.ChangelogPage)

		api := auth.Group("/api")
		api.Use(middleware.JSONAuthRequired(authService))
		{
			api.GET("/user", authHandler.GetCurrentUser)
			api.GET("/dashboard/stats", dashboardHandler.GetDashboardStats)
			api.GET("/system/stats", dashboardHandler.GetSystemStats)

			// Job management
			api.GET("/jobs", jobHandler.GetJobs)
			api.POST("/jobs", jobHandler.CreateJob)
			api.GET("/jobs/:uid", jobHandler.GetJobDetails)
			api.GET("/jobs/:uid/cracked", jobHandler.GetJobCrackedHashes)
			api.GET("/jobs/:uid/copy", jobHandler.CopyJob)
			api.POST("/jobs/:uid/cancel", jobHandler.CancelJob)
			api.DELETE("/jobs/:uid", jobHandler.DeleteJob)

			// Job options
			api.GET("/jobs/wordlists", jobHandler.GetWordlists)
			api.GET("/jobs/rulesets", jobHandler.GetRulesets)
			api.GET("/jobs/hash-modes", jobHandler.GetHashModes)

			// User management (regular users)
			api.PUT("/user/password", userHandler.ChangePassword)

			// Admin user management
			api.GET("/admin/users", userHandler.GetUsers)
			api.POST("/admin/users", userHandler.CreateUser)
			api.PUT("/admin/users/:id", userHandler.UpdateUser)
			api.PUT("/admin/users/:id/password", userHandler.ChangeUserPassword)
			api.DELETE("/admin/users/:id", userHandler.DeleteUser)
		}
	}

	// Background task to mark workers offline
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			workerHandler.MarkWorkersOffline()
		}
	}()

	return r
}
