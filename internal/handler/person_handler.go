package handler

import (
	"net/http"
	"strconv"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type PersonHandler struct {
	PersonService *service.PersonService
}

func NewPersonHandler() *PersonHandler {
	return &PersonHandler{
		PersonService: &service.PersonService{},
	}
}

func (h *PersonHandler) GetPerson(c *gin.Context) {
	idStr := c.Param("id")
	tmdbID, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid person ID"})
		return
	}

	person, err := h.PersonService.GetPersonDetail(tmdbID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": person})
}
