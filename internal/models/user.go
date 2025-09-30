package models

import (
	"time"
	"gorm.io/gorm"
)

type User struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Username    string    `gorm:"unique;not null" json:"username"`
	Email       string    `gorm:"unique" json:"email"`
	IsAdmin     bool      `gorm:"default:false" json:"is_admin"`
	IsLocal     bool      `gorm:"default:false" json:"is_local"`
	Password    string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type Session struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	User      User      `gorm:"foreignKey:UserID" json:"user"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}