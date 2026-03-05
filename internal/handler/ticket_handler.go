package handler

import (
	"net/http"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type TicketHandler struct {
	TicketService *service.TicketService
}

func NewTicketHandler() *TicketHandler {
	return &TicketHandler{
		TicketService: &service.TicketService{},
	}
}

// MyTickets — GET /tickets/my
func (h *TicketHandler) MyTickets(c *gin.Context) {
	userID := c.GetInt("userID")

	tickets, err := h.TicketService.MyTickets(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to fetch tickets"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": tickets})
}

// GetTicket — GET /tickets/:id
func (h *TicketHandler) GetTicket(c *gin.Context) {
	ticketID := c.Param("id")
	userID := c.GetInt("userID")
	role := c.GetString("role")
	isStaff := role == "ADMIN" || role == "CINEMA_MANAGER"

	ticket, err := h.TicketService.GetTicket(ticketID, userID, isStaff)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error(), "code": "NOT_FOUND"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": ticket})
}

// VerifyTicket — POST /tickets/:id/verify (staff only)
func (h *TicketHandler) VerifyTicket(c *gin.Context) {
	ticketID := c.Param("id")

	if err := h.TicketService.VerifyTicket(ticketID); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "ticket not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error(), "code": "VERIFY_FAILED"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Vé hợp lệ và đã được xác thực."})
}
