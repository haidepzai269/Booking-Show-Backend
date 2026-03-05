# Build stage
FROM golang:1.24-alpine AS builder

# Set timezone and install CA certs upfront
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Khai báo biến môi trường cho quá trình build (nếu cần)
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Chỉ copy file gomod và down trước để tiết kiệm cache layer (tiết kiệm Ram khi build trên server)
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy toàn bộ mã nguồn
COPY . .

# Build ứng dụng với optimize flags để giảm thiểu resources
RUN go build -ldflags="-w -s" -o main ./cmd/server

# Dọn dẹp module cache để nhẹ image build (Tuy không đem sang stage-2 nhưng giúp giảm OOM kill)
RUN go clean -modcache

# Run stage (Sử dụng scratch siêu nhẹ thay vì alpine)
# Từ alpine chuyển xuống scratch sẽ an toàn hơn về memory, nhưng tuỳ code có cần /bin/sh không.
# Nếu API của bạn có dùng os.Exec cần alpine. Phía dưới mình vẫn dùng Alpine :latest quen thuộc cho an toàn
FROM alpine:latest

WORKDIR /app

# Copy các thư viện SSL/Timezone cần từ builder sang
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy file thực thi từ builder stage
COPY --from=builder /app/main .

EXPOSE 8080

# Chạy ứng dụng
CMD ["./main"]
