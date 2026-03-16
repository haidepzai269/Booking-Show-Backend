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
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrderService struct{}

type CreateOrderReq struct {
	ShowtimeID      int                     `json:"showtime_id" binding:"required"`
	ShowtimeSeatIDs []int                   `json:"showtime_seat_ids" binding:"required,min=1"`
	ConcessionItems []ConcessionItemRequest `json:"concession_items"`
	PromotionCode   string                  `json:"promotion_code"`
}

type ConcessionItemRequest struct {
	ConcessionID int `json:"concession_id" binding:"required"`
	Quantity     int `json:"quantity" binding:"required,min=1"`
}

func (s *OrderService) CreateOrder(req CreateOrderReq, userID int) (*model.Order, error) {
	// ─── Idempotent Check: Tránh tạo đơn lặp khi refresh trang ────────────────
	var existingOrder model.Order
	if err := repository.DB.Preload("OrderSeats").
		Where("user_id = ? AND showtime_id = ? AND status = 'PENDING'", userID, req.ShowtimeID).
		Order("created_at DESC").
		First(&existingOrder).Error; err == nil {

		// So sánh danh sách ghế
		if len(existingOrder.OrderSeats) == len(req.ShowtimeSeatIDs) {
			match := true
			existingSeatIDs := make(map[int]bool)
			for _, os := range existingOrder.OrderSeats {
				existingSeatIDs[os.ShowtimeSeatID] = true
			}
			for _, id := range req.ShowtimeSeatIDs {
				if !existingSeatIDs[id] {
					match = false
					break
				}
			}
			if match {
				// Trả về đơn hàng cũ thay vì tạo mới
				return &existingOrder, nil
			}
		}
	}

	tx := repository.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Step 1: Verify ghế đang bị user hiện tại lock
	var seats []model.ShowtimeSeat
	if err := tx.Where("id IN ? AND showtime_id = ? AND status = 'LOCKED' AND locked_by = ?",
		req.ShowtimeSeatIDs, req.ShowtimeID, userID).Find(&seats).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("failed to retrieve seats")
	}
	if len(seats) != len(req.ShowtimeSeatIDs) {
		tx.Rollback()
		return nil, errors.New("one or more seats are not locked by you or session expired")
	}

	seatTotal := 0
	for _, seat := range seats {
		seatTotal += seat.Price
	}

	// Step 2: Tính Concession Total & Snapshot giá
	concessionTotal := 0
	var concessionOrders []model.OrderConcession
	for _, item := range req.ConcessionItems {
		var concession model.Concession
		if err := tx.Where("id = ? AND is_active = ?", item.ConcessionID, true).First(&concession).Error; err != nil {
			tx.Rollback()
			return nil, &AppError{Code: "CONCESSION_NOT_FOUND", Status: 404, Msg: "Sản phẩm không tồn tại hoặc đã ẩn."}
		}
		concessionOrders = append(concessionOrders, model.OrderConcession{
			ConcessionID: concession.ID,
			Quantity:     item.Quantity,
			PriceAtTime:  concession.Price,
		})
		concessionTotal += concession.Price * item.Quantity
	}

	// Step 3: original amount
	originalAmount := seatTotal + concessionTotal

	// Step 4: Xử lý Voucher (validate lần 2 trong transaction)
	discountAmount := 0
	var promotionID *int
	if req.PromotionCode != "" {
		ps := &PromotionService{}
		promoRes, _, err := ps.ValidatePromotion(ValidatePromotionReq{
			Code:       req.PromotionCode,
			OrderValue: originalAmount,
		})
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		discountAmount = promoRes.DiscountAmount
		pID := promoRes.PromotionID
		promotionID = &pID

		// Step 8: Race condition: UPDATE chỉ khi used_count < usage_limit
		result := tx.Model(&model.Promotion{}).
			Where("id = ? AND used_count < usage_limit", promoRes.PromotionID).
			UpdateColumn("used_count", repository.DB.Raw("used_count + 1"))
		if result.Error != nil {
			tx.Rollback()
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			tx.Rollback()
			return nil, &AppError{Code: "PROMOTION_RACE_CONDITION", Status: 409, Msg: "voucher vừa hết lượt sử dụng."}
		}
	}

	// Step 5: final amount
	finalAmount := originalAmount - discountAmount
	if finalAmount < 0 {
		finalAmount = 0
	}

	// Step 6: Tạo đơn hàng (timeout 10 phút)
	order := model.Order{
		UserID:         userID,
		ShowtimeID:     req.ShowtimeID,
		PromotionID:    promotionID,
		OriginalAmount: originalAmount,
		DiscountAmount: discountAmount,
		FinalAmount:    finalAmount,
		Status:         model.OrderPending,
		ExpiresAt:      time.Now().Add(10 * time.Minute),
	}
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// ✅ FIX: Lưu mapping order → showtime_seats vào bảng order_seats
	// Đây là cách duy nhất đáng tin để biết ghế nào thuộc order này,
	// không phụ thuộc vào trạng thái LOCKED (có thể expire sau 10 phút)
	var orderSeats []model.OrderSeat
	for _, seat := range seats {
		orderSeats = append(orderSeats, model.OrderSeat{
			OrderID:        order.ID,
			ShowtimeSeatID: seat.ID,
		})
	}
	if err := tx.Create(&orderSeats).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Step 7: Insert order_concessions

	if len(concessionOrders) > 0 {
		for i := range concessionOrders {
			concessionOrders[i].OrderID = order.ID
		}
		if err := tx.Create(&concessionOrders).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	// Step 8: Đồng bộ thời gian hết hạn của ghế với đơn hàng
	// Tránh việc trình dọn dẹp quét nhầm ghế đang trong quá trình thanh toán
	if err := tx.Model(&model.ShowtimeSeat{}).
		Where("id IN ? AND showtime_id = ?", req.ShowtimeSeatIDs, req.ShowtimeID).
		Update("locked_until", order.ExpiresAt).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// Invalidate Cache (all pages)
	if bookingredis.Client != nil {
		ctx := context.Background()
		iter := bookingredis.Client.Scan(ctx, 0, fmt.Sprintf("user:orders:%d:*", userID), 0).Iterator()
		for iter.Next(ctx) {
			bookingredis.Client.Del(ctx, iter.Val())
		}
	}

	return &order, nil
}

// GetOrder — lấy chi tiết 1 đơn hàng (chỉ của user sở hữu hoặc admin)
func (s *OrderService) GetOrder(orderIDStr string, userID int, isAdmin bool) (*model.Order, error) {
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return nil, errors.New("invalid order ID")
	}
	var order model.Order
	q := repository.DB.Preload("User").Preload("Showtime.Movie").Preload("Showtime.Room.Cinema").
		Preload("Promotion").Preload("OrderSeats").Where("id = ?", orderID)
	if !isAdmin {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&order).Error; err != nil {
		return nil, errors.New("order not found")
	}
	return &order, nil
}

// MyOrders — danh sách đơn hàng của user (có phân trang & Redis cache)
func (s *OrderService) MyOrders(userID int, page, limit int) ([]model.Order, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	cacheKey := fmt.Sprintf("user:orders:%d:p%d:l%d", userID, page, limit)
	ctx := context.Background()

	// Try Cache
	if bookingredis.Client != nil {
		cached, err := bookingredis.Client.Get(ctx, cacheKey).Result()
		if err == nil {
			var result struct {
				Orders []model.Order `json:"orders"`
				Total  int64         `json:"total"`
			}
			if err := json.Unmarshal([]byte(cached), &result); err == nil {
				return result.Orders, result.Total, nil
			}
		}
	}

	var orders []model.Order
	var total int64

	query := repository.DB.Model(&model.Order{}).Where("user_id = ?", userID)
	query.Count(&total)

	if err := query.Preload("Showtime.Movie").Preload("Showtime.Room.Cinema").
		Preload("Promotion").
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	// Save Cache (10 mins)
	if bookingredis.Client != nil {
		result := struct {
			Orders []model.Order `json:"orders"`
			Total  int64         `json:"total"`
		}{Orders: orders, Total: total}
		data, _ := json.Marshal(result)
		bookingredis.Client.Set(ctx, cacheKey, data, 10*time.Minute)
	}

	return orders, total, nil
}

// CancelOrder — hủy đơn hàng + release ghế (dùng order_seats) + hoàn voucher + cleanup Redis
func (s *OrderService) CancelOrder(orderIDStr string, userID int) error {
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return errors.New("invalid order ID")
	}

	tx := repository.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Chỉ được hủy đơn PENDING của chính mình
	var order model.Order
	if err := tx.Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
		tx.Rollback()
		return errors.New("order not found")
	}
	if order.Status != model.OrderPending {
		tx.Rollback()
		return errors.New("only pending orders can be cancelled")
	}

	// 1. Lấy danh sách ghế thuộc đơn này từ bảng order_seats (chính xác hơn locked_by)
	var orderSeats []model.OrderSeat
	if err := tx.Where("order_id = ?", order.ID).Find(&orderSeats).Error; err != nil {
		tx.Rollback()
		return err
	}
	seatIDs := make([]int, len(orderSeats))
	for i, os := range orderSeats {
		seatIDs[i] = os.ShowtimeSeatID
	}

	// 2. Cập nhật status đơn hàng → CANCELLED
	if err := tx.Model(&order).Update("status", model.OrderCancelled).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 3. Release đúng những ghế thuộc đơn này (không ảnh hưởng ghế đơn khác)
	if len(seatIDs) > 0 {
		if err := tx.Model(&model.ShowtimeSeat{}).
			Where("id IN ? AND status = 'LOCKED'", seatIDs).
			Updates(map[string]interface{}{
				"status":       model.StatusAvailable,
				"locked_by":    nil,
				"locked_until": nil,
			}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	// 4. Hoàn voucher used_count (nếu có)
	if order.PromotionID != nil {
		tx.Model(&model.Promotion{}).
			Where("id = ?", *order.PromotionID).
			UpdateColumn("used_count", repository.DB.Raw("GREATEST(0, used_count - 1)"))
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	// Invalidate User Orders Cache
	if bookingredis.Client != nil {
		cacheKey := fmt.Sprintf("user:orders:%d", userID)
		bookingredis.Client.Del(context.Background(), cacheKey)
	}

	// 5. Sau khi commit: cleanup Redis key và phát SSE cho từng ghế
	ctx := context.Background()
	for _, seatID := range seatIDs {
		key := fmt.Sprintf("lock:showtime_seat:%d:%d", order.ShowtimeID, seatID)
		bookingredis.Client.Del(ctx, key)
		sse.BroadcastSeatUpdate(order.ShowtimeID, seatID, "AVAILABLE")
	}

	return nil
}

// UpdateOrderConcessions — cập nhật danh sách bắp nước cho đơn PENDING
func (s *OrderService) UpdateOrderConcessions(orderIDStr string, userID int, items []ConcessionItemRequest) error {
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return errors.New("invalid order ID")
	}

	tx := repository.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order model.Order
	if err := tx.Where("id = ? AND user_id = ? AND status = 'PENDING'", orderID, userID).First(&order).Error; err != nil {
		tx.Rollback()
		return errors.New("order not found or not pending")
	}

	// Xóa concessions cũ
	if err := tx.Where("order_id = ?", order.ID).Delete(&model.OrderConcession{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Tính tổng mới và insert
	concessionTotal := 0
	var concessionOrders []model.OrderConcession
	for _, item := range items {
		var c model.Concession
		if err := tx.Where("id = ? AND is_active = true", item.ConcessionID).First(&c).Error; err != nil {
			tx.Rollback()
			return &AppError{Code: "CONCESSION_NOT_FOUND", Status: 404, Msg: "Sản phẩm không tồn tại."}
		}
		concessionOrders = append(concessionOrders, model.OrderConcession{
			OrderID:      order.ID,
			ConcessionID: c.ID,
			Quantity:     item.Quantity,
			PriceAtTime:  c.Price,
		})
		concessionTotal += c.Price * item.Quantity
	}
	if len(concessionOrders) > 0 {
		if err := tx.Create(&concessionOrders).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	// Tính lại seat total từ order_seats
	var orderSeats []model.OrderSeat
	tx.Where("order_id = ?", order.ID).Find(&orderSeats)
	seatIDs := make([]int, len(orderSeats))
	for i, os := range orderSeats {
		seatIDs[i] = os.ShowtimeSeatID
	}
	seatTotal := 0
	if len(seatIDs) > 0 {
		var seats []model.ShowtimeSeat
		tx.Where("id IN ?", seatIDs).Find(&seats)
		for _, seat := range seats {
			seatTotal += seat.Price
		}
	}

	originalAmount := seatTotal + concessionTotal
	finalAmount := originalAmount - order.DiscountAmount
	if finalAmount < 0 {
		finalAmount = 0
	}
	if err := tx.Model(&order).Updates(map[string]interface{}{
		"original_amount": originalAmount,
		"final_amount":    finalAmount,
	}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	// Invalidate Cache
	if bookingredis.Client != nil {
		cacheKey := fmt.Sprintf("user:orders:%d", userID)
		bookingredis.Client.Del(context.Background(), cacheKey)
	}

	return nil
}

// ApplyOrderVoucher — áp dụng hoặc xóa voucher cho đơn PENDING
func (s *OrderService) ApplyOrderVoucher(orderIDStr string, userID int, code string) (*model.Order, error) {
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return nil, errors.New("invalid order ID")
	}

	tx := repository.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order model.Order
	if err := tx.Where("id = ? AND user_id = ? AND status = 'PENDING'", orderID, userID).First(&order).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("order not found or not pending")
	}

	// Hoàn lại voucher cũ nếu có
	if order.PromotionID != nil {
		tx.Model(&model.Promotion{}).
			Where("id = ?", *order.PromotionID).
			UpdateColumn("used_count", repository.DB.Raw("GREATEST(0, used_count - 1)"))
	}

	discountAmount := 0
	var promotionID *int

	if code != "" {
		ps := &PromotionService{}
		promoRes, _, err := ps.ValidatePromotion(ValidatePromotionReq{
			Code:       code,
			OrderValue: order.OriginalAmount,
		})
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		discountAmount = promoRes.DiscountAmount
		pID := promoRes.PromotionID
		promotionID = &pID

		// Tăng used_count (race condition safe)
		result := tx.Model(&model.Promotion{}).
			Where("id = ? AND used_count < usage_limit", promoRes.PromotionID).
			UpdateColumn("used_count", repository.DB.Raw("used_count + 1"))
		if result.Error != nil {
			tx.Rollback()
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			tx.Rollback()
			return nil, &AppError{Code: "PROMOTION_RACE_CONDITION", Status: 409, Msg: "voucher vừa hết lượt sử dụng."}
		}
	}

	finalAmount := order.OriginalAmount - discountAmount
	if finalAmount < 0 {
		finalAmount = 0
	}
	if err := tx.Model(&order).Updates(map[string]interface{}{
		"promotion_id":    promotionID,
		"discount_amount": discountAmount,
		"final_amount":    finalAmount,
	}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// Invalidate Cache
	if bookingredis.Client != nil {
		cacheKey := fmt.Sprintf("user:orders:%d", userID)
		bookingredis.Client.Del(context.Background(), cacheKey)
	}

	// Reload order với đầy đủ thông tin
	var updatedOrder model.Order
	repository.DB.Preload("Promotion").Where("id = ?", order.ID).First(&updatedOrder)
	return &updatedOrder, nil
}

func (s *OrderService) ListAdminOrders(page, limit int, q string) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	// Validate pagination
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	query := repository.DB.Model(&model.Order{})

	if q != "" {
		// Thử parse UUID nếu nó là mã đơn
		if _, err := uuid.Parse(q); err == nil {
			query = query.Where("orders.id = ?", q)
		} else {
			like := "%" + q + "%"
			query = query.Joins("JOIN users ON users.id = orders.user_id").
				Joins("JOIN showtimes ON showtimes.id = orders.showtime_id").
				Joins("JOIN movies ON movies.id = showtimes.movie_id").
				Where("users.full_name ILIKE ? OR users.email ILIKE ? OR movies.title ILIKE ?", like, like, like)
		}
	}

	// Calculate total using a clean subquery or cloning
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit

	// Use Session(&gorm.Session{}) to ensure a fresh statement and avoid cached plan errors
	if err := query.Session(&gorm.Session{}).Select("orders.*").
		Preload("User").Preload("Showtime.Movie").
		Order("orders.created_at DESC").Offset(offset).Limit(limit).Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	// Ẩn mật khẩu user
	for i := range orders {
		orders[i].User.PasswordHash = ""
	}

	return orders, total, nil
}

func (s *OrderService) ListAdminRefunds(page, limit int) ([]model.RefundRequest, int64, error) {
	var refunds []model.RefundRequest
	var total int64

	query := repository.DB.Model(&model.RefundRequest{})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.
		Preload("Order.User").
		Preload("Order.Showtime.Movie").
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&refunds).Error; err != nil {
		return nil, 0, err
	}

	// Ẩn mật khẩu user
	for i := range refunds {
		refunds[i].Order.User.PasswordHash = ""
	}

	return refunds, total, nil
}
