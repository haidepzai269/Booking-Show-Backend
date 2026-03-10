package service

import (
	"errors"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"gorm.io/gorm"
)

type UserService struct{}

func (s *UserService) ListAdminUsers(page, limit int, q string) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	query := repository.DB.Model(&model.User{})

	if q != "" {
		query = query.Where("email ILIKE ? OR full_name ILIKE ?", "%"+q+"%", "%"+q+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	// Xóa password_hash trước khi trả về
	for i := range users {
		users[i].PasswordHash = ""
	}

	return users, total, nil
}

func (s *UserService) UpdateUserRole(id int, role string) error {
	if role != string(model.RoleCustomer) && role != string(model.RoleCinemaManager) && role != string(model.RoleAdmin) {
		return errors.New("invalid role")
	}

	var user model.User
	if err := repository.DB.First(&user, id).Error; err != nil {
		return errors.New("user not found")
	}

	if err := repository.DB.Model(&user).Update("role", role).Error; err != nil {
		return err
	}

	return nil
}

func (s *UserService) CalculateRank(spending float64) model.UserRank {
	if spending >= 20000000 {
		return model.RankDiamond
	}
	if spending >= 10000000 {
		return model.RankPlatinum
	}
	if spending >= 5000000 {
		return model.RankGold
	}
	if spending >= 2000000 {
		return model.RankSilver
	}
	return model.RankBronze
}

func (s *UserService) UpdateRankAfterPayment(tx *gorm.DB, userID int, amount float64) error {
	var user model.User
	if err := tx.First(&user, userID).Error; err != nil {
		return err
	}

	newSpending := user.TotalSpending + amount
	newRank := s.CalculateRank(newSpending)

	return tx.Model(&user).Updates(map[string]interface{}{
		"total_spending": newSpending,
		"rank":           newRank,
	}).Error
}
