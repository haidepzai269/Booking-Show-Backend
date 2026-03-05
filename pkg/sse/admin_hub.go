package sse

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// AdminNotification – payload gửi về admin client
type AdminNotification struct {
	Type       string `json:"type"` // "order_completed"
	OrderID    string `json:"order_id"`
	UserName   string `json:"user_name"`
	MovieTitle string `json:"movie_title"`
	Amount     int    `json:"amount"`
	Seats      int    `json:"seats"`
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
	log.Printf("[AdminHub] Client connected. Total: %d", len(h.clients))
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
	defer h.mu.Unlock()
	for client := range h.clients {
		select {
		case client.Channel <- message:
		default:
			close(client.Channel)
			delete(h.clients, client)
		}
	}
}

// BroadcastOrderCompleted – gọi từ ticket_service sau khi order COMPLETED
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
	data, _ := json.Marshal(notif)
	GetAdminHub().Broadcast(string(data))
	log.Printf("[AdminHub] Broadcasted order_completed: order=%s user=%s", orderID, userName)
}
