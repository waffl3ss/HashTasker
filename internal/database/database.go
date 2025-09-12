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
		Logger: logger.Default.LogMode(logger.Info),
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
	
	err := DB.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.Job{},
		&models.JobProgress{},
		&models.CrackedHash{},
		&models.JobChunk{},
		&models.Worker{},
		&models.GPU{},
	)
	
	if err != nil {
		return fmt.Errorf("failed to run migrations: %v", err)
	}
	
	log.Println("Database migrations completed successfully")
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