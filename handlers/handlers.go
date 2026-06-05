package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

func UploadAvatar(c *echo.Context) error {
	uid := currentUserID(c)
	if uid == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "login required",
		})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "failed to get avatar",
		})
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}
	defer src.Close()

	ext := strings.ToLower(filepath.Ext(file.Filename))

	allowed := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".webp": true,
	}

	if !allowed[ext] {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "only jpg, png, webp allowed",
		})
	}

	// create upload directory if missing
	uploadDir := filepath.Join("static", "uploads")

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	// unique filename
	filename := fmt.Sprintf(
		"%d_user%d%s",
		time.Now().UnixNano(),
		uid,
		ext,
	)

	dstPath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(dstPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	avatarURL := "/uploads/" + filename

	// save avatar path into users table
	_, err = db.DB.Exec(`
		UPDATE users
		SET avatar=?
		WHERE id=?
	`, avatarURL, uid)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":     true,
		"avatar": avatarURL,
	})
}

func ChatBot(c *echo.Context) error {
	return c.JSON(200, map[string]string{
		"reply": "Assalam ‘Alaykum 👋 I am your IUIU assistant.",
	})
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

// POSTS
func GetPosts(c *echo.Context) error {
	uid := currentUserID(c)

	rows, err := db.DB.Query(`
		SELECT p.id, p.user_id, p.content, p.created_at, u.username, u.role, u.faculty, u.avatar_color,

		COUNT(DISTINCT l.id) as likes,

		CASE
		WHEN EXISTS(SELECT 1 FROM likes lx WHERE lx.user_id=? AND lx.post_id=p.id)
	  THEN 1
		ELSE 0
		END as liked

		FROM posts p
		JOIN users u
		ON p.user_id=u.id

		LEFT JOIN likes l
		ON l.post_id=p.id

		GROUP BY p.id

		ORDER BY p.created_at DESC
	`, uid)

	if err != nil {
		return c.JSON(500, map[string]string{
			"error": err.Error(),
		})
	}
	defer rows.Close()

	var posts []map[string]any

	for rows.Next() {
		var (
			id, userID                  int
			content, createdAt          string
			name, role, faculty, avatar string
		)

		var likes int
		var liked int

		rows.Scan(
			&id,
			&userID,
			&content,
			&createdAt,
			&name,
			&role,
			&faculty,
			&avatar,
			&likes,
			&liked,
		)

		posts = append(posts, map[string]any{
			"id":           id,
			"user_id":      userID,
			"name":         name,
			"content":      content,
			"created_at":   createdAt,
			"role":         role,
			"faculty":      faculty,
			"avatar_color": avatar,
			"likes":        likes,
			"liked":        liked == 1,
		})
	}

	return c.JSON(200, posts)
}

func CreatePost(c *echo.Context) error {
	uid := currentUserID(c)

	if uid == 0 {
		return c.JSON(401, map[string]string{
			"error": "login required",
		})
	}

	content := c.FormValue("content")

	if content == "" {
		return c.JSON(400, map[string]string{
			"error": "empty post",
		})
	}

	_, err := db.DB.Exec(
		`INSERT INTO posts(user_id,content)
		 VALUES(?,?)`,
		uid,
		content,
	)

	if err != nil {
		return c.JSON(500, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(201, map[string]bool{
		"ok": true,
	})
}

func LikePost(c *echo.Context) error {

	uid := currentUserID(c)

	if uid == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "login required",
		})
	}

	postID := c.Param("id")

	// check if user already liked this post
	var exists int

	err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM likes
		WHERE user_id=? AND post_id=?
	`,
		uid,
		postID,
	).Scan(&exists)

	if err != nil {
		return c.JSON(500, map[string]string{
			"error": err.Error(),
		})
	}

	liked := false

	if exists > 0 {

		// unlike
		_, err = db.DB.Exec(`
			DELETE FROM likes
			WHERE user_id=? AND post_id=?
		`,
			uid,
			postID,
		)

		if err != nil {
			return c.JSON(500, map[string]string{
				"error": err.Error(),
			})
		}

	} else {

		// like
		_, err = db.DB.Exec(`
			INSERT INTO likes(user_id,post_id)
			VALUES(?,?)
		`,
			uid,
			postID,
		)

		if err != nil {
			return c.JSON(500, map[string]string{
				"error": err.Error(),
			})
		}

		liked = true
	}

	// get current total likes
	var total int

	err = db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM likes
		WHERE post_id=?
	`, postID).Scan(&total)

	if err != nil {
		return c.JSON(500, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(200, map[string]any{
		"liked": liked,
		"likes": total,
	})
}

func GetUsers(c *echo.Context) error {
	search := c.QueryParam("q")

	query := `
	SELECT
	id,
	username,
	role,
	faculty,
	entry_year,
	bio,
	avatar_color
	FROM users
	`

	var rows *sql.Rows
	var err error

	if search != "" {

		query += `
		WHERE username LIKE ?
		OR faculty LIKE ?
		OR role LIKE ?
		`

		search = "%" + search + "%"

		rows, err = db.DB.Query(
			query,
			search,
			search,
			search,
		)

	} else {
		rows, err = db.DB.Query(query)
	}

	if err != nil {
		return c.JSON(500, err.Error())
	}

	defer rows.Close()

	var users []map[string]any

	for rows.Next() {

		var (
			id                     int
			name, role, faculty    string
			entryYear, bio, avatar sql.NullString
		)

		rows.Scan(
			&id,
			&name,
			&role,
			&faculty,
			&entryYear,
			&bio,
			&avatar,
		)

		users = append(users, map[string]any{
			"id":           id,
			"name":         name,
			"role":         role,
			"faculty":      faculty,
			"entry_year":   entryYear.String,
			"bio":          bio.String,
			"avatar_color": avatar.String,
		})
	}

	return c.JSON(200, users)
}
