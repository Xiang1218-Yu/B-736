package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"prompt736/internal/models"
	"prompt736/internal/utils"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// LoginPage 登录页面
func (h *Handler) LoginPage(c *gin.Context) {
	if _, exists := c.Get("user"); exists {
		c.Redirect(http.StatusFound, "/")
		return
	}

	data := h.basePageData(c)
	data.Title = h.tr(c, "auth.login_title")
	h.renderPage(c, "login.html", data)
}

// LoginHandler 登录处理
func (h *Handler) LoginHandler(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")

	user, err := h.App.GetUserByUsername(username)
	if err != nil {
		h.setFlashKey(c, "flash_error", "flash.invalid_credentials")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	if user.Status != "active" {
		h.setFlashKey(c, "flash_error", "flash.account_disabled")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	if !utils.CheckPassword(user.PasswordHash, password) {
		h.setFlashKey(c, "flash_error", "flash.invalid_credentials")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	session := sessions.Default(c)
	session.Set("userID", user.ID)
	session.Save()

	h.setFlashKey(c, "flash_success", "flash.login_success")
	c.Redirect(http.StatusFound, "/")
}

// RegisterPage 注册页面
func (h *Handler) RegisterPage(c *gin.Context) {
	if _, exists := c.Get("user"); exists {
		c.Redirect(http.StatusFound, "/")
		return
	}

	data := h.basePageData(c)
	data.Title = h.tr(c, "auth.register_title")
	h.renderPage(c, "register.html", data)
}

// RegisterHandler 注册处理
func (h *Handler) RegisterHandler(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")
	confirmPassword := c.PostForm("confirm_password")

	if len(username) < 3 || len(username) > 30 {
		h.setFlashKey(c, "flash_error", "flash.username_length")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	if len(password) < 6 {
		h.setFlashKey(c, "flash_error", "flash.password_length")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	if password != confirmPassword {
		h.setFlashKey(c, "flash_error", "flash.password_mismatch")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	_, err := h.App.GetUserByUsername(username)
	if err == nil {
		h.setFlashKey(c, "flash_error", "flash.username_exists")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	hash, err := utils.HashPassword(password)
	if err != nil {
		h.setFlashKey(c, "flash_error", "flash.register_failed")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	user, err := h.App.CreateUser(username, hash)
	if err != nil {
		h.setFlashKey(c, "flash_error", "flash.register_failed")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	session := sessions.Default(c)
	session.Set("userID", user.ID)
	session.Save()

	h.App.ClearCache()
	h.setFlashKey(c, "flash_success", "flash.register_success")
	c.Redirect(http.StatusFound, "/")
}

// LogoutHandler 登出处理
func (h *Handler) LogoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/")
}

// ProfilePage 个人中心页面
func (h *Handler) ProfilePage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	canCheckin := true
	if user.LastCheckinAt != nil {
		last := user.LastCheckinAt.Format("2006-01-02")
		today := time.Now().Format("2006-01-02")
		if last == today {
			canCheckin = false
		}
	}

	data := h.basePageData(c)
	data.Title = h.tr(c, "profile.title")
	data.CanCheckin = canCheckin

	h.renderPage(c, "profile.html", data)
}

// CheckinHandler 签到处理
func (h *Handler) CheckinHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	err := h.App.Checkin(user.ID)
	if err != nil {
		if errors.Is(err, utils.ErrAlreadyCheckedIn) {
			h.setFlashKey(c, "flash_error", "flash.checkin_done")
		} else {
			h.App.Log.WithError(err).Error("checkin transaction failed")
			h.setFlashKey(c, "flash_error", "flash.update_failed")
		}
		c.Redirect(http.StatusFound, "/profile")
		return
	}

	h.setFlashKey(c, "flash_success", "flash.checkin_success")
	c.Redirect(http.StatusFound, "/profile")
}
