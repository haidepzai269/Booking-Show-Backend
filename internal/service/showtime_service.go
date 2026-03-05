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

type ShowtimeService struct{}

// GetShowtimesByMovie - Lay lich chieu cho 1 phim (cache 5 phut)
func (s *ShowtimeService) GetShowtimesByMovie(movieID int) ([]model.Showtime, error) {
	key := fmt.Sprintf("showtimes:movie:%d", movieID)

	if redispkg.Client != nil {
		if cached, err := redispkg.Client.Get(redispkg.Ctx, key).Result(); err == nil {
			var showtimes []model.Showtime
			if json.Unmarshal([]byte(cached), &showtimes) == nil {
				log.Printf("[Cache HIT] %s\n", key)
				return showtimes, nil
			}
		}
	}
	log.Printf("[Cache MISS] %s - querying DB\n", key)

	now := time.Now()
	var showtimes []model.Showtime
	if err := repository.DB.Where("movie_id = ? AND start_time > ?", movieID, now).
		Preload("Movie").
		Preload("Room.Cinema").
		Order("start_time asc").
		Find(&showtimes).Error; err != nil {
		return nil, err
	}

	// Cache 5 phut (lich chieu biet thay doi khi rap them/xoa suat)
	if redispkg.Client != nil {
		if data, err := json.Marshal(showtimes); err == nil {
			redispkg.Client.Set(redispkg.Ctx, key, data, 5*time.Minute)
		}
	}
	return showtimes, nil
}

func (s *ShowtimeService) GetShowtime(id int) (*model.Showtime, error) {
	var showtime model.Showtime
	if err := repository.DB.
		Preload("Movie").
		Preload("Room.Cinema").
		First(&showtime, id).Error; err != nil {
		return nil, err
	}
	return &showtime, nil
}

type CreateShowtimeReq struct {
	MovieID   int       `json:"movie_id" binding:"required"`
	CinemaID  int       `json:"cinema_id" binding:"required"`
	RoomID    int       `json:"room_id" binding:"required"`
	StartTime time.Time `json:"start_time" binding:"required"` // Format ISO8601
}

func (s *ShowtimeService) CreateShowtime(req CreateShowtimeReq) (*model.Showtime, error) {
	showtime := model.Showtime{
		MovieID:   req.MovieID,
		RoomID:    req.RoomID,
		StartTime: req.StartTime,
		EndTime:   req.StartTime.Add(2 * time.Hour),
		BasePrice: 75000,
	}

	if err := repository.DB.Create(&showtime).Error; err != nil {
		return nil, err
	}

	// Tự động khởi tạo ghế cho suất chiếu vừa tạo
	seatSvc := &SeatService{}
	if err := seatSvc.InitSeats(req.RoomID, showtime.ID); err != nil {
		log.Printf("[WARN] InitSeats failed for showtime %d (room %d): %v", showtime.ID, req.RoomID, err)
	} else {
		log.Printf("[INFO] InitSeats OK for showtime %d (room %d)", showtime.ID, req.RoomID)
	}

	// Active Invalidation - xoa cache lich chieu cua phim nay
	if redispkg.Client != nil {
		movieKey := fmt.Sprintf("showtimes:movie:%d", req.MovieID)
		cinemaDateKey := fmt.Sprintf("cinema:%d:movies:%s", req.CinemaID, req.StartTime.Format("2006-01-02"))
		redispkg.Client.Del(redispkg.Ctx, movieKey, cinemaDateKey)
		log.Printf("[Cache INVALIDATED] %s, %s\n", movieKey, cinemaDateKey)
	}

	return &showtime, nil
}

// UpdateShowtimeReq - request body cho cập nhật suất chiếu
type UpdateShowtimeReq struct {
	StartTime *time.Time `json:"start_time"`
	BasePrice *int       `json:"base_price"`
	RoomID    *int       `json:"room_id"`
	IsActive  *bool      `json:"is_active"`
}

// UpdateShowtime - Cập nhật suất chiếu
func (s *ShowtimeService) UpdateShowtime(id int, req UpdateShowtimeReq) (*model.Showtime, error) {
	var showtime model.Showtime
	if err := repository.DB.Preload("Movie").Preload("Room.Cinema").First(&showtime, id).Error; err != nil {
		return nil, fmt.Errorf("showtime not found")
	}

	if req.StartTime != nil {
		showtime.StartTime = *req.StartTime
		showtime.EndTime = req.StartTime.Add(2 * time.Hour)
	}
	if req.BasePrice != nil {
		showtime.BasePrice = *req.BasePrice
	}
	if req.RoomID != nil {
		showtime.RoomID = *req.RoomID
	}
	if req.IsActive != nil {
		showtime.IsActive = *req.IsActive
	}

	if err := repository.DB.Save(&showtime).Error; err != nil {
		return nil, err
	}

	// Invalidate cache
	if redispkg.Client != nil {
		movieKey := fmt.Sprintf("showtimes:movie:%d", showtime.MovieID)
		redispkg.Client.Del(redispkg.Ctx, movieKey)
	}

	return &showtime, nil
}

// DeleteShowtime - Soft delete suất chiếu (set is_active = false)
func (s *ShowtimeService) DeleteShowtime(id int) error {
	var showtime model.Showtime
	if err := repository.DB.First(&showtime, id).Error; err != nil {
		return fmt.Errorf("showtime not found")
	}

	result := repository.DB.Model(&model.Showtime{}).Where("id = ?", id).Update("is_active", false)
	if result.Error != nil {
		return result.Error
	}

	// Invalidate cache
	if redispkg.Client != nil {
		movieKey := fmt.Sprintf("showtimes:movie:%d", showtime.MovieID)
		redispkg.Client.Del(redispkg.Ctx, movieKey)
	}

	return nil
}

// ListAdminShowtimes - Danh sách suất chiếu cho admin (có filter)
func (s *ShowtimeService) ListAdminShowtimes(movieID, page, limit int) ([]model.Showtime, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	db := repository.DB.Preload("Movie").Preload("Room.Cinema")
	if movieID > 0 {
		db = db.Where("movie_id = ?", movieID)
	}

	var total int64
	db.Model(&model.Showtime{}).Count(&total)

	var showtimes []model.Showtime
	if err := db.Order("start_time DESC").Limit(limit).Offset(offset).Find(&showtimes).Error; err != nil {
		return nil, 0, err
	}

	return showtimes, total, nil
}
