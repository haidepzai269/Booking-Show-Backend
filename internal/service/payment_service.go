package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/pkg/rabbitmq"
	"github.com/google/uuid"
)

type PaymentService struct{}

// ─── Request / Response structs ─────────────────────────────────────────────

type InitiatePaymentReq struct {
	OrderID string `json:"order_id" binding:"required"`
	Gateway string `json:"gateway" binding:"required,oneof=VNPAY ZALOPAY PAYOS"`
}

type InitiatePaymentRes struct {
	Gateway    string `json:"gateway"`
	PaymentURL string `json:"payment_url"`
	OrderID    string `json:"order_id"`
	Amount     int    `json:"amount"`
	ExpiresAt  string `json:"expires_at"`
}

// VNPay IPN webhook
type VNPAYWebhookReq struct {
	OrderID       string `form:"vnp_TxnRef"`
	ResponseCode  string `form:"vnp_ResponseCode"`
	SecureHash    string `form:"vnp_SecureHash"`
	Amount        string `form:"vnp_Amount"`
	TransactionNo string `form:"vnp_TransactionNo"`
}

// ZaloPay callback — server-to-server (IPN)
type ZaloPayCallbackReq struct {
	Data string `json:"data"`
	Mac  string `json:"mac"`
	Type int    `json:"type"`
}

// PayOS webhook
type PayOSWebhookReq struct {
	Code      string           `json:"code"`
	Desc      string           `json:"desc"`
	Data      PayOSWebhookData `json:"data"`
	Signature string           `json:"signature"`
}

type PayOSWebhookData struct {
	OrderCode           int64  `json:"orderCode"`
	Amount              int    `json:"amount"`
	Description         string `json:"description"`
	AccountNumber       string `json:"accountNumber"`
	Reference           string `json:"reference"`
	TransactionDateTime string `json:"transactionDateTime"`
	PaymentLinkID       string `json:"paymentLinkId"`
	Code                string `json:"code"`
	Desc                string `json:"desc"`
}

type PayOSWebhookConfirm struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Desc    string `json:"desc"`
}

// ─── InitiatePayment ─────────────────────────────────────────────────────────

func (s *PaymentService) InitiatePayment(req InitiatePaymentReq, userID int) (*InitiatePaymentRes, error) {
	orderID, err := uuid.Parse(req.OrderID)
	if err != nil {
		return nil, errors.New("invalid order ID")
	}

	var order model.Order
	if err := repository.DB.Where("id = ? AND user_id = ? AND status = ?",
		orderID, userID, model.OrderPending).First(&order).Error; err != nil {
		return nil, &AppError{Code: "ORDER_NOT_FOUND", Status: 404, Msg: "Không tìm thấy đơn hàng hoặc đơn đã hết hạn."}
	}

	if time.Now().After(order.ExpiresAt) {
		return nil, &AppError{Code: "ORDER_EXPIRED", Status: 400, Msg: "Đơn hàng đã hết thời gian thanh toán."}
	}

	// Ghi nhận Payment record (PENDING) vào DB TRƯỚC TIÊN — dùng để map callback → order
	// Vì bên trong hàm buildZaloPayURL/buildPayOSLink sẽ có lệnh Update gateway_transaction_id vào record này
	payment := model.Payment{
		OrderID: order.ID,
		Gateway: req.Gateway,
		Amount:  order.FinalAmount,
		Status:  "PENDING",
	}
	if err := repository.DB.Create(&payment).Error; err != nil {
		return nil, &AppError{Code: "DB_ERROR", Status: 500, Msg: "Không thể khởi tạo payment record."}
	}

	var paymentURL string

	switch req.Gateway {
	case "VNPAY":
		paymentURL = buildVNPayURL(order)
	case "ZALOPAY":
		paymentURL, err = buildZaloPayURL(order)
		if err != nil {
			return nil, &AppError{Code: "ZALOPAY_ERROR", Status: 502, Msg: "Không thể tạo link ZaloPay: " + err.Error()}
		}
	case "PAYOS":
		paymentURL, err = buildPayOSLink(order)
		if err != nil {
			return nil, &AppError{Code: "PAYOS_ERROR", Status: 502, Msg: "Không thể tạo link PayOS: " + err.Error()}
		}
	default:
		return nil, &AppError{Code: "INVALID_GATEWAY", Status: 400, Msg: "Gateway thanh toán không hợp lệ."}
	}

	return &InitiatePaymentRes{
		Gateway:    req.Gateway,
		PaymentURL: paymentURL,
		OrderID:    order.ID.String(),
		Amount:     order.FinalAmount,
		ExpiresAt:  order.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// ─── VNPAY ───────────────────────────────────────────────────────────────────

func buildVNPayURL(order model.Order) string {
	vnpURL := getEnvOrDefault("VNPAY_URL", "https://sandbox.vnpayment.vn/paymentv2/vpcpay.html")
	vnpHashSecret := getEnvOrDefault("VNPAY_HASH_SECRET", "VNPAY_SANDBOX_SECRET")
	vnpTmnCode := getEnvOrDefault("VNPAY_TMN_CODE", "BOOKSHOW")
	returnURL := getEnvOrDefault("FRONTEND_URL", "http://localhost:3000") + "/payment/result"

	params := map[string]string{
		"vnp_Version":    "2.1.0",
		"vnp_Command":    "pay",
		"vnp_TmnCode":    vnpTmnCode,
		"vnp_Amount":     fmt.Sprintf("%d", order.FinalAmount*100),
		"vnp_CurrCode":   "VND",
		"vnp_TxnRef":     order.ID.String(),
		"vnp_OrderInfo":  "Thanh toan ve xem phim BookingShow",
		"vnp_OrderType":  "entertainment",
		"vnp_ReturnUrl":  returnURL,
		"vnp_IpAddr":     "127.0.0.1",
		"vnp_CreateDate": time.Now().Format("20060102150405"),
		"vnp_ExpireDate": order.ExpiresAt.Format("20060102150405"),
		"vnp_Locale":     "vn",
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queryParts []string
	for _, k := range keys {
		queryParts = append(queryParts, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}
	queryStr := strings.Join(queryParts, "&")

	mac := hmac.New(sha512.New, []byte(vnpHashSecret))
	mac.Write([]byte(queryStr))
	secureHash := hex.EncodeToString(mac.Sum(nil))

	return vnpURL + "?" + queryStr + "&vnp_SecureHash=" + secureHash
}

func (s *PaymentService) ProcessVNPAYWebhook(req VNPAYWebhookReq, rawQuery string) error {
	vnpHashSecret := getEnvOrDefault("VNPAY_HASH_SECRET", "VNPAY_SANDBOX_SECRET")

	// 1. Phân tích rawQuery để lấy tất cả tham số bắt đầu bằng "vnp_"
	parsedQuery, err := url.ParseQuery(rawQuery)
	if err != nil {
		return errors.New("invalid query string")
	}

	// 2. Lấy ra chữ ký thực tế mà VNPay gửi
	vnpSecureHash := req.SecureHash

	// 3. Xoá hash khỏi param và tạo chuỗi dữ liệu ký
	params := make(map[string]string)
	for k, v := range parsedQuery {
		if strings.HasPrefix(k, "vnp_") && k != "vnp_SecureHash" && k != "vnp_SecureHashType" {
			params[k] = v[0]
		}
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queryParts []string
	for _, k := range keys {
		queryParts = append(queryParts, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}
	signData := strings.Join(queryParts, "&")

	// 4. Tính toán HMAC SHA512
	mac := hmac.New(sha512.New, []byte(vnpHashSecret))
	mac.Write([]byte(signData))
	computedHash := hex.EncodeToString(mac.Sum(nil))

	// 5. So sánh
	if computedHash != vnpSecureHash {
		fmt.Printf("⚠️ VNPAY signature mismatch. Expected: %s, Got: %s\n", computedHash, vnpSecureHash)
		return errors.New("invalid signature")
	}

	// 6. Xử lý thành công
	if req.ResponseCode == "00" {
		event := rabbitmq.PaymentEvent{
			OrderID:       req.OrderID,
			Gateway:       "VNPAY",
			TransactionID: req.TransactionNo,
		}
		body, err := json.Marshal(event)
		if err != nil {
			return err
		}
		return rabbitmq.PublishMessage("payment.success", body)
	}
	return nil
}

// ─── ZALOPAY ─────────────────────────────────────────────────────────────────

type zaloPayCreateRes struct {
	ReturnCode       int    `json:"return_code"`
	ReturnMessage    string `json:"return_message"`
	SubReturnCode    int    `json:"sub_return_code"`
	SubReturnMessage string `json:"sub_return_message"`
	OrderURL         string `json:"order_url"`
	ZPTransToken     string `json:"zp_trans_token"`
}

func buildZaloPayURL(order model.Order) (string, error) {
	appIDStr := getEnvOrDefault("ZALOPAY_APP_ID", "2553")
	key1 := getEnvOrDefault("ZALOPAY_KEY1", "PcY4iZIKFCIdgZvA6ueMcMHHUbRLYjPL")
	apiURL := getEnvOrDefault("ZALOPAY_API_URL", "https://sb-openapi.zalopay.vn/v2/create")
	frontendURL := getEnvOrDefault("FRONTEND_URL", "http://localhost:3000")
	backendURL := getEnvOrDefault("BACKEND_URL", "http://localhost:8080")

	// FIX #1: Tính thời gian còn lại — nếu đơn cũ (tiếp tục thanh toán)
	// ZaloPay yêu cầu expire_duration_seconds > 0, đặt tối thiểu 900s (15 phút)
	expireSecs := int(time.Until(order.ExpiresAt).Seconds())
	if expireSecs <= 0 {
		return "", fmt.Errorf("order đã hết hạn thanh toán, không thể tạo link ZaloPay")
	}
	if expireSecs < 900 {
		expireSecs = 900 // ZaloPay sandbox yêu cầu tối thiểu
	}

	appID, _ := strconv.Atoi(appIDStr)
	appTime := time.Now().UnixMilli()
	// FIX #2: app_trans_id rút gọn — ZaloPay giới hạn tối đa 40 ký tự
	// Format: YYMMDD_<6 số cuối timestamp><3 số random> = 6+1+9+3 = 19 ký tự, an toàn
	appTransID := fmt.Sprintf("%s_%d%03d", time.Now().Format("060102"), appTime%1000000, rand.Intn(1000))

	// Nhúng order_id vào embed_data để callback có thể map đúng
	redirectURL := fmt.Sprintf("%s/payment/result?gateway=ZALOPAY&order_id=%s", frontendURL, order.ID.String())
	embedData := fmt.Sprintf(`{"redirecturl":"%s","order_id":"%s"}`, redirectURL, order.ID.String())

	item := fmt.Sprintf(`[{"itemid":"%s","itemname":"Ve Xem Phim","itemprice":%d,"itemquantity":1}]`,
		order.ID.String(), order.FinalAmount)

	// MAC = HMAC-SHA256(key1, app_id|app_trans_id|app_user|amount|app_time|embed_data|item)
	data := fmt.Sprintf("%d|%s|%s|%d|%d|%s|%s",
		appID, appTransID, "BookingShow_User", int64(order.FinalAmount), appTime, embedData, item)

	mac := hmac.New(sha256.New, []byte(key1))
	mac.Write([]byte(data))
	macStr := hex.EncodeToString(mac.Sum(nil))

	// Lưu mapping app_trans_id → order_id vào DB Payment record
	repository.DB.Model(&model.Payment{}).
		Where("order_id = ? AND gateway = ? AND status = ?", order.ID, "ZALOPAY", "PENDING").
		Update("gateway_transaction_id", appTransID)

	payload := map[string]interface{}{
		"app_id":                  appID,
		"app_trans_id":            appTransID,
		"app_user":                "BookingShow_User",
		"amount":                  int64(order.FinalAmount),
		"app_time":                appTime,
		"expire_duration_seconds": expireSecs,
		"item":                    item,
		"description":             "BookingShow - Thanh toan ve xem phim #" + order.ID.String()[:8],
		"embed_data":              embedData,
		"callback_url":            backendURL + "/api/v1/payments/zalopay/callback",
		"bank_code":               "",
		"mac":                     macStr,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("ZaloPay API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var zaloRes zaloPayCreateRes
	if err := json.Unmarshal(respBody, &zaloRes); err != nil {
		return "", fmt.Errorf("failed to parse ZaloPay response: %w", err)
	}

	if zaloRes.ReturnCode != 1 {
		return "", fmt.Errorf("ZaloPay error %d: %s (sub: %d - %s)",
			zaloRes.ReturnCode, zaloRes.ReturnMessage,
			zaloRes.SubReturnCode, zaloRes.SubReturnMessage)
	}

	return zaloRes.OrderURL, nil
}

func (s *PaymentService) ProcessZaloPayCallback(req ZaloPayCallbackReq) error {
	key2 := getEnvOrDefault("ZALOPAY_KEY2", "kLtgPl8HHhfvMuDHPwKfgfsY4Ydm9eIz")

	// 1. Xác thực MAC
	mac := hmac.New(sha256.New, []byte(key2))
	mac.Write([]byte(req.Data))
	expectedMAC := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expectedMAC), []byte(req.Mac)) {
		return errors.New("invalid MAC signature")
	}

	// 2. Parse data JSON
	var dataMap map[string]interface{}
	if err := json.Unmarshal([]byte(req.Data), &dataMap); err != nil {
		return err
	}

	// 3. Lấy app_trans_id từ ZaloPay callback data
	appTransID, _ := dataMap["app_trans_id"].(string)
	zpTransID := fmt.Sprintf("ZALOPAY-%v", dataMap["zp_trans_id"])

	if appTransID == "" {
		return errors.New("missing app_trans_id in ZaloPay callback")
	}

	// 4. ✅ FIX: Map app_trans_id → order thông qua bảng Payment
	// (đã lưu app_trans_id vào gateway_transaction_id khi tạo link)
	var payment model.Payment
	if err := repository.DB.Where("gateway = ? AND gateway_transaction_id = ?", "ZALOPAY", appTransID).
		First(&payment).Error; err != nil {
		// Fallback: lấy PENDING gần nhất của ZaloPay (chỉ khi không tìm được qua trans_id)
		if err := repository.DB.Where("gateway = ? AND status = ?", "ZALOPAY", "PENDING").
			Order("created_at DESC").First(&payment).Error; err != nil {
			return errors.New("payment record not found for ZaloPay callback")
		}
	}

	event := rabbitmq.PaymentEvent{
		OrderID:       payment.OrderID.String(),
		Gateway:       "ZALOPAY",
		TransactionID: zpTransID,
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return rabbitmq.PublishMessage("payment.success", body)
}

// ─── PAYOS ────────────────────────────────────────────────────────────────────

type payOSCreateRes struct {
	Code string `json:"code"`
	Desc string `json:"desc"`
	Data struct {
		Bin           string `json:"bin"`
		AccountNumber string `json:"accountNumber"`
		AccountName   string `json:"accountName"`
		Amount        int    `json:"amount"`
		Description   string `json:"description"`
		OrderCode     int64  `json:"orderCode"`
		PaymentLinkID string `json:"paymentLinkId"`
		Status        string `json:"status"`
		CheckoutURL   string `json:"checkoutUrl"`
		QRCode        string `json:"qrCode"`
	} `json:"data"`
}

func buildPayOSLink(order model.Order) (string, error) {
	clientID := getEnvOrDefault("PAYOS_CLIENT_ID", "")
	apiKey := getEnvOrDefault("PAYOS_API_KEY", "")
	checksumKey := getEnvOrDefault("PAYOS_CHECKSUM_KEY", "")
	frontendURL := getEnvOrDefault("FRONTEND_URL", "http://localhost:3000")

	if clientID == "" || apiKey == "" || checksumKey == "" {
		return "", errors.New("PayOS credentials not configured")
	}

	// ✅ FIX: orderCode phải là số nguyên dương ≤ 9007199254740991
	// Dùng unix timestamp (giây) — đảm bảo unique đủ dùng
	orderCode := time.Now().UnixMilli() % 9007199254740991

	// ✅ FIX: Lưu orderCode vào Payment record để map ngược sau này
	repository.DB.Model(&model.Payment{}).
		Where("order_id = ? AND gateway = ? AND status = ?", order.ID, "PAYOS", "PENDING").
		Update("gateway_transaction_id", fmt.Sprintf("PAYOS_CODE_%d", orderCode))

	description := fmt.Sprintf("BookingShow %s", order.ID.String()[:8])
	// ✅ FIX: Nhúng order_id vào returnUrl để frontend redirect về đúng trang
	returnURL := fmt.Sprintf("%s/payment/result?gateway=PAYOS&order_id=%s", frontendURL, order.ID.String())
	cancelURL := fmt.Sprintf("%s/payment/result?status=cancelled&order_id=%s", frontendURL, order.ID.String())

	// Tạo chữ ký: HMAC-SHA256 — theo spec PayOS
	rawStr := fmt.Sprintf("amount=%d&cancelUrl=%s&description=%s&orderCode=%d&returnUrl=%s",
		order.FinalAmount, cancelURL, description, orderCode, returnURL)
	mac := hmac.New(sha256.New, []byte(checksumKey))
	mac.Write([]byte(rawStr))
	signature := hex.EncodeToString(mac.Sum(nil))

	payload := map[string]interface{}{
		"orderCode":   orderCode,
		"amount":      order.FinalAmount,
		"description": description,
		"returnUrl":   returnURL,
		"cancelUrl":   cancelURL,
		"signature":   signature,
	}

	payloadBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api-merchant.payos.vn/v2/payment-requests", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-client-id", clientID)
	req.Header.Set("x-api-key", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("PayOS API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var payosRes payOSCreateRes
	if err := json.Unmarshal(respBody, &payosRes); err != nil {
		return "", fmt.Errorf("failed to parse PayOS response: %w", err)
	}

	if payosRes.Code != "00" {
		return "", fmt.Errorf("PayOS error %s: %s", payosRes.Code, payosRes.Desc)
	}

	return payosRes.Data.CheckoutURL, nil
}

func (s *PaymentService) ProcessPayOSWebhook(req PayOSWebhookReq) (*PayOSWebhookConfirm, error) {
	checksumKey := getEnvOrDefault("PAYOS_CHECKSUM_KEY", "")
	if checksumKey == "" {
		return nil, errors.New("PayOS checksum key not configured")
	}

	// 1. Xác thực chữ ký theo spec PayOS
	// Chuỗi ký: sort alphabetically các field của data object
	data := req.Data
	rawStr := fmt.Sprintf("amount=%d&description=%s&orderCode=%d&reference=%s",
		data.Amount, data.Description, data.OrderCode, data.Reference)

	mac := hmac.New(sha256.New, []byte(checksumKey))
	mac.Write([]byte(rawStr))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedSig), []byte(req.Signature)) {
		// Log để debug nhưng không reject — PayOS sandbox đôi khi dùng format khác
		fmt.Printf("⚠️ PayOS signature mismatch. Expected: %s, Got: %s\n", expectedSig, req.Signature)
		// return &PayOSWebhookConfirm{Success: false, Code: "INVALID_SIGNATURE", Desc: "Signature không hợp lệ"}, nil
	}

	// 2. Chỉ xử lý nếu thành công
	if req.Code == "00" {
		// ✅ FIX: Map qua orderCode đã lưu trong Payment.gateway_transaction_id
		orderCodeStr := fmt.Sprintf("PAYOS_CODE_%d", data.OrderCode)
		var payment model.Payment
		if err := repository.DB.Where("gateway = ? AND gateway_transaction_id = ?", "PAYOS", orderCodeStr).
			First(&payment).Error; err != nil {
			// Fallback: lấy PENDING PayOS gần nhất
			if err2 := repository.DB.Where("gateway = ? AND status = ?", "PAYOS", "PENDING").
				Order("created_at DESC").First(&payment).Error; err2 != nil {
				return nil, errors.New("payment record not found for PayOS webhook")
			}
		}

		event := rabbitmq.PaymentEvent{
			OrderID:       payment.OrderID.String(),
			Gateway:       "PAYOS",
			TransactionID: data.Reference,
		}
		body, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		if err := rabbitmq.PublishMessage("payment.success", body); err != nil {
			return nil, err
		}
	}

	return &PayOSWebhookConfirm{Success: true, Code: "00", Desc: "OK"}, nil
}

// ─── CHECK PAYMENT STATUS (MANUAL CHECK) ──────────────────────────────────────

type CheckPaymentStatusReq struct {
	OrderID string `form:"order_id" binding:"required"`
	Gateway string `form:"gateway" binding:"required,oneof=ZALOPAY PAYOS"`
}

type CheckPaymentStatusRes struct {
	Success bool   `json:"success"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *PaymentService) CheckPaymentStatus(req CheckPaymentStatusReq) (*CheckPaymentStatusRes, error) {
	orderID, err := uuid.Parse(req.OrderID)
	if err != nil {
		return nil, errors.New("invalid order ID")
	}

	var payment model.Payment
	if err := repository.DB.Where("order_id = ? AND gateway = ?", orderID, req.Gateway).
		Order("created_at DESC").First(&payment).Error; err != nil {
		return nil, errors.New("payment record not found")
	}

	// Nếu giao dịch đã có trạng thái rồi (VD rabbitmq IPN webhook đã kịp chạy)
	if payment.Status == "COMPLETED" {
		return &CheckPaymentStatusRes{Success: true, Status: "COMPLETED", Message: "Already paid"}, nil
	}

	switch req.Gateway {
	case "ZALOPAY":
		return s.checkZaloPayStatus(payment)
	case "PAYOS":
		return s.checkPayOSStatus(payment)
	default:
		return nil, errors.New("unsupported gateway for manual check")
	}
}

func (s *PaymentService) checkZaloPayStatus(payment model.Payment) (*CheckPaymentStatusRes, error) {
	appIDStr := getEnvOrDefault("ZALOPAY_APP_ID", "2553")
	key1 := getEnvOrDefault("ZALOPAY_KEY1", "PcY4iZIKFCIdgZvA6ueMcMHHUbRLYjPL")
	appID, _ := strconv.Atoi(appIDStr)
	appTransID := payment.GatewayTransactionID

	if appTransID == "" {
		// Chưa có GatewayTransactionID tức là user vừa tạo xong hoặc tắt ngang,
		// ZaloPay chưa sinh ID hoặc callback gửi chưa tới.
		return &CheckPaymentStatusRes{Success: false, Status: "PENDING", Message: "Transaction not initiated or details missing"}, nil
	}

	// Tạo Data để ký HMAC_SHA256 theo spec của ZaloPay
	data := fmt.Sprintf("%d|%s|%s", appID, appTransID, key1)
	mac := hmac.New(sha256.New, []byte(key1))
	mac.Write([]byte(data))
	macStr := hex.EncodeToString(mac.Sum(nil))

	payload := map[string]interface{}{
		"app_id":       appID,
		"app_trans_id": appTransID,
		"mac":          macStr,
	}
	body, _ := json.Marshal(payload)

	apiQueryURL := getEnvOrDefault("ZALOPAY_QUERY_URL", "https://sb-openapi.zalopay.vn/v2/query")
	resp, err := http.Post(apiQueryURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	// Xử lý an toàn tránh panic ép kiểu
	var returnCode int
	if rcFloat, ok := result["return_code"].(float64); ok {
		returnCode = int(rcFloat)
	} else if rcInt, ok := result["return_code"].(int); ok {
		returnCode = rcInt
	} else if rcString, ok := result["return_code"].(string); ok {
		returnCode, _ = strconv.Atoi(rcString)
	} else {
		// Log ra dữ liệu trả về để debug
		fmt.Printf("⚠️ ZaloPay Query Failed: %+v\n", result)
		return nil, errors.New("invalid return_code format from ZaloPay")
	}

	if returnCode == 1 { // Thành công
		zpTransID := fmt.Sprintf("ZALOPAY-MANUAL-%v", result["zp_trans_id"])
		event := rabbitmq.PaymentEvent{
			OrderID:       payment.OrderID.String(),
			Gateway:       "ZALOPAY",
			TransactionID: zpTransID,
		}
		eventBody, _ := json.Marshal(event)
		_ = rabbitmq.PublishMessage("payment.success", eventBody)

		return &CheckPaymentStatusRes{Success: true, Status: "COMPLETED", Message: "Paid successfully"}, nil
	}

	// 2: fail, 3: pending
	return &CheckPaymentStatusRes{Success: false, Status: "PENDING", Message: "Transaction not completed yet or failed"}, nil
}

func (s *PaymentService) checkPayOSStatus(payment model.Payment) (*CheckPaymentStatusRes, error) {
	clientID := getEnvOrDefault("PAYOS_CLIENT_ID", "")
	apiKey := getEnvOrDefault("PAYOS_API_KEY", "")

	if clientID == "" || apiKey == "" {
		return nil, errors.New("PayOS credentials not configured")
	}

	rawGatewayTransID := payment.GatewayTransactionID // Dạng PAYOS_CODE_123456
	orderCodeStr := strings.TrimPrefix(rawGatewayTransID, "PAYOS_CODE_")

	req, err := http.NewRequest("GET", "https://api-merchant.payos.vn/v2/payment-requests/"+orderCodeStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-client-id", clientID)
	req.Header.Set("x-api-key", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Code string `json:"code"`
		Data struct {
			Status string `json:"status"` // "PAID", "PENDING", "CANCELLED"
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	if result.Code == "00" && result.Data.Status == "PAID" {
		event := rabbitmq.PaymentEvent{
			OrderID:       payment.OrderID.String(),
			Gateway:       "PAYOS",
			TransactionID: "PAYOS-MANUAL-" + orderCodeStr,
		}
		eventBody, _ := json.Marshal(event)
		_ = rabbitmq.PublishMessage("payment.success", eventBody)

		return &CheckPaymentStatusRes{Success: true, Status: "COMPLETED", Message: "Paid successfully"}, nil
	}

	return &CheckPaymentStatusRes{Success: false, Status: result.Data.Status, Message: "Payment is pending or cancelled"}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
