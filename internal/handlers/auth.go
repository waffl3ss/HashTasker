package handlers

import (
	"net/http"
	
	"hashtasker-go/internal/auth"
	"hashtasker-go/internal/models"
	
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *auth.AuthService
}

func NewAuthHandler(authService *auth.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) LoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{
		"title": "Login - HashTasker",
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")
	
	if username == "" || password == "" {
		c.HTML(http.StatusBadRequest, "login.html", gin.H{
			"title": "Login - HashTasker",
			"error": "Username and password are required",
		})
		return
	}
	
	user, err := h.authService.Authenticate(username, password)
	if err != nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"title": "Login - HashTasker",
			"error": "Invalid credentials",
		})
		return
	}
	
	session, err := h.authService.CreateSession(user.ID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"title": "Login - HashTasker",
			"error": "Failed to create session",
		})
		return
	}
	
	c.SetCookie("session_id", session.ID, 86400, "/", "", false, true)
	c.Redirect(http.StatusFound, "/")
}

func (h *AuthHandler) Logout(c *gin.Context) {
	sessionID, err := c.Cookie("session_id")
	if err == nil {
		h.authService.DeleteSession(sessionID)
	}
	
	c.SetCookie("session_id", "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/login")
}

func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}
	
	u := user.(*models.User)
	c.JSON(http.StatusOK, gin.H{
		"id":       u.ID,
		"username": u.Username,
		"email":    u.Email,
		"is_admin": u.IsAdmin,
	})
}