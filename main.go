package main

import (
	"embed"
	"html/template"
	"io"
	"net/http"

	"github.com/Yassinproweb/alumnconn/db"
	"github.com/Yassinproweb/alumnconn/handlers"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

//go:embed views/*
var templateFS embed.FS

// TemplateRenderer is a custom html/template renderer for Echo framework
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
	db.ConnectDB()
	e := echo.New()

	e.Static("/", "static")

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	// e.Use(middleware.CORS())

	tmpl := template.Must(template.ParseFS(
		templateFS,
		"views/*.html",          // index.html and oyher pages
		"views/partials/*.html", // partials
	))

	e.Renderer = &TemplateRenderer{
		templates: tmpl,
	}

	e.GET("/", func(c *echo.Context) error {
		return c.Render(http.StatusOK, "index.html", nil)
	})

	e.GET("/health", handlers.Health)
	e.GET("/login", handlers.LoginForm)
	e.GET("/register", handlers.RegisterForm)

	e.POST("/register", handlers.RegisterUser)
	e.POST("/login", handlers.LoginUser)

	api := e.Group("/api")
	api.GET("/posts", handlers.GetPosts)
	api.POST("/posts", handlers.CreatePost)
	api.POST("/posts/:id/like", handlers.LikePost)

	api.GET("/users", handlers.GetUsers)

	if err := e.Start("0.0.0.0:7000"); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
