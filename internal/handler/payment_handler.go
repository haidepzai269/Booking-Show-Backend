package handler

import (
	"net/http"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type PaymentHandler struct {
	PaymentService *service.PaymentService
}

func NewPaymentHandler() *PaymentHandler {
	return &PaymentHandler{
		PaymentService: &service.PaymentService{},
	}
}

// Initiate — POST /api/v1/payments/initiate (user tạo URL thanh toán)
func (h *PaymentHandler) Initiate(c *gin.Context) {
	var req service.InitiatePaymentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error(), "code": "INVALID_INPUT"})
		return
	}

	userID := c.GetInt("userID")
	result, err := h.PaymentService.InitiatePayment(req, userID)
	if err != nil {
		if appErr, ok := service.IsAppError(err); ok {
			c.JSON(appErr.Status, gin.H{"success": false, "error": appErr.Msg, "code": appErr.Code})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "code": "PAYMENT_FAILED"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// VNPAYWebhook — GET /api/v1/payments/vnpay_return (redirect callback từ VNPay)
func (h *PaymentHandler) VNPAYWebhook(c *gin.Context) {
	// Lấy toàn bộ query string thô (raw query) để verify Hash chính xác do VNPay gen
	rawQuery := c.Request.URL.RawQuery

	var req service.VNPAYWebhookReq
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"Message": "Invalid request", "RspCode": "99"})
		return
	}

	if err := h.PaymentService.ProcessVNPAYWebhook(req, rawQuery); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Message": "Processing error", "RspCode": "99"})
		return
	}

	// VNPay yêu cầu redirect về frontend sau khi xử lý IPN
	c.JSON(http.StatusOK, gin.H{"Message": "Confirm Success", "RspCode": "00"})
}

// ZaloPayCallback — POST /api/v1/payments/zalopay/callback
// ZaloPay gọi về server này để thông báo kết quả thanh toán
func (h *PaymentHandler) ZaloPayCallback(c *gin.Context) {
	var req service.ZaloPayCallbackReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"return_code": -1, "return_message": "invalid request"})
		return
	}

	if err := h.PaymentService.ProcessZaloPayCallback(req); err != nil {
		// Trả về return_code 0 để ZaloPay không retry nếu là lỗi hệ thống,
		// hoặc return_code -1 nếu MAC không hợp lệ
		if err.Error() == "invalid MAC signature" {
			c.JSON(http.StatusOK, gin.H{"return_code": 0, "return_message": "mac invalid"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"return_code": 0, "return_message": err.Error()})
		return
	}

	// ZaloPay yêu cầu trả về return_code = 1 khi thành công
	c.JSON(http.StatusOK, gin.H{"return_code": 1, "return_message": "success"})
}

// PayOSWebhook — POST /api/v1/payments/payos/callback
// PayOS gọi về server này để thông báo kết quả thanh toán
func (h *PaymentHandler) PayOSWebhook(c *gin.Context) {
	var req service.PayOSWebhookReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}

	confirm, err := h.PaymentService.ProcessPayOSWebhook(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	// PayOS xác nhận đã nhận webhook
	c.JSON(http.StatusOK, confirm)
}

// CheckStatus — GET /api/v1/payments/check_status
// Frontend chủ động gọi để kích hoạt việc kiểm tra hóa đơn (ZaloPay, PayOS)
func (h *PaymentHandler) CheckStatus(c *gin.Context) {
	var req service.CheckPaymentStatusReq
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request parameters"})
		return
	}

	result, err := h.PaymentService.CheckPaymentStatus(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
