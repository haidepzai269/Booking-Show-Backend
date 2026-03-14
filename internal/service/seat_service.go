package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	bookingredis "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/booking-show/booking-show-api/pkg/sse"
)

type SeatService struct{}

func (s *SeatService) GetSeats(showtimeID int) ([]model.ShowtimeSeat, error) {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("cache:showtime_seats:%d", showtimeID)

	// 1. Thử lấy từ Redis Cache trước
	if bookingredis.Client != nil {
		val, err := bookingredis.Client.Get(ctx, cacheKey).Result()
		if err == nil {
			var cachedSeats []model.ShowtimeSeat
			if err := json.Unmarshal([]byte(val), &cachedSeats); err == nil {
				return cachedSeats, nil
			}
		}
	}

	// 2. Nếu không có cache, truy vấn Database
	var seats []model.ShowtimeSeat
	if err := repository.DB.Preload("Seat").Where("showtime_id = ?", showtimeID).Find(&seats).Error; err != nil {
		return nil, err
	}

	// 3. Read-Repair: Giải phóng ghế hết hạn ngay lập tức nếu thấy
	now := time.Now()
	for i := range seats {
		if seats[i].Status == "LOCKED" && seats[i].LockedUntil != nil && seats[i].LockedUntil.Before(now) {
			seats[i].Status = "AVAILABLE"
			seats[i].LockedBy = nil
			seats[i].LockedUntil = nil
			// Cập nhật ngầm vào DB (không chặn request chính nếu có thể, hoặc làm đồng bộ để chính xác)
			repository.DB.Model(&seats[i]).Updates(map[string]interface{}{
				"status":       "AVAILABLE",
				"locked_by":    nil,
				"locked_until": nil,
			})
			// Xóa Redis key và phát SSE
			key := fmt.Sprintf("lock:showtime_seat:%d:%d", showtimeID, seats[i].ID)
			if bookingredis.Client != nil {
				bookingredis.Client.Del(ctx, key)
			}
			sse.BroadcastSeatUpdate(showtimeID, seats[i].ID, "AVAILABLE")
		}
	}

	// 4. Lưu vào Cache với thời gian sống ngắn (2 giây) nếu có thay đổi hoặc cache trống
	if bookingredis.Client != nil {
		if seatsData, err := json.Marshal(seats); err == nil {
			bookingredis.Client.Set(ctx, cacheKey, seatsData, 2*time.Second)
		}
	}

	return seats, nil
}

type LockSeatReq struct {
	ShowtimeID int   `json:"showtime_id" binding:"required"`
	SeatIDs    []int `json:"seat_ids" binding:"required,min=1"`
}

// LockSeat Dùng Redis Set NX để tránh Race Condition khi Lock ghế.
func (s *SeatService) LockSeat(req LockSeatReq, userID int) error {
	ctx := context.Background()
	lockDuration := 10 * time.Minute // 10 phút timeout

	// 1. Kiểm tra nhanh bằng redis nx loop
	var lockedKeys []string

	for _, seatID := range req.SeatIDs {
		key := fmt.Sprintf("lock:showtime_seat:%d:%d", req.ShowtimeID, seatID)
		// NX = Chỉ set nếu key chưa tồn tại
		if bookingredis.Client != nil {
			success, err := bookingredis.Client.SetNX(ctx, key, userID, lockDuration).Result()
			if err != nil {
				// Rollback Redis if err
				s.unlockRedisKeys(lockedKeys)
				return err
			}
			if !success {
				// Rollback rediscover keys
				s.unlockRedisKeys(lockedKeys)
				return errors.New("one or more seats are already locked or booked")
			}
		}
		lockedKeys = append(lockedKeys, key)
	}

	// 2. Tới đây tức là đã lock được toàn bộ trên Redis, tiến hành Update DB Database
	tx := repository.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			s.unlockRedisKeys(lockedKeys)
		}
	}()

	var seats []model.ShowtimeSeat
	// Cho phép lock nếu ghế AVAILABLE HOẶC ghế đang LOCKED nhưng đã hết hạn (locked_until < now)
	now := time.Now()
	if err := tx.Where("id IN ? AND showtime_id = ? AND (status = 'AVAILABLE' OR (status = 'LOCKED' AND locked_until < ?))", 
		req.SeatIDs, req.ShowtimeID, now).Find(&seats).Error; err != nil {
		tx.Rollback()
		s.unlockRedisKeys(lockedKeys)
		return err
	}

	if len(seats) != len(req.SeatIDs) {
		tx.Rollback()
		s.unlockRedisKeys(lockedKeys)
		return errors.New("một số ghế bạn chọn đã bị người khác đặt hoặc đang trong quá trình thanh toán")
	}

	for _, seat := range seats {
		seat.Status = "LOCKED"
		userIDPtr := userID
		seat.LockedBy = &userIDPtr
		until := now.Add(lockDuration)
		seat.LockedUntil = &until
		if err := tx.Save(&seat).Error; err != nil {
			tx.Rollback()
			s.unlockRedisKeys(lockedKeys)
			return err
		}

		sse.BroadcastSeatUpdate(req.ShowtimeID, seat.ID, "LOCKED")
	}

	if err := tx.Commit().Error; err != nil {
		s.unlockRedisKeys(lockedKeys)
		return err
	}

	return nil
}

func (s *SeatService) unlockRedisKeys(keys []string) {
	if bookingredis.Client == nil {
		return
	}
	ctx := context.Background()
	for _, k := range keys {
		bookingredis.Client.Del(ctx, k)
	}
}

// Mở Khóa Ghế (Do user chủ động hủy hoặc hết timeout)
func (s *SeatService) UnlockSeats(showtimeID, userID int, seatIDs []int) error {
	tx := repository.DB.Begin()

	for _, seatID := range seatIDs {
		// Dùng SQL nguyên thuỷ để update nếu đúng ng giữ khoá
		if err := tx.Model(&model.ShowtimeSeat{}).
			Where("id = ? AND showtime_id = ? AND locked_by = ?", seatID, showtimeID, userID).
			Updates(map[string]interface{}{
				"status":       "AVAILABLE",
				"locked_by":    nil,
				"locked_until": nil,
			}).Error; err != nil {
			tx.Rollback()
			return err
		}
		// Trigger SSE Event
		if bookingredis.Client != nil {
			key := fmt.Sprintf("lock:showtime_seat:%d:%d", showtimeID, seatID)
			bookingredis.Client.Del(context.Background(), key)
		}

		sse.BroadcastSeatUpdate(showtimeID, seatID, "AVAILABLE")
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}
	return nil
}

// Bổ sung luồng InitSeats cho Admin Setup trước khi mở bán
func (s *SeatService) InitSeats(roomID, showtimeID int) error {
	var room model.Room
	if err := repository.DB.First(&room, roomID).Error; err != nil {
		return errors.New("room not found")
	}

	var allSeats []model.Seat
	if err := repository.DB.Where("room_id = ?", roomID).Find(&allSeats).Error; err != nil {
		return err
	}

	var seats []model.ShowtimeSeat
	for _, seat := range allSeats {
		seats = append(seats, model.ShowtimeSeat{
			ShowtimeID: showtimeID,
			SeatID:     seat.ID,
			Status:     "AVAILABLE",
			Price:      75000,
		})
	}

	if err := repository.DB.Create(&seats).Error; err != nil {
		return err
	}

	return nil
}

type SeatLayoutUpdateDTO struct {
	ID    int
	X     float64
	Y     float64
	Angle float64
}

// UpdateSeatsLayout cập nhật hàng loạt tọa độ X, Y, Angle của ghế trong 1 phòng
func (s *SeatService) UpdateSeatsLayout(roomID int, updates []SeatLayoutUpdateDTO) error {
	tx := repository.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	for _, u := range updates {
		// Update map chỉ các trường tọa độ để không ảnh hưởng dữ liệu khác
		if err := tx.Model(&model.Seat{}).Where("id = ? AND room_id = ?", u.ID, roomID).
			Updates(map[string]interface{}{
				"x":     u.X,
				"y":     u.Y,
				"angle": u.Angle,
			}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}
