package handlers

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

func Health(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "AlumnConnect",
	})
}

func Login(c *echo.Context) error {
	return c.Render(200, "auth.html", map[string]interface{}{
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

func SignUp(c *echo.Context) error {
	return c.Render(200, "auth.html", map[string]any{
		"Mode": "signup",
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
