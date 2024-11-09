// admin.go
package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var dbPool *pgxpool.Pool

func main() {
	// Connect to database...
	var err error
	dbURL := "postgres://app:app@localhost:5432/sampledb" // replace with your credentials
	dbPool, err = pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer dbPool.Close()
	// Create a new user
	err = createUser("colnio", "132134")
	if err != nil {
		log.Fatal(err)
	}

	// Approve the user
	err = approveUser("colnio")
	if err != nil {
		log.Fatal(err)
	}
}

func createUser(username, password string) error {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Insert user
	_, err = dbPool.Exec(context.Background(),
		"INSERT INTO users (username, password_hash, is_approved) VALUES ($1, $2, false)",
		username, string(hashedPassword))
	return err
}

// Helper function to approve a user (you'll need to run this manually or create an admin interface)
func approveUser(username string) error {
	_, err := dbPool.Exec(context.Background(),
		"UPDATE users SET is_approved = true WHERE username = $1",
		username)
	return err
}
