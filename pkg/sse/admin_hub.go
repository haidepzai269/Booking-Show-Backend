package sse

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	bookingredis "github.com/booking-show/booking-show-api/pkg/redis"
)

// AdminNotification – payload gửi về admin client
type AdminNotification struct {
	Type       string `json:"type"` // "order_completed", "order_cancelled"
	OrderID    string `json:"order_id"`
	UserName   string `json:"user_name,omitempty"`
	MovieTitle string `json:"movie_title,omitempty"`
	Amount     int    `json:"amount,omitempty"`
	Seats      int    `json:"seats,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// adminHub – singleton hub dành cho admin notifications
type adminHub struct {
	clients map[*Client]bool
	mu      sync.Mutex
}

var globalAdminHub = &adminHub{
	clients: make(map[*Client]bool),
}

// GetAdminHub trả về admin hub global
func GetAdminHub() *adminHub {
	return globalAdminHub
}

func (h *adminHub) AddClient(c *Client) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	log.Printf("📥 [AdminHub] Client added. Total clients: %d", len(h.clients))
}

func (h *adminHub) RemoveClient(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.Channel)
	}
	h.mu.Unlock()
	log.Printf("[AdminHub] Client disconnected. Total: %d", len(h.clients))
}

func (h *adminHub) Broadcast(message string) {
	h.mu.Lock()
	count := len(h.clients)
	h.mu.Unlock()

	log.Printf("📢 [AdminHub] Broadcasting message to %d clients", count)
	
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		log.Printf("  -> Sending to Client (UserID: %d)", client.UserID)
		select {
		case client.Channel <- message:
		default:
			log.Printf("⚠️ [AdminHub] Client %d channel full or slow, removing", client.UserID)
			close(client.Channel)
			delete(h.clients, client)
		}
	}
}

// BroadcastOrderCompleted – gọi từ ticket_service sau khi order COMPLETED
// Gửi lên Redis channel để đồng bộ các node
func BroadcastOrderCompleted(orderID, userName, movieTitle string, amount, seats int) {
	notif := AdminNotification{
		Type:       "order_completed",
		OrderID:    orderID,
		UserName:   userName,
		MovieTitle: movieTitle,
		Amount:     amount,
		Seats:      seats,
		CreatedAt:  time.Now().Format(time.RFC3339),
	}
	publishAdminNotification(notif)
}

// BroadcastOrderCancelled – gọi khi đơn hàng bị hủy
func BroadcastOrderCancelled(orderID string) {
	notif := AdminNotification{
		Type:      "order_cancelled",
		OrderID:   orderID,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	publishAdminNotification(notif)
}

func publishAdminNotification(notif AdminNotification) {
	data, _ := json.Marshal(notif)
	
	if bookingredis.Client != nil {
		err := bookingredis.Client.Publish(context.Background(), "sse:admin_notifications", data).Err()
		if err != nil {
			log.Printf("❌ [AdminHub] Redis PUBLISH FAILED: %v", err)
			// Nếu Redis lỗi, fallback broadcast local để đảm bảo admin hiện tại vẫn nhận được
			GetAdminHub().Broadcast(string(data))
		} else {
			log.Printf("[AdminHub] Redis PUBLISH SUCCESS: sse:admin_notifications (will be broadcasted by Subscriber)")
		}
	} else {
		// Nếu không dùng Redis (local mode), broadcast trực tiếp
		log.Printf("[AdminHub] No Redis, performing LOCAL Broadcast...")
		GetAdminHub().Broadcast(string(data))
	}
}

// StartAdminSubscriber Lắng nghe các sự kiện admin từ Redis
func StartAdminSubscriber() {
	if bookingredis.Client == nil {
		log.Println("⚠️ [AdminSubscriber] Redis client is nil, skipping Admin subscription.")
		return
	}
	pubsub := bookingredis.Client.Subscribe(context.Background(), "sse:admin_notifications")
	defer pubsub.Close()

	log.Println("📡 [AdminSubscriber] Started listening to Redis channel: sse:admin_notifications")

	ch := pubsub.Channel()
	for msg := range ch {
		log.Printf("📥 [AdminSubscriber] Received from Redis: %s", msg.Payload)
		// Broadcast tới các client SSE đang kết nối vào instance này
		GetAdminHub().Broadcast(msg.Payload)
	}
}

// BroadcastRoomUpdated – gọi khi thông tin phòng chiếu thay đổi (tên, sức chứa)
func BroadcastRoomUpdated(roomID int, cinemaID int, name string, capacity int) {
	notif := map[string]interface{}{
		"type":      "room_updated",
		"room_id":   roomID,
		"cinema_id": cinemaID,
		"name":      name,
		"capacity":  capacity,
		"at":        time.Now().Format(time.RFC3339),
	}
	data, _ := json.Marshal(notif)
	// Tạm thời cũng publish vào admin_notifications để đồng bộ
	if bookingredis.Client != nil {
		bookingredis.Client.Publish(context.Background(), "sse:admin_notifications", data)
	} else {
		GetAdminHub().Broadcast(string(data))
	}
	log.Printf("[AdminHub] Broadcasted room_updated: id=%d name=%s", roomID, name)
}
