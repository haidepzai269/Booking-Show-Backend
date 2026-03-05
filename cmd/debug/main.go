package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	godotenv.Load("../.env")
	dbURL := os.Getenv("DB_URL")
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Fatal("DB connect failed:", err)
	}

	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	fmt.Println("=== LAST 5 ORDERS ===")
	rows, err := sqlDB.Query(`
		SELECT o.id, o.status, o.user_id, o.created_at,
			COALESCE((SELECT COUNT(*) FROM order_seats os WHERE os.order_id = o.id), 0) AS seat_count,
			COALESCE((SELECT COUNT(*) FROM tickets t WHERE t.order_id = o.id), 0) AS ticket_count,
			COALESCE((SELECT status FROM payments p WHERE p.order_id = o.id ORDER BY created_at DESC LIMIT 1), 'none') AS payment_status,
			COALESCE((SELECT gateway FROM payments p WHERE p.order_id = o.id ORDER BY created_at DESC LIMIT 1), 'none') AS gateway
		FROM orders o
		ORDER BY o.created_at DESC LIMIT 5
	`)
	if err != nil {
		log.Fatal("Query failed:", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, payStatus, gateway string
		var userID int
		var seatCount, ticketCount int
		var createdAt interface{}
		rows.Scan(&id, &status, &userID, &createdAt, &seatCount, &ticketCount, &payStatus, &gateway)
		fmt.Printf("Order: %s | Status: %-10s | User: %d | Seats: %d | Tickets: %d | Payment: %-8s(%s) | Created: %v\n",
			id[:8], status, userID, seatCount, ticketCount, payStatus, gateway, createdAt)
	}

	fmt.Println("\n=== ORDER_SEATS TABLE (check exists) ===")
	var tableCount int
	sqlDB.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'order_seats'`).Scan(&tableCount)
	fmt.Printf("order_seats table exists: %v (count: %d)\n", tableCount > 0, tableCount)

	fmt.Println("\n=== RECENT PAYMENT RECORDS ===")
	rows2, _ := sqlDB.Query(`
		SELECT id, order_id, gateway, gateway_transaction_id, status, created_at
		FROM payments ORDER BY created_at DESC LIMIT 5
	`)
	defer rows2.Close()
	for rows2.Next() {
		var id, orderID, gateway, txID, status string
		var createdAt interface{}
		rows2.Scan(&id, &orderID, &gateway, &txID, &status, &createdAt)
		fmt.Printf("Payment: %s | Order: %s | Gateway: %-8s | TxID: %-20s | Status: %s\n",
			id[:8], orderID[:8], gateway, txID, status)
	}

	fmt.Println("\n=== SEATS STATUS (last 10 showtime_seats with BOOKED) ===")
	rows3, _ := sqlDB.Query(`
		SELECT id, showtime_id, status, locked_by, updated_at
		FROM showtime_seats WHERE status IN ('BOOKED', 'LOCKED')
		ORDER BY updated_at DESC LIMIT 10
	`)
	defer rows3.Close()
	for rows3.Next() {
		var id, showtimeID int
		var status string
		var lockedBy interface{}
		var updatedAt interface{}
		rows3.Scan(&id, &showtimeID, &status, &lockedBy, &updatedAt)
		fmt.Printf("Seat: %d | Showtime: %d | Status: %-8s | LockedBy: %v | Updated: %v\n",
			id, showtimeID, status, lockedBy, updatedAt)
	}
}
