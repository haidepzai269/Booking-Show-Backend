package model

import "time"

type UserRole string

const (
	RoleCustomer      UserRole = "CUSTOMER"
	RoleAdmin         UserRole = "ADMIN"
	RoleCinemaManager UserRole = "CINEMA_MANAGER"
)

type User struct {
	ID              int       `json:"id" gorm:"primaryKey;autoIncrement"`
	FullName        string    `json:"full_name" gorm:"type:varchar(100);not null"`
	Email           string    `json:"email" gorm:"type:varchar(255);unique;not null"`
	Phone           string    `json:"phone" gorm:"type:varchar(20);default:''"`
	PasswordHash    string    `json:"-" gorm:"type:varchar(255);not null"`
	Role            UserRole  `json:"role" gorm:"type:varchar(20);default:'CUSTOMER'"`
	ThemePreference string    `json:"theme_preference" gorm:"type:varchar(20);default:'dark'"`
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	IsActive        bool      `json:"is_active" gorm:"default:true"`
}
