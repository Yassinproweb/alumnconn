package main

import (
	"embed"
	"html/template"
	"io"
	"net/http"

	"github.com/Yassinproweb/alumnconn/db"
	"github.com/Yassinproweb/alumnconn/handlers"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

//go:embed views/*
var templateFS embed.FS

type TemplateRenderer struct {
	templates *template.Template
}

func (t *TemplateRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	if viewContext, isMap := data.(map[string]any); isMap {
		viewContext["reverse"] = c.RouteInfo().Reverse
	}
	return t.templates.ExecuteTemplate(w, name, data)
}

func main() {
	godotenv.Load()
	db.ConnectDB()
	e := echo.New()

	e.Static("/", "static")

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	tmpl := template.Must(template.ParseFS(
		templateFS,
		"views/*.html",          // index.html and other pages
		"views/partials/*.html", // partials
	))

	e.Renderer = &TemplateRenderer{
		templates: tmpl,
	}

	// ── Landing page ──
	e.GET("/", func(c *echo.Context) error {
		return c.Render(http.StatusOK, "index.html", nil)
	})

	// ── Health ──
	e.GET("/health", handlers.Health)

	// ── Auth pages ──
	e.GET("/login", handlers.RegisterForm)
	e.POST("/register", handlers.RegisterUser)
	e.POST("/login", handlers.LoginUser)
	e.GET("/logout", handlers.Logout)

	// ── Feed (requires session) ──
	e.GET("/feed", handlers.FeedPage)

	// ── API ──
	api := e.Group("/api")

	// Current user — called by feed.html on init
	api.GET("/me", handlers.GetMe)

	// Posts
	api.GET("/posts", handlers.GetPosts)
	api.POST("/posts", handlers.CreatePost)
	api.POST("/posts/:id/like", handlers.LikePost)
	api.POST("/me/avatar", handlers.UploadAvatar)
	api.POST("/chat", handlers.ChatBot)

	// Posts
	api.GET("/conversations", handlers.GetConversations)
	api.GET("/messages/:uid", handlers.GetMessages)
	api.POST("/messages/:uid", handlers.SendMessage)
	api.GET("/messages/:uid/media/:mid", handlers.GetMessageMedia)

	// Posts
	api.GET("/posts/:id/comments", handlers.GetComments)
	api.POST("/posts/:id/comments", handlers.AddComment)
	api.POST("/comments/:id/vote/:vote", handlers.VoteComment)

	// Users
	api.GET("/users", handlers.GetUsers)

	if err := e.Start("0.0.0.0:8000"); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
