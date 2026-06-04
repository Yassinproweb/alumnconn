package models

import (
	"net/http"

	"github.com/Yassinproweb/alumnconn/db"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

// =======================
// USER MODELS
// =======================

type UserRequest struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Role      string `json:"role"`
	Faculty   string `json:"faculty"`
	EntryYear string `json:"entryYear"`
	Bio       string `json:"bio"`
}

type Role string

const (
	Alumni  Role = "Alumni"
	Student Role = "Student"
	Staff   Role = "Staff"
)

func RegisterUser(c *echo.Context) error {
	req := new(UserRequest)
	if err := c.Bind(req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid User Request")
	}

	if req.Name == "" || req.Password == "" || req.Email == "" || req.Role == "" || req.Faculty == "" {
		return c.String(http.StatusBadRequest, "Missing a vital form field")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to hash password")
	}

	stmt, err := db.DB.Prepare("INSERT INTO users (username, password) VALUES (?, ?)")
	if err != nil {
		return c.String(http.StatusInternalServerError, "Database error")
	}
	defer stmt.Close()

	_, err = stmt.Exec(req.Name, string(hashedPassword), req.Email, req.Role, req.Faculty, req.EntryYear, req.Bio)
	if err != nil {
		return c.String(http.StatusConflict, "Username already taken")
	}

	return c.String(http.StatusCreated, "User registered successfully!")
}
