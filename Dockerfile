# Build stage
FROM golang:1.23.1-alpine AS builder

# Thiết lập thư mục làm việc
WORKDIR /app

# Khai báo biến môi trường cho quá trình build (nếu cần)
ENV CGO_ENABLED=0 GOOS=linux

# Cài đặt các gói phụ thuộc để giảm kích thước image cache
COPY go.mod go.sum ./
RUN go mod download

# Copy toàn bộ mã nguồn
COPY . .

# Build ứng dụng
RUN go build -o main ./cmd/server

# Run stage (Sử dụng Alpine siêu nhẹ)
FROM alpine:latest

WORKDIR /app

# Cài đặt timezone, ca-certificates (Cần cho SSL/HTTPS call gọi API ngoại)
RUN apk --no-cache add ca-certificates tzdata

# Copy file thực thi từ builder stage
COPY --from=builder /app/main .

# Copy file config/template nếu có phục vụ email hoặc render
# Khuyến nghị bỏ .env từ repo và dùng Docker ENV/Cloud ENV thay thế:
# COPY .env .env 

EXPOSE 8080

# Chạy ứng dụng
CMD ["./main"]
