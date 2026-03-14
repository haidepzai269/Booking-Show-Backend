package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
)

type ChatService struct {
	Repo *repository.ChatRepository
}

func NewChatService() *ChatService {
	return &ChatService{
		Repo: repository.NewChatRepository(),
	}
}

func (s *ChatService) SaveMessage(userID *uint, sessionID, role, content string) error {
	msg := &model.ChatHistory{
		UserID:    userID,
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	}
	err := s.Repo.SaveMessage(msg)
	if err == nil {
		// Xóa cache khi có tin nhắn mới
		if redispkg.Client != nil {
			cacheKey := fmt.Sprintf("chat:history:%s", sessionID)
			redispkg.Client.Del(redispkg.Ctx, cacheKey)
			if userID != nil {
				userCacheKey := fmt.Sprintf("chat:history:user:%d", *userID)
				redispkg.Client.Del(redispkg.Ctx, userCacheKey)
			}
		}
	}
	return err
}

func (s *ChatService) GetHistory(sessionID string, userID *uint) ([]model.ChatHistory, error) {
	cacheKey := fmt.Sprintf("chat:history:%s", sessionID)
	if userID != nil {
		cacheKey = fmt.Sprintf("chat:history:user:%d", *userID)
	}

	// 1. Thử lấy từ Redis
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, cacheKey).Result(); err == nil && cached != "" {
			var history []model.ChatHistory
			if json.Unmarshal([]byte(cached), &history) == nil {
				return history, nil
			}
		}
	}

	// 2. Nếu rỗng, lấy từ DB
	var history []model.ChatHistory
	var err error
	if userID != nil {
		history, err = s.Repo.GetHistoryByUser(*userID)
	} else {
		history, err = s.Repo.GetHistoryBySession(sessionID)
	}

	if err != nil {
		return nil, err
	}

	// 3. Lưu vào Redis (TTL 30 phút)
	if redispkg.Client != nil && len(history) > 0 {
		if data, err := json.Marshal(history); err == nil {
			_ = redispkg.Client.Set(redispkg.Ctx, cacheKey, data, 30*time.Minute).Err()
		}
	}

	return history, nil
}
