package service

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
)

type NotificationService struct{}

func NewNotificationService() *NotificationService {
	return &NotificationService{}
}

func (s *NotificationService) CreateNotification(n *model.Notification) error {
	if err := repository.DB.Create(n).Error; err != nil {
		return err
	}
	// Invalidate cache
	s.ClearCache()
	return nil
}

func (s *NotificationService) ListNotifications(page, limit int) ([]model.Notification, int64, error) {
	cacheKey := fmt.Sprintf("admin:notifications:list:%d:%d", page, limit)

	// Try cache
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil {
			var result struct {
				Data  []model.Notification `json:"data"`
				Total int64                `json:"total"`
			}
			if err := json.Unmarshal([]byte(cached), &result); err == nil {
				return result.Data, result.Total, nil
			}
		}
	}

	var notifications []model.Notification
	var total int64

	repository.DB.Model(&model.Notification{}).Count(&total)

	offset := (page - 1) * limit
	if err := repository.DB.Order("created_at DESC").Limit(limit).Offset(offset).Find(&notifications).Error; err != nil {
		return nil, 0, err
	}

	// Save cache
	if redispkg.Client != nil {
		res := struct {
			Data  []model.Notification `json:"data"`
			Total int64                `json:"total"`
		}{
			Data:  notifications,
			Total: total,
		}
		if b, err := json.Marshal(res); err == nil {
			redispkg.Client.Set(redispkg.Ctx, cacheKey, b, 10*time.Minute)
		}
	}

	return notifications, total, nil
}

func (s *NotificationService) MarkAllRead() error {
	if err := repository.DB.Model(&model.Notification{}).Where("is_read = ?", false).Update("is_read", true).Error; err != nil {
		return err
	}
	s.ClearCache()
	return nil
}

func (s *NotificationService) DeleteNotification(id uint) error {
	if err := repository.DB.Delete(&model.Notification{}, id).Error; err != nil {
		return err
	}
	s.ClearCache()
	return nil
}

func (s *NotificationService) ClearAllNotifications() error {
	if err := repository.DB.Exec("DELETE FROM notifications").Error; err != nil {
		return err
	}
	s.ClearCache()
	return nil
}

func (s *NotificationService) ClearCache() {
	if redispkg.Client != nil {
		iter := redispkg.Client.Scan(redispkg.Ctx, 0, "admin:notifications:list:*", 0).Iterator()
		var keys []string
		for iter.Next(redispkg.Ctx) {
			keys = append(keys, iter.Val())
		}
		if len(keys) > 0 {
			redispkg.Client.Del(redispkg.Ctx, keys...)
		}
		log.Printf("[NotificationService] Cache cleared")
	}
}
