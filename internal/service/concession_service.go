package service

import (
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
)

// ─── AppError — loi co HTTP status + error code ────────────────────────────
type AppError struct {
	Code   string
	Status int
	Msg    string
	Data   map[string]interface{}
}

func (e *AppError) Error() string { return e.Msg }

// IsAppError ep kieu error sang *AppError
func IsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// ─── ConcessionService ─────────────────────────────────────────────────────
type ConcessionService struct{}

func (s *ConcessionService) ListConcessions() ([]model.Concession, error) {
	const key = "concessions:all"
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, key).Result(); err == nil {
			var concessions []model.Concession
			if json.Unmarshal([]byte(cached), &concessions) == nil {
				log.Println("[Cache HIT] concessions:all")
				return concessions, nil
			}
		}
	}
	log.Println("[Cache MISS] concessions:all - querying DB")

	var concessions []model.Concession
	if err := repository.DB.Where("is_active = ?", true).Find(&concessions).Error; err != nil {
		return nil, err
	}

	if redispkg.Client != nil {
		if data, err := json.Marshal(concessions); err == nil {
			redispkg.Client.Set(redispkg.Ctx, key, data, 24*time.Hour)
		}
	}
	return concessions, nil
}

func (s *ConcessionService) ListAdminConcessions(page, limit int, q string) ([]model.Concession, int64, error) {
	var concessions []model.Concession
	var total int64

	query := repository.DB.Model(&model.Concession{})

	if q != "" {
		query = query.Where("name ILIKE ?", "%"+q+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&concessions).Error; err != nil {
		return nil, 0, err
	}

	return concessions, total, nil
}

func (s *ConcessionService) GetConcession(id int) (*model.Concession, error) {
	const key = "concessions:all"
	// Thu tim trong cache cua danh sach truoc
	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, key).Result(); err == nil {
			var concessions []model.Concession
			if json.Unmarshal([]byte(cached), &concessions) == nil {
				for _, c := range concessions {
					if c.ID == id {
						return &c, nil
					}
				}
			}
		}
	}

	var c model.Concession
	if err := repository.DB.Where("id = ? AND is_active = ?", id, true).First(&c).Error; err != nil {
		return nil, errors.New("concession not found")
	}
	return &c, nil
}

type ConcessionReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Price       int    `json:"price" binding:"required,gt=0"`
	ImageURL    string `json:"image_url"`
	IsActive    *bool  `json:"is_active"`
}

func (s *ConcessionService) CreateConcession(req ConcessionReq) (*model.Concession, error) {
	concession := model.Concession{
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		ImageURL:    req.ImageURL,
		IsActive:    true,
	}

	if req.IsActive != nil {
		concession.IsActive = *req.IsActive
	}

	if err := repository.DB.Create(&concession).Error; err != nil {
		return nil, err
	}

	if redispkg.Client != nil {
		redispkg.Client.Del(redispkg.Ctx, "concessions:all")
	}

	return &concession, nil
}

func (s *ConcessionService) UpdateConcession(id int, req ConcessionReq) (*model.Concession, error) {
	var concession model.Concession
	if err := repository.DB.First(&concession, id).Error; err != nil {
		return nil, errors.New("concession not found")
	}

	if req.Name != "" {
		concession.Name = req.Name
	}
	if req.Description != "" {
		concession.Description = req.Description
	}
	if req.Price > 0 {
		concession.Price = req.Price
	}
	if req.ImageURL != "" {
		concession.ImageURL = req.ImageURL
	}
	if req.IsActive != nil {
		concession.IsActive = *req.IsActive
	}

	if err := repository.DB.Save(&concession).Error; err != nil {
		return nil, err
	}

	if redispkg.Client != nil {
		redispkg.Client.Del(redispkg.Ctx, "concessions:all")
	}

	return &concession, nil
}

func (s *ConcessionService) DeleteConcession(id int) error {
	var concession model.Concession
	if err := repository.DB.First(&concession, id).Error; err != nil {
		return errors.New("concession not found")
	}

	if err := repository.DB.Model(&concession).Update("is_active", false).Error; err != nil {
		return err
	}

	if redispkg.Client != nil {
		redispkg.Client.Del(redispkg.Ctx, "concessions:all")
	}

	return nil
}


