package cron

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	bookingredis "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/booking-show/booking-show-api/pkg/sse"
)

// StartOrderCleanupCronjob chạy 1 phút 1 lần để hủy các đơn hàng chưa thanh toán quá 10 phút.
func StartOrderCleanupCronjob() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			processExpiredOrders()
			processExpiredLockedSeats()
		}
	}()
}

func processExpiredOrders() {
	// Lấy tất cả Order Pending hết hạn
	var expiredOrders []model.Order
	now := time.Now()

	// Truy vấn các order PENDING chưa quá hạn nhưng field ExpiredAt đã qua now
	if err := repository.DB.Where("status = ? AND expires_at < ?", model.OrderPending, now).Find(&expiredOrders).Error; err != nil {
		log.Printf("Cronjob Error querying expired orders: %v", err)
		return
	}

	for _, order := range expiredOrders {
		tx := repository.DB.Begin()
		if err := tx.Error; err != nil {
			log.Printf("Cronjob error starting transaction: %v", err)
			continue
		}

		// 1. Rollback Voucher used_count
		if order.PromotionID != nil {
			if err := tx.Model(&model.Promotion{}).
				Where("id = ?", *order.PromotionID).
				UpdateColumn("used_count", repository.DB.Raw("GREATEST(0, used_count - 1)")).Error; err != nil {
				tx.Rollback()
				continue
			}
		}

		// 2. Chuyển Status = CANCELLED
		if err := tx.Model(&order).Update("status", model.OrderCancelled).Error; err != nil {
			tx.Rollback()
			continue
		}

		// 3. Lấy danh sách ghế thuộc đơn này từ order_seats (mapping chính xác)
		var orderSeats []model.ShowtimeSeat
		repository.DB.Joins("JOIN order_seats ON order_seats.showtime_seat_id = showtime_seats.id").
			Where("order_seats.order_id = ? AND showtime_seats.status = 'LOCKED'", order.ID).
			Find(&orderSeats)

		seatIDs := make([]int, len(orderSeats))
		for i, s := range orderSeats {
			seatIDs[i] = s.ID
		}

		if len(seatIDs) > 0 {
			if err := tx.Model(&model.ShowtimeSeat{}).
				Where("id IN ? AND status = 'LOCKED'", seatIDs).
				Updates(map[string]interface{}{
					"status":       model.StatusAvailable,
					"locked_by":    nil,
					"locked_until": nil,
				}).Error; err != nil {
				tx.Rollback()
				continue
			}
		}

		if err := tx.Commit().Error; err != nil {
			log.Printf("Cronjob error committing for order %s: %v", order.ID, err)
		} else {
			log.Printf("Successfully cancelled expired order %s", order.ID)
			// Phát SSE + cleanup Redis cho từng ghế vừa được release
			for _, seatID := range seatIDs {
				log.Printf("SSE: Broadcasting seat %d available for showtime %d", seatID, order.ShowtimeID)
				key := fmt.Sprintf("lock:showtime_seat:%d:%d", order.ShowtimeID, seatID)
				bookingredis.Client.Del(context.Background(), key)
				sse.BroadcastSeatUpdate(order.ShowtimeID, seatID, "AVAILABLE")
			}
		}
	}
}

// processExpiredLockedSeats dọn dẹp các ghế bị khóa (LOCKED) nhưng không có đơn hàng hoặc quá hạn locked_until
func processExpiredLockedSeats() {
	var expiredSeats []model.ShowtimeSeat
	now := time.Now()

	// Tìm ghế LOCKED đã quá hạn locked_until
	if err := repository.DB.Where("status = 'LOCKED' AND locked_until < ?", now).Find(&expiredSeats).Error; err != nil {
		log.Printf("Cronjob error querying expired seats: %v", err)
		return
	}

	for _, seat := range expiredSeats {
		tx := repository.DB.Begin()

		// Chuyển về AVAILABLE
		if err := tx.Model(&seat).Updates(map[string]interface{}{
			"status":       model.StatusAvailable,
			"locked_by":    nil,
			"locked_until": nil,
		}).Error; err != nil {
			tx.Rollback()
			continue
		}

		if err := tx.Commit().Error; err != nil {
			log.Printf("Cronjob error releasing seat %d: %v", seat.ID, err)
		} else {
			// Xóa Redis key
			key := fmt.Sprintf("lock:showtime_seat:%d:%d", seat.ShowtimeID, seat.ID)
			bookingredis.Client.Del(context.Background(), key)
			// Phát SSE
			sse.BroadcastSeatUpdate(seat.ShowtimeID, seat.ID, "AVAILABLE")
			log.Printf("Released expired locked seat %d (showtime %d)", seat.ID, seat.ShowtimeID)
		}
	}
}
