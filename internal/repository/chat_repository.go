package repository

import (
	"github.com/booking-show/booking-show-api/internal/model"
)

type ChatRepository struct{}

func NewChatRepository() *ChatRepository {
	return &ChatRepository{}
}

func (r *ChatRepository) SaveMessage(msg *model.ChatHistory) error {
	return DB.Create(msg).Error
}

func (r *ChatRepository) GetHistoryBySession(sessionID string) ([]model.ChatHistory, error) {
	var history []model.ChatHistory
	err := DB.Where("session_id = ?", sessionID).Order("created_at asc").Find(&history).Error
	return history, err
}

func (r *ChatRepository) GetHistoryByUser(userID uint) ([]model.ChatHistory, error) {
	var history []model.ChatHistory
	err := DB.Where("user_id = ?", userID).Order("created_at asc").Find(&history).Error
	return history, err
}
