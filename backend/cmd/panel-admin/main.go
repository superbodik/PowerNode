package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/config"
	"github.com/yourorg/panel/internal/db"
)

func main() {
	email := flag.String("email", "", "admin email (required)")
	username := flag.String("username", "", "admin username (required)")
	flag.Parse()

	if *email == "" || *username == "" {
		fmt.Fprintln(os.Stderr, "usage: panel-admin -email admin@example.com -username admin")
		os.Exit(2)
	}

	password := readPassword()
	if len(password) < 8 {
		log.Fatal("password must be at least 8 characters")
	}

	cfg := config.Load()
	ctx := context.Background()
	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	var id int64
	err = pool.QueryRow(ctx, `
		INSERT INTO users (email, username, password_hash, is_admin, is_active)
		VALUES ($1, $2, $3, true, true)
		ON CONFLICT (email) DO UPDATE
			SET password_hash = EXCLUDED.password_hash, is_admin = true, is_active = true
		RETURNING id`, *email, *username, hash,
	).Scan(&id)
	if err != nil {
		log.Fatalf("create admin: %v", err)
	}

	fmt.Printf("Admin user ready: id=%d email=%s\n", id, *email)
}

func readPassword() string {
	fmt.Fprint(os.Stderr, "Password: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		log.Fatalf("read password: %v", err)
	}
	return strings.TrimSpace(line)
}
