package sse

import (
	"fmt"
	"sync"
)

type Client struct {
	Channel chan string
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
	for client := range h.clients {
		select {
		case client.Channel <- message:
		default:
			close(client.Channel)
			delete(h.clients, client)
		}
	}
}

// BroadcastSeatUpdate Gửi JSON event trạng thái ghế về client
func BroadcastSeatUpdate(showtimeID int, seatID int, status string) {
	msg := fmt.Sprintf(`{"seat_id": %d, "status": "%s"}`, seatID, status)
	GetHub(showtimeID).Broadcast(msg)
}
