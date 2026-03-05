package service

import (
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/pkg/sse"
	"github.com/google/uuid"
)

type TicketService struct{}

// ProcessPaymentSuccess — được gọi từ RabbitMQ worker sau khi thanh toán thành công
func (s *TicketService) ProcessPaymentSuccess(orderIDStr, gateway, transactionID string) error {
	log.Printf("🎫 [ProcessPaymentSuccess] START — order=%s gateway=%s txn=%s", orderIDStr, gateway, transactionID)

	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		log.Printf("❌ [ProcessPaymentSuccess] Invalid UUID: %v", err)
		return err
	}

	tx := repository.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ [ProcessPaymentSuccess] PANIC: %v", r)
			tx.Rollback()
		}
	}()

	var order model.Order
	if err := tx.Where("id = ?", orderID).First(&order).Error; err != nil {
		log.Printf("❌ [ProcessPaymentSuccess] Order not found: %v", err)
		tx.Rollback()
		return err
	}
	log.Printf("✅ [ProcessPaymentSuccess] Order found — status=%s userID=%d showtimeID=%d", order.Status, order.UserID, order.ShowtimeID)

	if order.Status == model.OrderCompleted {
		log.Printf("⚠️  [ProcessPaymentSuccess] Order already COMPLETED — skip (idempotent)")
		tx.Rollback()
		return nil
	}

	// 🛡️ DOUBLE BOOKING GUARD: Nếu Order đã bị hủy (cronjob dọn dẹp), nghĩa là
	// thanh toán đến TRỄ sau khi đơn đã hết hạn.
	// Tuyệt đối không được cấp vé — phải tạo RefundRequest để hoàn tiền lại cho khách.
	if order.Status == model.OrderCancelled {
		log.Printf("🚨 [ProcessPaymentSuccess] Order %s was CANCELLED before payment arrived — creating RefundRequest", orderIDStr)
		refund := model.RefundRequest{
			OrderID:              order.ID,
			Gateway:              gateway,
			GatewayTransactionID: transactionID,
			Amount:               order.FinalAmount,
			Reason:               "Payment arrived after order expiry — seats may have been reassigned",
			Status:               "PENDING",
		}
		if err := tx.Create(&refund).Error; err != nil {
			log.Printf("❌ [ProcessPaymentSuccess] Failed to create RefundRequest: %v", err)
		}
		tx.Commit()
		// Trả về nil (không lỗi) để RabbitMQ không retry, nhưng vé KHÔNG được tạo
		return nil
	}

	// 1. Cập nhật Order sang COMPLETED
	order.Status = model.OrderCompleted
	if err := tx.Save(&order).Error; err != nil {
		log.Printf("❌ [ProcessPaymentSuccess] Failed to save order: %v", err)
		tx.Rollback()
		return err
	}

	// 2. Lấy ghế từ bảng order_seats (mapping cố định, không phụ thuộc LOCKED)
	var orderSeats []model.OrderSeat
	if err := tx.Where("order_id = ?", orderID).Find(&orderSeats).Error; err != nil {
		log.Printf("❌ [ProcessPaymentSuccess] Failed to query order_seats: %v", err)
		tx.Rollback()
		return err
	}
	log.Printf("✅ [ProcessPaymentSuccess] order_seats: %d records", len(orderSeats))

	if len(orderSeats) == 0 {
		// Fallback cho các order cũ (trước khi có order_seats table)
		log.Printf("⚠️  [ProcessPaymentSuccess] No order_seats — fallback to locked_by query")
		var fallbackSeats []model.ShowtimeSeat
		tx.Where("showtime_id = ? AND locked_by = ?", order.ShowtimeID, order.UserID).Find(&fallbackSeats)
		if len(fallbackSeats) == 0 {
			log.Printf("❌ [ProcessPaymentSuccess] No seats found by any method")
			tx.Rollback()
			return errors.New("no seats found for order: " + orderIDStr)
		}
		log.Printf("✅ [ProcessPaymentSuccess] Fallback: found %d seats", len(fallbackSeats))
		for _, s := range fallbackSeats {
			orderSeats = append(orderSeats, model.OrderSeat{OrderID: order.ID, ShowtimeSeatID: s.ID})
		}
	}

	seatIDs := make([]int, len(orderSeats))
	for i, os := range orderSeats {
		seatIDs[i] = os.ShowtimeSeatID
	}
	log.Printf("✅ [ProcessPaymentSuccess] seat IDs: %v", seatIDs)

	var showtimeSeats []model.ShowtimeSeat
	if err := tx.Where("id IN ?", seatIDs).Find(&showtimeSeats).Error; err != nil {
		log.Printf("❌ [ProcessPaymentSuccess] Failed to load showtime_seats: %v", err)
		tx.Rollback()
		return err
	}
	log.Printf("✅ [ProcessPaymentSuccess] showtime_seats loaded: %d", len(showtimeSeats))

	// 3. Tạo vé + BOOKED ghế
	var tickets []model.Ticket
	seatConflict := false

	for _, seat := range showtimeSeats {
		// 🛡️ SEAT CONFLICT GUARD: Kiểm tra ghế này đã bị BOOKED bởi người / đơn hàng khác chưa
		// (tức là một ticket khác đã tồn tại cho seat này, KHÔNG phải của order hiện tại)
		var conflictTicket model.Ticket
		if tx.Where("showtime_seat_id = ? AND order_id != ?", seat.ID, orderID).First(&conflictTicket).Error == nil {
			log.Printf("🚨 [ProcessPaymentSuccess] Seat %d already BOOKED by another order — skipping ticket, flagging refund", seat.ID)
			seatConflict = true
			continue // Bỏ qua ghế này, không tạo vé
		}

		if seat.Status != model.StatusBooked {
			seat.Status = model.StatusBooked
			seat.LockedBy = nil
			seat.LockedUntil = nil
			if err := tx.Save(&seat).Error; err != nil {
				log.Printf("❌ [ProcessPaymentSuccess] Failed to save seat %d: %v", seat.ID, err)
				tx.Rollback()
				return err
			}
		}

		// Idempotent: bỏ qua nếu vé đã tồn tại (của chính order này)
		var existing model.Ticket
		if tx.Where("showtime_seat_id = ? AND order_id = ?", seat.ID, orderID).First(&existing).Error == nil {
			log.Printf("⚠️  [ProcessPaymentSuccess] Ticket exists for seat %d — skip", seat.ID)
			continue
		}

		qrBytes, _ := json.Marshal(map[string]interface{}{
			"order_id": order.ID.String(),
			"seat_id":  seat.ID,
			"user_id":  order.UserID,
		})
		tickets = append(tickets, model.Ticket{
			OrderID:        order.ID,
			ShowtimeSeatID: seat.ID,
			QRCodeData:     string(qrBytes),
			IsUsed:         false,
		})
	}

	if len(tickets) > 0 {
		if err := tx.Create(&tickets).Error; err != nil {
			log.Printf("❌ [ProcessPaymentSuccess] Failed to create tickets: %v", err)
			tx.Rollback()
			return err
		}
		log.Printf("✅ [ProcessPaymentSuccess] Created %d tickets", len(tickets))
	} else {
		log.Printf("⚠️  [ProcessPaymentSuccess] No new tickets needed")
	}

	// 🛡️ Nếu có ghế bị chiếm bởi người khác, ghi nhận RefundRequest (một phần hoàn tiền)
	if seatConflict {
		log.Printf("🚨 [ProcessPaymentSuccess] Seat conflict detected — creating RefundRequest for order %s", orderIDStr)
		refund := model.RefundRequest{
			OrderID:              order.ID,
			Gateway:              gateway,
			GatewayTransactionID: transactionID,
			Amount:               order.FinalAmount,
			Reason:               "One or more seats were booked by another customer during payment processing",
			Status:               "PENDING",
		}
		if err := tx.Create(&refund).Error; err != nil {
			log.Printf("❌ [ProcessPaymentSuccess] Failed to create partial RefundRequest: %v", err)
		}
	}

	// 4. Cập nhật Payment record
	now := time.Now()
	result := tx.Model(&model.Payment{}).
		Where("order_id = ? AND gateway = ? AND status = ?", order.ID, gateway, "PENDING").
		Updates(map[string]interface{}{
			"status":                 "SUCCESS",
			"gateway_transaction_id": transactionID,
			"paid_at":                &now,
		})
	if result.Error != nil {
		log.Printf("❌ [ProcessPaymentSuccess] Failed to update payment: %v", result.Error)
		tx.Rollback()
		return result.Error
	}
	log.Printf("✅ [ProcessPaymentSuccess] Payment updated (%d rows)", result.RowsAffected)

	if result.RowsAffected == 0 {
		p := model.Payment{
			OrderID:              order.ID,
			Gateway:              gateway,
			GatewayTransactionID: transactionID,
			Amount:               order.FinalAmount,
			Status:               "SUCCESS",
			PaidAt:               &now,
		}
		if err := tx.Create(&p).Error; err != nil {
			log.Printf("❌ [ProcessPaymentSuccess] Fallback payment create failed: %v", err)
			tx.Rollback()
			return err
		}
		log.Printf("✅ [ProcessPaymentSuccess] Created fallback payment")
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("❌ [ProcessPaymentSuccess] COMMIT FAILED: %v", err)
		return err
	}
	log.Printf("🎉 [ProcessPaymentSuccess] SUCCESS — order=%s, %d tickets", orderIDStr, len(tickets))

	// 🔔 Push real-time notification đến Admin panel
	go func() {
		var user model.User
		var showtime model.Showtime
		repository.DB.First(&user, order.UserID)
		repository.DB.Preload("Movie").First(&showtime, order.ShowtimeID)
		movieTitle := ""
		if showtime.Movie.ID != 0 {
			movieTitle = showtime.Movie.Title
		}
		sse.BroadcastOrderCompleted(orderIDStr, user.FullName, movieTitle, order.FinalAmount, len(tickets))
	}()

	return nil
}

// MyTickets — danh sách vé của user
func (s *TicketService) MyTickets(userID int) ([]model.Ticket, error) {
	var tickets []model.Ticket
	if err := repository.DB.
		Joins("JOIN orders ON orders.id = tickets.order_id").
		Where("orders.user_id = ?", userID).
		Preload("ShowtimeSeat.Seat").
		Preload("ShowtimeSeat.Showtime.Movie").
		Preload("ShowtimeSeat.Showtime.Room.Cinema").
		Order("tickets.created_at DESC").
		Find(&tickets).Error; err != nil {
		return nil, err
	}
	return tickets, nil
}

// GetTicket — chi tiết 1 vé (chỉ user sở hữu hoặc staff)
func (s *TicketService) GetTicket(ticketIDStr string, userID int, isStaff bool) (*model.Ticket, error) {
	ticketID, err := uuid.Parse(ticketIDStr)
	if err != nil {
		return nil, errors.New("invalid ticket ID")
	}

	var ticket model.Ticket
	q := repository.DB.
		Preload("ShowtimeSeat.Seat").
		Preload("ShowtimeSeat.Showtime.Movie").
		Preload("ShowtimeSeat.Showtime.Room.Cinema").
		Where("tickets.id = ?", ticketID)

	if !isStaff {
		q = q.Joins("JOIN orders ON orders.id = tickets.order_id").
			Where("orders.user_id = ?", userID)
	}

	if err := q.First(&ticket).Error; err != nil {
		return nil, errors.New("ticket not found")
	}
	return &ticket, nil
}

// VerifyTicket — staff quét mã QR để xác thực vé
func (s *TicketService) VerifyTicket(ticketIDStr string) error {
	ticketID, err := uuid.Parse(ticketIDStr)
	if err != nil {
		return errors.New("invalid ticket ID")
	}

	tx := repository.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var ticket model.Ticket
	if err := tx.First(&ticket, "id = ?", ticketID).Error; err != nil {
		tx.Rollback()
		return errors.New("ticket not found")
	}
	if ticket.IsUsed {
		tx.Rollback()
		return errors.New("ticket already used")
	}

	now := time.Now()
	ticket.IsUsed = true
	ticket.UsedAt = &now
	if err := tx.Save(&ticket).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}
