package handlers

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/Yassinproweb/alumnconn/db"
	"github.com/Yassinproweb/alumnconn/models"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

func Health(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "AlumnConnect",
	})
}

func LoginForm(c *echo.Context) error {
	return c.Render(200, "auth.html", map[string]any{
		"Mode":  "login",
		"Email": "",
		"Faculties": []string{
			"Science",
			"Engineering",
			"Business",
			"Education",
			"Law",
			"Medicine",
			"Arts",
		},
		"Role": []string{
			"Alumni",
			"Student",
			"Staff",
		},
	})
}

func RegisterForm(c *echo.Context) error {
	return c.Render(200, "auth.html", map[string]any{
		"Mode": "register",
		"Faculties": []string{
			"Science",
			"Engineering",
			"Business",
			"Education",
			"Law",
			"Medicine",
			"Arts",
		},
		"Role": []string{
			"Alumni",
			"Student",
			"Staff",
		},
	})
}

func RegisterUser(c *echo.Context) error {
	req := new(models.UserRequest)
	if err := c.Bind(req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid User Request")
	}

	req.Name = c.FormValue("username")
	req.Email = c.FormValue("email")
	req.Password = c.FormValue("password")
	req.Role = c.FormValue("role")
	req.Faculty = c.FormValue("faculty")
	req.EntryYear = c.FormValue("entryYear")
	req.Bio = c.FormValue("bio")

	fmt.Println(req.Name, req.Email, req.Password, req.Role, req.Faculty, req.EntryYear, req.Bio)
	if req.Name == "" || req.Email == "" || req.Password == "" || req.Role == "" || req.Faculty == "" || req.EntryYear == "" {
		fmt.Println(req.Name, req.Email, req.Password, req.Role, req.Faculty, req.EntryYear, req.Bio)
		return c.String(http.StatusBadRequest, "Missing a vital form field")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to hash password")
	}

	stmt, err := db.DB.Prepare("INSERT INTO users (username, email, password, role, faculty, entry_year, bio) VALUES (?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return c.String(http.StatusInternalServerError, "Database error")
	}
	defer stmt.Close()

	_, err = stmt.Exec(req.Name, req.Email, string(hashedPassword), req.Role, req.Faculty, req.EntryYear, req.Bio)
	if err != nil {
		return c.String(http.StatusConflict, "Email already taken")
	}

	return c.String(http.StatusCreated, "User registered successfully!")
}

func LoginUser(c *echo.Context) error {
	req := new(models.UserRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}

	// Retrieve the stored hash from SQLite for this user
	var storedHash string
	err := db.DB.QueryRow("SELECT password FROM users WHERE email = ?", req.Email).Scan(&storedHash)
	if err != nil {
		if err == sql.ErrNoRows {
			// Generic error message to prevent username enumeration attacks
			return c.String(http.StatusUnauthorized, "Invalid email or password")
		}
		return c.String(http.StatusInternalServerError, "Database error")
	}

	err = bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password))
	if err != nil {
		return c.String(http.StatusUnauthorized, "Invalid email or password")
	}

	return c.String(http.StatusOK, "Login successful!")
}

func GetPosts(c *echo.Context) error {
	return c.JSON(http.StatusOK, []string{})
}

func CreatePost(c *echo.Context) error {
	return c.JSON(http.StatusCreated, map[string]bool{"ok": true})
}

func LikePost(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"liked": true})
}

func GetUsers(c *echo.Context) error {
	return c.JSON(http.StatusOK, []string{})
}
