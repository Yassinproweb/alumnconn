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
const iuiuSystemPrompt = `You are AlumnConnect AI — a precise, straight-to-the-point assistant for the Islamic University in Uganda (IUIU). 

CRITICAL RULE: Keep all answers extremely brief and straight to the point (under 2-3 sentences maximum). Avoid any conversational filler, long summaries, or unnecessary fluff.

=== ABOUT IUIU ===
- Founded: 1988 in Mbale, Uganda by OIC.
- Campuses: Main (Kibuli Hill, Kampala), Female (Mbale), Arua, Kabale.
- Website: iuiu.ac.ug | Email: info@iuiu.ac.ug

=== TRANSCRIPTS ===
- Apply at the Registrar's office on your campus or email registrar@iuiu.ac.ug. Requires Student ID, application form, and payment receipt. Takes 5-10 working days.

=== MENTORSHIP & BENEFITS ===
- Use the 'People' tab to filter, find, and message mentors/students. 
- Benefits include networking, career fairs, library access, and job boards.

=== TONE & FORMAT ===
- If the user greets with "Assalamu Alaikum", strictly open your reply with "Wa Alaykum Assalam".
- Answer the user's question instantly without introductory phrases.`

// aiMessage matches the shape the frontend sends: { role, content }
type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiRequest struct {
	Messages []aiMessage `json:"messages"`
}

// ── Gemini REST API types ─────────────────────────────────────────────────────

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	SystemInstruction geminiSystemInstruction `json:"system_instruction"`
	Contents          []geminiContent         `json:"contents"`
	GenerationConfig  map[string]any          `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func ChatBot(c *echo.Context) error {
	var req aiRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}

	if len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "No messages found"})
	}

	// Trim history to prevent blowing past token limits (Last 20 turns)
	msgs := req.Messages
	if len(msgs) > 20 {
		msgs = msgs[len(msgs)-20:]
	}

	var contents []geminiContent
	for _, m := range msgs {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		if role != "user" && role != "model" {
			continue
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}

	if len(contents) == 0 || contents[0].Role != "user" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Conversation must start with a user message"})
	}

	payload := geminiRequest{
		SystemInstruction: geminiSystemInstruction{
			Parts: []geminiPart{{Text: iuiuSystemPrompt}},
		},
		Contents: contents,
		GenerationConfig: map[string]any{
			"maxOutputTokens": 1050, // Strictly limits response length window
			"temperature":     0.3,  // Forces highly direct, objective word choices
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to encode body"})
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "AI API Key environment target missing"})
	}

	// FIX: Updated endpoint targeting the gemini-3.5-flash model
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash:generateContent?key=" + apiKey

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create request"})
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "AI service unreachable"})
	}
	defer resp.Body.Close()

	var gr geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to decode response"})
	}

	if gr.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": gr.Error.Message})
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "No response returned from the model"})
	}

	var reply strings.Builder
	for _, part := range gr.Candidates[0].Content.Parts {
		reply.WriteString(part.Text)
	}

	return c.JSON(http.StatusOK, map[string]string{"reply": reply.String()})
}

// ─── conversations ────────────────────────────────────────────────────────────
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
		END as liked,
		
		COUNT(DISTINCT cm.id) as comment_count
		
		FROM posts p
		JOIN users u
		ON p.user_id=u.id
		
		LEFT JOIN likes l
		ON l.post_id=p.id
		
		LEFT JOIN comments cm
		ON cm.post_id=p.id
		
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
		var commentCount int

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
			&commentCount,
		)

		if err != nil {
			continue
		}

		posts = append(posts, map[string]any{
			"id":            id,
			"user_id":       userID,
			"name":          name,
			"content":       content,
			"created_at":    createdAt,
			"role":          role,
			"faculty":       faculty,
			"user_avatar":   avatar,
			"avatar_color":  avatarColor.String,
			"likes":         likes,
			"liked":         liked == 1,
			"comment_count": commentCount,

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

// ─── comments ─────────────────────────────────────────────────────────────────

// GetComments returns all top-level comments for a post, each with:
//   - author info (name, avatar, role)
//   - thumbs-up and thumbs-down counts
//   - whether the current user has voted (and which direction)
//   - replies (one level deep), each with the same vote fields
func GetComments(c *echo.Context) error {
	uid := currentUserID(c)
	postID := c.Param("id")

	// ── top-level comments ────────────────────────────────────────
	rows, err := db.DB.Query(`
		SELECT
			cm.id,
			cm.user_id,
			cm.content,
			cm.created_at,
			u.username,
			u.role,
			u.avatar,
			u.avatar_color,
			COALESCE(SUM(CASE WHEN cv.vote=1  THEN 1 ELSE 0 END), 0) AS thumbs_up,
			COALESCE(SUM(CASE WHEN cv.vote=-1 THEN 1 ELSE 0 END), 0) AS thumbs_down,
			COALESCE((SELECT vote FROM comment_votes WHERE user_id=? AND comment_id=cm.id), 0) AS my_vote
		FROM comments cm
		JOIN users u ON u.id = cm.user_id
		LEFT JOIN comment_votes cv ON cv.comment_id = cm.id
		WHERE cm.post_id=? AND cm.parent_id IS NULL
		GROUP BY cm.id
		ORDER BY cm.created_at ASC
	`, uid, postID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	defer rows.Close()

	// Build a map so we can attach replies efficiently
	type comment struct {
		ID          int       `json:"id"`
		UserID      int       `json:"user_id"`
		Content     string    `json:"content"`
		CreatedAt   string    `json:"created_at"`
		Name        string    `json:"name"`
		Role        string    `json:"role"`
		Avatar      string    `json:"avatar"`
		AvatarColor string    `json:"avatar_color"`
		ThumbsUp    int       `json:"thumbs_up"`
		ThumbsDown  int       `json:"thumbs_down"`
		MyVote      int       `json:"my_vote"` // 1=up, -1=down, 0=none
		Replies     []comment `json:"replies"`
	}

	var topLevel []comment
	idIndex := map[int]int{} // comment id → index in topLevel

	for rows.Next() {
		var cm comment
		var avatar, avatarColor sql.NullString
		if err := rows.Scan(
			&cm.ID, &cm.UserID, &cm.Content, &cm.CreatedAt,
			&cm.Name, &cm.Role, &avatar, &avatarColor,
			&cm.ThumbsUp, &cm.ThumbsDown, &cm.MyVote,
		); err != nil {
			continue
		}
		cm.Avatar = avatar.String
		cm.AvatarColor = avatarColor.String
		cm.Replies = []comment{}
		idIndex[cm.ID] = len(topLevel)
		topLevel = append(topLevel, cm)
	}

	// ── replies (one level deep) ──────────────────────────────────
	repRows, err := db.DB.Query(`
		SELECT
			cm.id,
			cm.parent_id,
			cm.user_id,
			cm.content,
			cm.created_at,
			u.username,
			u.role,
			u.avatar,
			u.avatar_color,
			COALESCE(SUM(CASE WHEN cv.vote=1  THEN 1 ELSE 0 END), 0) AS thumbs_up,
			COALESCE(SUM(CASE WHEN cv.vote=-1 THEN 1 ELSE 0 END), 0) AS thumbs_down,
			COALESCE((SELECT vote FROM comment_votes WHERE user_id=? AND comment_id=cm.id), 0) AS my_vote
		FROM comments cm
		JOIN users u ON u.id = cm.user_id
		LEFT JOIN comment_votes cv ON cv.comment_id = cm.id
		WHERE cm.post_id=? AND cm.parent_id IS NOT NULL
		GROUP BY cm.id
		ORDER BY cm.created_at ASC
	`, uid, postID)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	defer repRows.Close()

	for repRows.Next() {
		var cm comment
		var parentID int
		var avatar, avatarColor sql.NullString
		if err := repRows.Scan(
			&cm.ID, &parentID, &cm.UserID, &cm.Content, &cm.CreatedAt,
			&cm.Name, &cm.Role, &avatar, &avatarColor,
			&cm.ThumbsUp, &cm.ThumbsDown, &cm.MyVote,
		); err != nil {
			continue
		}
		cm.Avatar = avatar.String
		cm.AvatarColor = avatarColor.String
		cm.Replies = []comment{}
		if idx, ok := idIndex[parentID]; ok {
			topLevel[idx].Replies = append(topLevel[idx].Replies, cm)
		}
	}

	if topLevel == nil {
		topLevel = []comment{}
	}
	return c.JSON(200, topLevel)
}

// AddComment creates a new comment on a post.
// Body: { "content": "...", "parent_id": 0 }  (parent_id=0 → top-level)
func AddComment(c *echo.Context) error {
	uid := currentUserID(c)
	if uid == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
	}
	postID := c.Param("id")

	var body struct {
		Content  string `json:"content"`
		ParentID *int   `json:"parent_id"` // nil or omitted → top-level
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}
	if strings.TrimSpace(body.Content) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "comment cannot be empty"})
	}

	var result sql.Result
	var err error
	if body.ParentID != nil && *body.ParentID > 0 {
		result, err = db.DB.Exec(`
			INSERT INTO comments (post_id, user_id, parent_id, content)
			VALUES (?, ?, ?, ?)
		`, postID, uid, *body.ParentID, strings.TrimSpace(body.Content))
	} else {
		result, err = db.DB.Exec(`
			INSERT INTO comments (post_id, user_id, content)
			VALUES (?, ?, ?)
		`, postID, uid, strings.TrimSpace(body.Content))
	}
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}

	newID, _ := result.LastInsertId()
	return c.JSON(201, map[string]any{"ok": true, "id": newID})
}

// VoteComment handles thumbs-up (vote=1) and thumbs-down (vote=-1) on a comment.
// Calling the same vote twice toggles it off (removes the vote).
// Route param :vote must be "up" or "down".
func VoteComment(c *echo.Context) error {
	uid := currentUserID(c)
	if uid == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
	}
	commentID := c.Param("id")
	direction := c.Param("vote") // "up" or "down"

	voteVal := 0
	switch direction {
	case "up":
		voteVal = 1
	case "down":
		voteVal = -1
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "vote must be 'up' or 'down'"})
	}

	// Check existing vote
	var existing int
	err := db.DB.QueryRow(`
		SELECT COALESCE(vote, 0) FROM comment_votes
		WHERE user_id=? AND comment_id=?
	`, uid, commentID).Scan(&existing)

	if err != nil && err != sql.ErrNoRows {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}

	if existing == voteVal {
		// Same vote again → remove it (toggle off)
		db.DB.Exec(`DELETE FROM comment_votes WHERE user_id=? AND comment_id=?`, uid, commentID)
		voteVal = 0
	} else if existing != 0 {
		// Switching from up→down or down→up
		db.DB.Exec(`UPDATE comment_votes SET vote=? WHERE user_id=? AND comment_id=?`, voteVal, uid, commentID)
	} else {
		// New vote
		db.DB.Exec(`INSERT INTO comment_votes (user_id, comment_id, vote) VALUES (?,?,?)`, uid, commentID, voteVal)
	}

	// Return updated counts
	var up, down int
	db.DB.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN vote=1  THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN vote=-1 THEN 1 ELSE 0 END), 0)
		FROM comment_votes WHERE comment_id=?`, commentID).Scan(&up, &down)

	return c.JSON(200, map[string]any{
		"my_vote":     voteVal,
		"thumbs_up":   up,
		"thumbs_down": down,
	})
}
