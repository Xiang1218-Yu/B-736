package handler

import "prompt736/internal/types"

// HomePage 首页
func (h *Handler) HomePage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = ""
	data.ActiveNav = "home"

	if payload, err := h.App.GetHomePagePayload(); err == nil {
		data.Stats = payload.Stats
		data.HotResources = payload.HotResources
		data.LatestArticles = payload.LatestArticles
		data.Categories = payload.Categories
	} else {
		h.App.Log.WithError(err).Warn("load home page cache payload failed")
	}

	h.renderPage(c, "home.html", data)
}
