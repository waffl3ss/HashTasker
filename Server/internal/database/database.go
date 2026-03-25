package database

import (
	"fmt"
	"log"
	"time"
	
	"hashtasker-go/internal/models"
	
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect(config models.DatabaseConfig) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.User,
		config.Password,
		config.Host,
		config.Port,
		config.Database,
	)

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return nil
}

func Migrate() error {
	log.Println("Running database migrations...")

	// Clean up duplicate cracked hashes before adding unique constraint
	log.Println("Cleaning up duplicate cracked hashes...")
	if err := cleanupDuplicateCrackedHashes(); err != nil {
		log.Printf("Warning: Failed to cleanup duplicates: %v", err)
	}

	err := DB.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.Job{},
		&models.JobProgress{},
		&models.CrackedHash{},
		&models.JobChunk{},
		&models.Worker{},
		&models.GPU{},
		// Analytics tables
		&models.AnalyticsSummary{},
		&models.AnalyticsJobsByPeriod{},
		&models.AnalyticsWordlistUsage{},
		&models.AnalyticsRulesetUsage{},
		&models.AnalyticsJobsByUser{},
		&models.AnalyticsHashModeUsage{},
	)

	if err != nil {
		return fmt.Errorf("failed to run migrations: %v", err)
	}

	// Recalculate cracked counts for all jobs to fix any incorrect counts
	log.Println("Recalculating cracked counts for all jobs...")
	if err := recalculateCrackedCounts(); err != nil {
		log.Printf("Warning: Failed to recalculate cracked counts: %v", err)
	}

	// Retroactively flag existing NTDS jobs (hash_mode=1000 and >300 hashes)
	log.Println("Flagging NTDS jobs...")
	if err := flagNTDSJobs(); err != nil {
		log.Printf("Warning: Failed to flag NTDS jobs: %v", err)
	}

	// Clean up orphaned records whose parent job was deleted
	log.Println("Cleaning up orphaned job records...")
	if err := cleanupOrphanedRecords(); err != nil {
		log.Printf("Warning: Failed to cleanup orphaned records: %v", err)
	}

	log.Println("Database migrations completed successfully")
	return nil
}

// flagNTDSJobs retroactively sets is_ntds=true on existing jobs that match criteria
func flagNTDSJobs() error {
	result := DB.Model(&models.Job{}).
		Where("hash_mode = ? AND total_hashes > ? AND is_ntds = ?", 1000, 300, false).
		Update("is_ntds", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		log.Printf("Flagged %d existing jobs as NTDS", result.RowsAffected)
	}
	return nil
}

// cleanupDuplicateCrackedHashes removes duplicate cracked hashes, keeping the earliest one
func cleanupDuplicateCrackedHashes() error {
	// Find all duplicate hash entries (same job_id and hash)
	type DuplicateHash struct {
		JobID uint
		Hash  string
		Count int
	}

	var duplicates []DuplicateHash
	err := DB.Raw(`
		SELECT job_id, hash, COUNT(*) as count
		FROM cracked_hashes
		GROUP BY job_id, hash
		HAVING COUNT(*) > 1
	`).Scan(&duplicates).Error

	if err != nil {
		return err
	}

	if len(duplicates) == 0 {
		log.Println("No duplicate cracked hashes found")
		return nil
	}

	log.Printf("Found %d duplicate hash entries, cleaning up...", len(duplicates))

	// For each duplicate, keep only the earliest entry
	for _, dup := range duplicates {
		// Get all IDs for this job_id/hash combination, ordered by cracked_at
		var ids []uint
		err := DB.Model(&models.CrackedHash{}).
			Where("job_id = ? AND hash = ?", dup.JobID, dup.Hash).
			Order("cracked_at ASC, id ASC").
			Pluck("id", &ids).Error

		if err != nil {
			log.Printf("Error finding IDs for duplicate job_id=%d hash=%s: %v", dup.JobID, dup.Hash, err)
			continue
		}

		if len(ids) <= 1 {
			continue
		}

		// Keep the first ID, delete the rest
		idsToDelete := ids[1:]
		result := DB.Where("id IN ?", idsToDelete).Delete(&models.CrackedHash{})
		if result.Error != nil {
			log.Printf("Error deleting duplicates for job_id=%d hash=%s: %v", dup.JobID, dup.Hash, result.Error)
		} else {
			log.Printf("Deleted %d duplicate entries for job_id=%d hash=%s", result.RowsAffected, dup.JobID, dup.Hash)
		}
	}

	return nil
}

// recalculateCrackedCounts updates the cracked_count for all jobs based on distinct hashes
func recalculateCrackedCounts() error {
	var jobs []models.Job
	if err := DB.Find(&jobs).Error; err != nil {
		return err
	}

	for _, job := range jobs {
		var crackedCount int64
		DB.Model(&models.CrackedHash{}).Where("job_id = ?", job.ID).Distinct("hash").Count(&crackedCount)

		if int(crackedCount) != job.CrackedCount {
			log.Printf("Updating job %s cracked count from %d to %d", job.UID, job.CrackedCount, crackedCount)
			job.CrackedCount = int(crackedCount)
			DB.Save(&job)
		}
	}

	return nil
}

// cleanupOrphanedRecords removes JobChunk, JobProgress, and CrackedHash records
// whose parent job has been soft-deleted or no longer exists.
func cleanupOrphanedRecords() error {
	tables := []struct {
		name  string
		model interface{}
	}{
		{"job_chunks", &models.JobChunk{}},
		{"job_progresses", &models.JobProgress{}},
		{"cracked_hashes", &models.CrackedHash{}},
	}

	for _, t := range tables {
		result := DB.Where("job_id NOT IN (SELECT id FROM jobs WHERE deleted_at IS NULL)").Delete(t.model)
		if result.Error != nil {
			log.Printf("Warning: Failed to cleanup orphaned %s: %v", t.name, result.Error)
			continue
		}
		if result.RowsAffected > 0 {
			log.Printf("Cleaned up %d orphaned records from %s", result.RowsAffected, t.name)
		}
	}

	return nil
}

func CreateDefaultAdmin(config models.AdminConfig) error {
	var user models.User
	result := DB.Where("username = ?", config.Username).First(&user)
	
	if result.Error == gorm.ErrRecordNotFound {
		hashedPassword, err := hashPassword(config.Password)
		if err != nil {
			return fmt.Errorf("failed to hash admin password: %v", err)
		}
		
		admin := models.User{
			Username: config.Username,
			Password: hashedPassword,
			IsAdmin:  true,
			IsLocal:  true,
		}
		
		if err := DB.Create(&admin).Error; err != nil {
			return fmt.Errorf("failed to create default admin user: %v", err)
		}
		
		log.Printf("Default admin user '%s' created successfully", config.Username)
	}
	
	return nil
}

func hashPassword(password string) (string, error) {
	// This function is now implemented in the auth package
	// Import it here to avoid circular dependencies
	return password, nil // The actual hashing is done in cmd/server/main.go
}