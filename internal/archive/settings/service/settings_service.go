package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/pkg/redis"
)

type UserSettings struct {
	Theme    string `json:"theme"`
	Language string `json:"language"`
}

type SettingsService struct{}

func (s *SettingsService) GetSettings(userID int) (*UserSettings, error) {
	cacheKey := fmt.Sprintf("user_settings:%d", userID)

	// Try Redis first
	if redis.Client != nil {
		val, err := redis.Client.Get(context.Background(), cacheKey).Result()
		if err == nil {
			var settings UserSettings
			if err := json.Unmarshal([]byte(val), &settings); err == nil {
				return &settings, nil
			}
		}
	}

	// Fallback to DB
	var user model.User
	if err := repository.DB.Select("theme, language").First(&user, userID).Error; err != nil {
		return nil, err
	}

	settings := &UserSettings{
		Theme:    user.Theme,
		Language: user.Language,
	}

	// Cache to Redis
	s.cacheSettings(userID, settings)

	return settings, nil
}

func (s *SettingsService) UpdateSettings(userID int, settings UserSettings) error {
	// Update DB
	err := repository.DB.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"theme":    settings.Theme,
		"language": settings.Language,
	}).Error
	if err != nil {
		return err
	}

	// Update Redis
	s.cacheSettings(userID, &settings)

	return nil
}

func (s *SettingsService) cacheSettings(userID int, settings *UserSettings) {
	if redis.Client != nil {
		cacheKey := fmt.Sprintf("user_settings:%d", userID)
		val, _ := json.Marshal(settings)
		redis.Client.Set(context.Background(), cacheKey, val, 24*time.Hour)
	}
}
