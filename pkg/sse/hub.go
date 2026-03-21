package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	bookingredis "github.com/booking-show/booking-show-api/pkg/redis"
)

type Client struct {
	Channel chan string
	UserID  int // Thêm UserID để debug
}

type Hub struct {
	clients map[*Client]bool
	mu      sync.Mutex
}

var hubs = make(map[int]*Hub)
var hubsMu sync.Mutex

func GetHub(showtimeID int) *Hub {
	hubsMu.Lock()
	defer hubsMu.Unlock()

	if h, ok := hubs[showtimeID]; ok {
		return h
	}

	h := &Hub{
		clients: make(map[*Client]bool),
	}
	hubs[showtimeID] = h
	return h
}

func (h *Hub) AddClient(c *Client) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
}

func (h *Hub) RemoveClient(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.Channel)
	}
	h.mu.Unlock()
}

func (h *Hub) Broadcast(message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients) == 0 {
		return
	}
	fmt.Printf("📢 [SSE HUB] Broadcasting to %d clients: %s\n", len(h.clients), message)
	for client := range h.clients {
		select {
		case client.Channel <- message:
		default:
			close(client.Channel)
			delete(h.clients, client)
		}
	}
}

type SeatUpdateEvent struct {
	ShowtimeID int    `json:"showtime_id"`
	SeatID     int    `json:"seat_id"`
	Status     string `json:"status"`
}

// BroadcastSeatUpdate Gửi sự kiện lên Redis Pub/Sub để tất cả các instance đều nhận được
func BroadcastSeatUpdate(showtimeID int, seatID int, status string) {
	event := SeatUpdateEvent{
		ShowtimeID: showtimeID,
		SeatID:     seatID,
		Status:     status,
	}
	payload, _ := json.Marshal(event)

	fmt.Printf("🚀 [Seat Update] ShowtimeID=%d, SeatID=%d, Status=%s\n", showtimeID, seatID, status)
	if bookingredis.Client != nil {
		fmt.Printf("🚀 [Redis PUBLISH] sse:seat_updates -> %s\n", string(payload))
		bookingredis.Client.Publish(context.Background(), "sse:seat_updates", payload)
	} else {
		// Nếu không có Redis, ta vẫn broadcast local cho instance hiện tại
		sseMsg := fmt.Sprintf(`{"seat_id": %d, "status": "%s"}`, seatID, status)
		GetHub(showtimeID).Broadcast(sseMsg)
	}
}

// StartSubscriber Lắng nghe các sự kiện từ Redis để cập nhật SSE Hub local
func StartSubscriber() {
	if bookingredis.Client == nil {
		fmt.Println("⚠️ [SSE Subscriber] Redis client is nil, skipping Redis subscription. Real-time sync across instances will be disabled.")
		return
	}
	pubsub := bookingredis.Client.Subscribe(context.Background(), "sse:seat_updates")
	defer pubsub.Close()

	fmt.Println("📡 [SSE Subscriber] Listening to Redis channel: sse:seat_updates")

	ch := pubsub.Channel()
	for msg := range ch {
		var event SeatUpdateEvent
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
			fmt.Printf("❌ [SSE Subscriber] Failed to unmarshal: %v\n", err)
			continue
		}

		fmt.Printf("📥 [Redis RECEIVE] sse:seat_updates -> Showtime=%d, Seat=%d, Status=%s\n",
			event.ShowtimeID, event.SeatID, event.Status)

		// Broadcast tới các client SSE đang kết nối vào instance này
		sseMsg := fmt.Sprintf(`{"seat_id": %d, "status": "%s"}`, event.SeatID, event.Status)
		GetHub(event.ShowtimeID).Broadcast(sseMsg)
	}
}
