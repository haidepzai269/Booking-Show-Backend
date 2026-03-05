package handler

import (
	"net/http"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type OrderHandler struct {
	OrderService *service.OrderService
}

func NewOrderHandler() *OrderHandler {
	return &OrderHandler{
		OrderService: &service.OrderService{},
	}
}

func (h *OrderHandler) CreateOrder(c *gin.Context) {
	var req service.CreateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error(), "code": "INVALID_INPUT"})
		return
	}

	userID := c.GetInt("userID")
	order, err := h.OrderService.CreateOrder(req, userID)
	if err != nil {
		status, code := http.StatusBadRequest, "ORDER_FAILED"
		if appErr, ok := service.IsAppError(err); ok {
			status = appErr.Status
			code = appErr.Code
			c.JSON(status, gin.H{"success": false, "error": appErr.Msg, "code": code})
			return
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error(), "code": code})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": order})
}

func (h *OrderHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("id")
	userID := c.GetInt("userID")
	role := c.GetString("role")
	isAdmin := role == "ADMIN" || role == "CINEMA_MANAGER"

	order, err := h.OrderService.GetOrder(orderID, userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error(), "code": "NOT_FOUND"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": order})
}

func (h *OrderHandler) MyOrders(c *gin.Context) {
	userID := c.GetInt("userID")

	orders, err := h.OrderService.MyOrders(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to fetch orders"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": orders})
}

func (h *OrderHandler) CancelOrder(c *gin.Context) {
	orderID := c.Param("id")
	userID := c.GetInt("userID")

	if err := h.OrderService.CancelOrder(orderID, userID); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "order not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error(), "code": "CANCEL_FAILED"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Đơn hàng đã được hủy thành công."})
}

func (h *OrderHandler) UpdateOrderConcessions(c *gin.Context) {
	orderID := c.Param("id")
	userID := c.GetInt("userID")

	var req struct {
		Items []service.ConcessionItemRequest `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if err := h.OrderService.UpdateOrderConcessions(orderID, userID, req.Items); err != nil {
		status := http.StatusBadRequest
		if appErr, ok := service.IsAppError(err); ok {
			status = appErr.Status
			c.JSON(status, gin.H{"success": false, "error": appErr.Msg, "code": appErr.Code})
			return
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Cập nhật bắp nước thành công."})
}

func (h *OrderHandler) ApplyOrderVoucher(c *gin.Context) {
	orderID := c.Param("id")
	userID := c.GetInt("userID")

	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	updatedOrder, err := h.OrderService.ApplyOrderVoucher(orderID, userID, req.Code)
	if err != nil {
		status := http.StatusBadRequest
		if appErr, ok := service.IsAppError(err); ok {
			status = appErr.Status
			c.JSON(status, gin.H{"success": false, "error": appErr.Msg, "code": appErr.Code})
			return
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": updatedOrder})
}
