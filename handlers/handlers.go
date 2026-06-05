package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Yassinproweb/alumnconn/db"
	"github.com/Yassinproweb/alumnconn/models"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
)

// ─── helpers ──────────────────────────────────────────────────────────────────
const sessionCookie = "ac_session"

// setSession stores the user ID in a plain cookie (replace with JWT / signed
// cookie in production).
func setSession(c *echo.Context, userID int) {
	c.SetCookie(&http.Cookie{
		Name:     sessionCookie,
		Value:    strconv.Itoa(userID),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 30, // 30 days
	})
}

// currentUserID returns the logged-in user ID from the session cookie, or 0.
func currentUserID(c *echo.Context) int {
	cookie, err := c.Request().Cookie(sessionCookie)
	if err != nil {
		return 0
	}
	id, _ := strconv.Atoi(cookie.Value)
	return id
}

// ─── health ───
func Health(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "AlumnConnect",
	})
}

var faculties = []string{
	"Science", "Engineering", "Business",
	"Education", "Law", "Medicine", "Arts",
}

func LoginForm(c *echo.Context) error {
	return c.Render(200, "auth.html", map[string]any{
		"Mode":      "login",
		"Email":     "",
		"Faculties": faculties,
	})
}

func RegisterForm(c *echo.Context) error {
	return c.Render(200, "auth.html", map[string]any{
		"Mode":      "register",
		"Faculties": faculties,
	})
}

// FeedPage serves the feed — redirect to login if not authenticated.
func FeedPage(c *echo.Context) error {
	if currentUserID(c) == 0 {
		return c.Redirect(http.StatusFound, "/login")
	}
	return c.Render(200, "feed.html", nil)
}

// ─── auth actions ─────────────────────────────────────────────────────────────

func RegisterUser(c *echo.Context) error {
	req := new(models.UserRequest)
	if err := c.Bind(req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid User Request")
	}

	// Belt-and-suspenders: also read form values directly
	req.Name = c.FormValue("username")
	req.Email = c.FormValue("email")
	req.Password = c.FormValue("password")
	req.Role = c.FormValue("role")
	req.Faculty = c.FormValue("faculty")
	req.EntryYear = c.FormValue("entryYear")
	req.Bio = c.FormValue("bio")

	fmt.Println("[register]", req.Name, req.Email, req.Role, req.Faculty, req.EntryYear)

	if req.Name == "" || req.Email == "" || req.Password == "" || req.Role == "" || req.Faculty == "" || req.EntryYear == "" {
		return c.String(http.StatusBadRequest, "Missing a required field")
	}

	if len(req.Password) < 6 {
		return c.String(http.StatusBadRequest, "Password must be at least 6 characters")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to hash password")
	}

	stmt, err := db.DB.Prepare(
		"INSERT INTO users (username, email, password, role, faculty, entry_year, bio) VALUES (?, ?, ?, ?, ?, ?, ?)",
	)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Database error")
	}
	defer stmt.Close()

	result, err := stmt.Exec(req.Name, req.Email, string(hashedPassword), req.Role, req.Faculty, req.EntryYear, req.Bio)
	if err != nil {
		return c.String(http.StatusConflict, "Email already taken")
	}

	newID, _ := result.LastInsertId()
	setSession(c, int(newID))

	// 201 signals success to the HTMX handler in auth.html which redirects to /feed
	return c.String(http.StatusCreated, "User registered successfully!")
}

func LoginUser(c *echo.Context) error {
	req := new(models.UserRequest)
	if err := c.Bind(req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid request payload")
	}

	var (
		storedHash string
		userID     int
	)
	err := db.DB.QueryRow(
		"SELECT id, password FROM users WHERE email = ?", req.Email,
	).Scan(&userID, &storedHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.String(http.StatusUnauthorized, "Invalid email or password")
		}
		return c.String(http.StatusInternalServerError, "Database error")
	}

	if err = bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password)); err != nil {
		return c.String(http.StatusUnauthorized, "Invalid email or password")
	}

	setSession(c, userID)
	return c.String(http.StatusOK, "Login successful!")
}

func Logout(c *echo.Context) error {
	c.SetCookie(&http.Cookie{
		Name:   sessionCookie,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	return c.Redirect(http.StatusFound, "/login")
}

// ─── /api/me ──────────────────────────────────────────────────────────────────

// GetMe returns the currently logged-in user's profile.
// The feed.html JS calls this on load to hydrate the UI with real user data.
func GetMe(c *echo.Context) error {
	uid := currentUserID(c)
	if uid == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
	}

	var (
		id                          int
		name, email, role, faculty  string
		entryYear, bio, avatarColor sql.NullString
	)
	err := db.DB.QueryRow(
		"SELECT id, username, email, role, faculty, entry_year, bio, avatar_color FROM users WHERE id = ?", uid,
	).Scan(&id, &name, &email, &role, &faculty, &entryYear, &bio, &avatarColor)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found"})
		}
		return c.String(http.StatusInternalServerError, "Database error")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"id":           id,
		"name":         name,
		"email":        email,
		"role":         role,
		"faculty":      faculty,
		"entry_year":   entryYear.String,
		"bio":          bio.String,
		"avatar_color": avatarColor.String,
	})
}

// ─── posts ────────────────────────────────────────────────────────────────────

func GetPosts(c *echo.Context) error {
	return c.JSON(http.StatusOK, []string{})
}

func CreatePost(c *echo.Context) error {
	return c.JSON(http.StatusCreated, map[string]bool{"ok": true})
}

func LikePost(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"liked": true})
}

// ─── users ────────────────────────────────────────────────────────────────────

func GetUsers(c *echo.Context) error {
	return c.JSON(http.StatusOK, []string{})
}
