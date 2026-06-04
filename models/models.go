package models

import (
	"database/sql"
	"fmt"
)

// =======================
// USER MODELS
// =======================

type User struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Faculty  string `json:"faculty"`
	GradYear string `json:"gradYear"`
	Bio      string `json:"bio"`
}

type Role string
type Faculty string

const (
	Alumni  Role = "Alumni"
	Student Role = "Student"
	Staff   Role = "Staff"
)
