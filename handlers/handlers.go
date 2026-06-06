package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
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

	file, err := c.FormFile("avatar")
	if err != nil {
		return c.JSON(400, map[string]string{
			"error": "No file",
		})
	}

	src, _ := file.Open()
	defer src.Close()

	fileName := fmt.Sprintf(
		"%d_%s",
		time.Now().Unix(),
		file.Filename,
	)

	os.MkdirAll("static/uploads/avatars", os.ModePerm)

	path := filepath.Join(
		"static/uploads/avatars",
		fileName,
	)

	dst, _ := os.Create(path)
	defer dst.Close()

	io.Copy(dst, src)

	avatarURL := "/uploads/avatars/" + fileName

	_, err = db.DB.Exec(`
        UPDATE users
        SET avatar=?
        WHERE id=?
    `,
		avatarURL,
		uid,
	)

	if err != nil {
		return c.JSON(500, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(200, map[string]any{
		"ok":     true,
		"avatar": avatarURL,
	})
}

// ─── AI chat ──────────────────────────────────────────────────────────────────

const iuiuSystemPrompt = `You are AlumnConnect AI, the official assistant for the Islamic University in Uganda (IUIU) alumni network.

IUIU background:
- Founded in 1988 in Mbale, Uganda; the main campus is in Kampala (Kibuli). Other campuses: Female campus (Mbale), Arua campus, Kabale campus.
- Faculties: Science, Engineering, Business & Management, Education, Law, Medicine & Health Sciences, Arts & Social Sciences, Islamic Studies.
- Transcript requests: apply at the Registrar office on campus or via registrar@iuiu.ac.ug. Processing takes 5-10 working days.
- Alumni benefits: career fair invitations, the AlumnConnect networking platform, continuing-education workshops, mosque & library access on campus.
- Mentorship: alumni can register as mentors through AlumnConnect. Students can search the People tab by faculty to find mentors.
- Upcoming events are posted on the feed; users can also check iuiu.ac.ug/events.

Respond in a friendly, helpful, concise way. Use **bold** for key terms. Keep answers under 200 words unless a detailed explanation is necessary. Always encourage users to connect with each other on AlumnConnect.`

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiRequest struct {
	Messages []aiMessage `json:"messages"`
}

type anthropicRequest struct {
	Model     string      `json:"model"`
	MaxTokens int         `json:"max_tokens"`
	System    string      `json:"system"`
	Messages  []aiMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func ChatBot(c *echo.Context) error {
	var req aiRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil || len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	msgs := req.Messages
	if len(msgs) > 20 {
		msgs = msgs[len(msgs)-20:]
	}

	payload := anthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 512,
		System:    iuiuSystemPrompt,
		Messages:  msgs,
	}
	body, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", os.Getenv("ANTHROPIC_API_KEY"))
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "AI service unavailable"})
	}
	defer resp.Body.Close()

	var ar anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to parse AI response"})
	}
	if ar.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": ar.Error.Message})
	}

	var reply strings.Builder
	for _, block := range ar.Content {
		if block.Type == "text" {
			reply.WriteString(block.Text)
		}
	}

	return c.JSON(200, map[string]string{"reply": reply.String()})
}

// ─── conversations ────────────────────────────────────────────────────────────

// GetConversations returns the current user's conversation list with last message and unread count.
func GetConversations(c *echo.Context) error {
	uid := currentUserID(c)
	if uid == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
	}

	rows, err := db.DB.Query(`
		SELECT
			u.id,
			u.username,
			u.avatar_color,
			u.avatar,
			m.content AS last_message,
			m.created_at,
			(SELECT COUNT(*) FROM messages m2 WHERE m2.sender_id=u.id AND m2.receiver_id=? AND m2.is_read=0) AS unread
		FROM (
			SELECT
				CASE WHEN sender_id=? THEN receiver_id ELSE sender_id END AS other_id,
				MAX(id) AS last_id
			FROM messages
			WHERE sender_id=? OR receiver_id=?
			GROUP BY other_id
		) conv
		JOIN users u ON u.id=conv.other_id
		JOIN messages m ON m.id=conv.last_id
		ORDER BY m.created_at DESC
	`, uid, uid, uid, uid)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	defer rows.Close()

	var convos []map[string]any
	for rows.Next() {
		var (
			id                  int
			name                string
			avatarColor, avatar sql.NullString
			lastMsg             sql.NullString
			createdAt           string
			unread              int
		)
		if err := rows.Scan(&id, &name, &avatarColor, &avatar, &lastMsg, &createdAt, &unread); err != nil {
			continue
		}
		convos = append(convos, map[string]any{
			"id":           id,
			"name":         name,
			"avatar_color": avatarColor.String,
			"avatar":       avatar.String,
			"last_message": lastMsg.String,
			"created_at":   createdAt,
			"unread":       unread,
		})
	}
	if convos == nil {
		convos = []map[string]any{}
	}
	return c.JSON(200, convos)
}

// ─── messages ─────────────────────────────────────────────────────────────────

// GetMessages returns the full thread between the current user and :uid, marking incoming messages as read.
func GetMessages(c *echo.Context) error {
	me := currentUserID(c)
	if me == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
	}
	other, _ := strconv.Atoi(c.Param("uid"))

	db.DB.Exec(`UPDATE messages SET is_read=1 WHERE sender_id=? AND receiver_id=?`, other, me)

	rows, err := db.DB.Query(`
		SELECT id, sender_id, receiver_id, content, media_name, media_type, created_at
		FROM messages
		WHERE (sender_id=? AND receiver_id=?) OR (sender_id=? AND receiver_id=?)
		ORDER BY created_at ASC
	`, me, other, other, me)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	defer rows.Close()

	var msgs []map[string]any
	for rows.Next() {
		var (
			id, senderID, receiverID int
			content, createdAt       string
			mediaName, mediaType     sql.NullString
		)
		if err := rows.Scan(&id, &senderID, &receiverID, &content, &mediaName, &mediaType, &createdAt); err != nil {
			continue
		}
		msgs = append(msgs, map[string]any{
			"id":          id,
			"sender_id":   senderID,
			"receiver_id": receiverID,
			"content":     content,
			"media_name":  mediaName.String,
			"media_type":  mediaType.String,
			"created_at":  createdAt,
		})
	}
	if msgs == nil {
		msgs = []map[string]any{}
	}
	return c.JSON(200, msgs)
}

// SendMessage sends a message (with optional media) from the current user to :uid.
func SendMessage(c *echo.Context) error {
	me := currentUserID(c)
	if me == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
	}
	other, _ := strconv.Atoi(c.Param("uid"))

	content := c.FormValue("content")

	var mediaPath, mediaType string
	if file, err := c.FormFile("media"); err == nil {
		src, _ := file.Open()
		defer src.Close()

		fileName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
		os.MkdirAll("static/uploads/messages", os.ModePerm)
		dstPath := filepath.Join("static/uploads/messages", fileName)
		dst, _ := os.Create(dstPath)
		defer dst.Close()
		io.Copy(dst, src)

		mediaPath = "/uploads/messages/" + fileName
		mediaType = file.Header.Get("Content-Type")
	}

	if content == "" && mediaPath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "empty message"})
	}

	_, err := db.DB.Exec(`
		INSERT INTO messages (sender_id, receiver_id, content, media_name, media_type, is_read)
		VALUES (?, ?, ?, ?, ?, 0)
	`, me, other, content, mediaPath, mediaType)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}

	return c.JSON(201, map[string]bool{"ok": true})
}

// GetMessageMedia returns the media URL for a message — only accessible to the sender or receiver.
func GetMessageMedia(c *echo.Context) error {
	me := currentUserID(c)
	if me == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
	}
	msgID := c.Param("mid")

	var mediaName sql.NullString
	err := db.DB.QueryRow(`
		SELECT media_name FROM messages
		WHERE id=? AND (sender_id=? OR receiver_id=?)
	`, msgID, me, me).Scan(&mediaName)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "not found"})
	}

	return c.JSON(200, map[string]string{"media": mediaName.String})
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
		id                         int
		name, email, role, faculty string
		entryYear, bio, avatar     sql.NullString
	)

	err := db.DB.QueryRow(
		"SELECT id, username, email, role, faculty, entry_year, bio, avatar FROM users WHERE id = ?", uid,
	).Scan(&id, &name, &email, &role, &faculty, &entryYear, &bio, &avatar)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found"})
		}
		return c.String(http.StatusInternalServerError, "Database error")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"id":         id,
		"name":       name,
		"email":      email,
		"role":       role,
		"faculty":    faculty,
		"entry_year": entryYear.String,
		"bio":        bio.String,
		"avatar":     avatar.String,
	})
}

// POSTS
func GetPosts(c *echo.Context) error {
	uid := currentUserID(c)

	rows, err := db.DB.Query(`
		SELECT 
		p.id,
		p.user_id,
		p.content,
		p.created_at,
		p.media_name,
		p.media_type,
		u.username,
		u.role,
		u.faculty,
		u.avatar AS user_avatar,
		u.avatar_color,
		
		COUNT(DISTINCT l.id) as likes,
		
		CASE
		WHEN EXISTS(
		    SELECT 1
		    FROM likes lx
		    WHERE lx.user_id=? AND lx.post_id=p.id
		)
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
			id, userID                        int
			content, createdAt                string
			name, role, faculty, avatar       string
			avatarColor, mediaName, mediaType sql.NullString
		)

		var likes int
		var liked int

		err = rows.Scan(
			&id,
			&userID,
			&content,
			&createdAt,
			&mediaName,
			&mediaType,
			&name,
			&role,
			&faculty,
			&avatar,
			&avatarColor,
			&likes,
			&liked,
		)

		if err != nil {
			continue
		}

		posts = append(posts, map[string]any{
			"id":           id,
			"user_id":      userID,
			"name":         name,
			"content":      content,
			"created_at":   createdAt,
			"role":         role,
			"faculty":      faculty,
			"user_avatar":  avatar,
			"avatar_color": avatarColor.String,
			"likes":        likes,
			"liked":        liked == 1,

			"media_name": mediaName.String,
			"media_type": mediaType.String,
		})
	}

	return c.JSON(200, posts)
}

func CreatePost(c *echo.Context) error {
	uid := currentUserID(c)

	if uid == 0 {
		return c.JSON(http.StatusUnauthorized,
			map[string]string{
				"error": "login required",
			})
	}

	content := c.FormValue("content")

	mediaFile, err := c.FormFile("media")

	var mediaPath string
	var mediaType string

	if err == nil {
		src, _ := mediaFile.Open()
		defer src.Close()

		fileName := fmt.Sprintf(
			"%d_%s",
			time.Now().Unix(),
			mediaFile.Filename,
		)

		os.MkdirAll("static/uploads/posts", os.ModePerm)

		dstPath := filepath.Join(
			"static/uploads/posts",
			fileName,
		)

		dst, _ := os.Create(dstPath)
		defer dst.Close()

		io.Copy(dst, src)

		mediaPath = "/uploads/posts/" + fileName
		mediaType = mediaFile.Header.Get("Content-Type")
	}

	_, err = db.DB.Exec(`
		INSERT INTO posts(
    	user_id,
	    content,
	    media_name,
	    media_type
		)
		VALUES(?,?,?,?)
	`,
		uid,
		content,
		mediaPath,
		mediaType,
	)

	if err != nil {
		return c.JSON(500, err.Error())
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
	avatar_color,
	avatar
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
			id                                  int
			name, role, faculty                 string
			entryYear, bio, avatarColor, avatar sql.NullString
		)

		rows.Scan(
			&id,
			&name,
			&role,
			&faculty,
			&entryYear,
			&bio,
			&avatarColor,
			&avatar,
		)

		users = append(users, map[string]any{
			"id":           id,
			"name":         name,
			"role":         role,
			"faculty":      faculty,
			"entry_year":   entryYear.String,
			"bio":          bio.String,
			"avatar_color": avatarColor.String,
			"avatar":       avatar.String,
		})
	}

	return c.JSON(200, users)
}
