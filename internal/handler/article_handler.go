package handler

import (
	"fmt"
	"net/http"
	"strings"

	"prompt736/internal/models"
	"prompt736/internal/utils"

	"github.com/gin-gonic/gin"
)

// ArticlesPage 文章列表页
func (h *Handler) ArticlesPage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "article.title")
	data.ActiveNav = "articles"

	page := utils.ParseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := 10
	data.Page = page
	data.PageSize = pageSize

	if payload, err := h.App.GetArticleListPayload(page, pageSize); err == nil {
		data.Total = payload.Total
		data.TotalPages = int((payload.Total + int64(pageSize) - 1) / int64(pageSize))
		data.Articles = payload.Articles
	} else {
		h.App.Log.WithError(err).Warn("load article list cache payload failed")
	}

	h.renderPage(c, "articles.html", data)
}

// ArticleDetailPage 文章详情页
func (h *Handler) ArticleDetailPage(c *gin.Context) {
	id := utils.ParseUint(c.Param("id"))
	if id == 0 {
		c.Redirect(http.StatusFound, "/articles")
		return
	}

	article, err := h.App.GetArticleByID(id)
	if err != nil || article.Status != "published" {
		c.Redirect(http.StatusFound, "/articles")
		return
	}

	h.App.IncrementArticleViews(id)

	comments, _ := h.App.GetArticleComments(id)

	data := h.basePageData(c)
	data.Title = article.Title
	data.ActiveNav = "articles"
	data.Article = article
	data.Comments = comments

	h.renderPage(c, "article_detail.html", data)
}

// ArticleCommentHandler 文章评论处理
func (h *Handler) ArticleCommentHandler(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		h.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}
	user, ok := userValue.(*models.User)
	if !ok || user == nil {
		h.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	articleID := utils.ParseUint(c.Param("id"))
	content := strings.TrimSpace(c.PostForm("content"))

	if articleID == 0 || content == "" {
		h.setFlashKey(c, "flash_error", "flash.comment_failed")
		if articleID == 0 {
			c.Redirect(http.StatusFound, "/articles")
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	err := h.App.AddArticleComment(user.ID, articleID, content)
	if err != nil {
		h.App.Log.WithError(err).Error("create article comment failed")
		h.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	h.setFlashKey(c, "flash_success", "flash.comment_success")
	c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
}
