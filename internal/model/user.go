package model

import "time"

type UserRole string

const (
	RoleCustomer      UserRole = "CUSTOMER"
	RoleAdmin         UserRole = "ADMIN"
	RoleCinemaManager UserRole = "CINEMA_MANAGER"
)

type UserRank string

const (
	RankBronze   UserRank = "BRONZE"
	RankSilver   UserRank = "SILVER"
	RankGold     UserRank = "GOLD"
	RankPlatinum UserRank = "PLATINUM"
	RankDiamond  UserRank = "DIAMOND"
)

type User struct {
	ID              int       `json:"id" gorm:"primaryKey;autoIncrement"`
	FullName        string    `json:"full_name" gorm:"type:varchar(100);not null"`
	Username        string    `json:"username" gorm:"type:varchar(50);unique"`
	Email           string    `json:"email" gorm:"type:varchar(255);unique;not null"`
	Phone           string    `json:"phone" gorm:"type:varchar(20);default:''"`
	PasswordHash    string    `json:"-" gorm:"type:varchar(255)"` // Không bắt buộc khi dùng OAuth
	Provider        string    `json:"provider" gorm:"type:varchar(20);default:'local'"`
	ProviderID      string    `json:"provider_id" gorm:"type:varchar(255)"`
	Role            UserRole  `json:"role" gorm:"type:varchar(20);default:'CUSTOMER'"`
	Rank            UserRank  `json:"rank" gorm:"type:varchar(20);default:'BRONZE'"`
	TotalSpending   float64   `json:"total_spending" gorm:"type:decimal(15,2);default:0"`
	Theme           string    `json:"theme" gorm:"type:varchar(20);default:'dark'"`
	Language        string    `json:"language" gorm:"type:varchar(10);default:'vi'"`
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	IsActive        bool      `json:"is_active" gorm:"default:true"`
}
