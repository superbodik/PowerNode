package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           int64      `json:"id"`
	UUID         uuid.UUID  `json:"uuid"`
	Email        string     `json:"email"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	IsAdmin      bool       `json:"is_admin"`
	TOTPEnabled  bool       `json:"totp_enabled"`
	IsActive     bool       `json:"is_active"`
	ServerLimit  *int       `json:"server_limit"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Role struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Permission struct {
	ID          int    `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description"`
}
