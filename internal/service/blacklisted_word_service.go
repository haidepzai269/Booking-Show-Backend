package service

import (
	"errors"
	"strings"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
)

type BlacklistedWordService struct{}

func (s *BlacklistedWordService) GetAllWords() ([]model.BlacklistedWord, error) {
	var words []model.BlacklistedWord
	if err := repository.DB.Order("word ASC").Find(&words).Error; err != nil {
		return nil, err
	}
	return words, nil
}

func (s *BlacklistedWordService) AddWord(word string) (*model.BlacklistedWord, error) {
	word = strings.TrimSpace(strings.ToLower(word))
	if word == "" {
		return nil, errors.New("word cannot be empty")
	}

	bw := model.BlacklistedWord{Word: word}
	if err := repository.DB.Create(&bw).Error; err != nil {
		return nil, err
	}

	return &bw, nil
}

func (s *BlacklistedWordService) DeleteWord(id int) error {
	result := repository.DB.Delete(&model.BlacklistedWord{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("word not found")
	}
	return nil
}

// CheckIfContainsBlacklistedWord - check text against DB (Lớp 1 filter)
func (s *BlacklistedWordService) CheckIfContainsBlacklistedWord(text string) (bool, error) {
	var words []model.BlacklistedWord
	if err := repository.DB.Find(&words).Error; err != nil {
		return false, err
	}

	lowerText := strings.ToLower(text)
	for _, bw := range words {
		if strings.Contains(lowerText, bw.Word) {
			return true, nil
		}
	}

	return false, nil
}
