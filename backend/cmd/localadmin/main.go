package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	host := flag.String("host", "127.0.0.1", "PostgreSQL host")
	port := flag.Int("port", 5432, "PostgreSQL port")
	user := flag.String("user", "postgres", "PostgreSQL user")
	dbPassword := flag.String("db-password", "", "PostgreSQL password")
	dbName := flag.String("dbname", "sub2api", "PostgreSQL database")
	sslMode := flag.String("sslmode", "disable", "PostgreSQL sslmode")
	email := flag.String("email", "admin@sub2api.local", "admin email")
	password := flag.String("password", "", "admin password")
	flag.Parse()

	if *password == "" {
		log.Fatal("admin password is required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		*host,
		*port,
		*user,
		*dbPassword,
		*dbName,
		*sslMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := db.ExecContext(
		ctx,
		`UPDATE users
		 SET password_hash = $1, role = 'admin', status = 'active', updated_at = NOW()
		 WHERE email = $2 AND deleted_at IS NULL`,
		string(hash),
		*email,
	)
	if err != nil {
		log.Fatalf("update admin password: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		log.Fatalf("read rows affected: %v", err)
	}
	if rows > 0 {
		log.Printf("updated local admin password for %s", *email)
		return
	}

	_, err = db.ExecContext(
		ctx,
		`INSERT INTO users (email, password_hash, role, balance, concurrency, status, created_at, updated_at)
		 VALUES ($1, $2, 'admin', 0, 5, 'active', NOW(), NOW())`,
		*email,
		string(hash),
	)
	if err != nil {
		log.Fatalf("create local admin: %v", err)
	}

	log.Printf("created local admin %s", *email)
}
