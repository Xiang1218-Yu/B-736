package app

import (
	"strings"
)

// adText 获取国际化广告文本
func (app *App) adText(locale, key, fallback string) string {
	value := strings.TrimSpace(app.I18n.T(locale, key))
	if value == "" || value == key {
		return fallback
	}
	return value
}

// defaultAdSlots 获取默认广告位配置
func (app *App) defaultAdSlots(locale string) map[string]AdSlot {
	defaultContact := app.adText(locale, "ad.default_contact", "Business Contact: WeChat GalaxyHub_BD / Email business@galaxyhub.example")
	return map[string]AdSlot{
		"hero-banner": {
			Code:    "hero-banner",
			Title:   app.adText(locale, "ad.slot_defaults.hero_banner.title", "品牌合作专区"),
			Desc:    app.adText(locale, "ad.slot_defaults.hero_banner.desc", "预留标准广告位，支持后续广告系统对接。"),
			CTA:     app.adText(locale, "ad.slot_defaults.hero_banner.cta", app.adText(locale, "ad.learn_more", "Learn More")),
			Contact: app.adText(locale, "ad.slot_defaults.hero_banner.contact", defaultContact),
			Enabled: true,
		},
		"resource-list-inline": {
			Code:    "resource-list-inline",
			Title:   app.adText(locale, "ad.slot_defaults.resource_list_inline.title", "精选推广资源位"),
			Desc:    app.adText(locale, "ad.slot_defaults.resource_list_inline.desc", "资源列表内标准广告位，可按 code 接入投放系统。"),
			CTA:     app.adText(locale, "ad.slot_defaults.resource_list_inline.cta", app.adText(locale, "ad.learn_more", "Learn More")),
			Contact: app.adText(locale, "ad.slot_defaults.resource_list_inline.contact", defaultContact),
			Enabled: true,
		},
		"resource-detail-sidebar": {
			Code:    "resource-detail-sidebar",
			Title:   app.adText(locale, "ad.slot_defaults.resource_detail_sidebar.title", "侧边广告位"),
			Desc:    app.adText(locale, "ad.slot_defaults.resource_detail_sidebar.desc", "详情页侧栏广告位，支持图文与跳转链接。"),
			CTA:     app.adText(locale, "ad.slot_defaults.resource_detail_sidebar.cta", app.adText(locale, "ad.learn_more", "Learn More")),
			Contact: app.adText(locale, "ad.slot_defaults.resource_detail_sidebar.contact", defaultContact),
			Enabled: true,
		},
		"article-detail-inline": {
			Code:    "article-detail-inline",
			Title:   app.adText(locale, "ad.slot_defaults.article_detail_inline.title", "内容推广位"),
			Desc:    app.adText(locale, "ad.slot_defaults.article_detail_inline.desc", "文章详情中部广告位，便于后续扩展素材形式。"),
			CTA:     app.adText(locale, "ad.slot_defaults.article_detail_inline.cta", app.adText(locale, "ad.learn_more", "Learn More")),
			Contact: app.adText(locale, "ad.slot_defaults.article_detail_inline.contact", defaultContact),
			Enabled: true,
		},
		"global-footer": {
			Code:    "global-footer",
			Title:   app.adText(locale, "ad.slot_defaults.global_footer.title", "全站底部广告位"),
			Desc:    app.adText(locale, "ad.slot_defaults.global_footer.desc", "全站统一广告位，适合品牌露出。"),
			CTA:     app.adText(locale, "ad.slot_defaults.global_footer.cta", app.adText(locale, "ad.learn_more", "Learn More")),
			Contact: app.adText(locale, "ad.slot_defaults.global_footer.contact", "Business Contact: WeChat GalaxyHub_Service / Email contact@galaxyhub.example"),
			Enabled: true,
		},
	}
}

// ParseAdSlots 解析广告配置 JSON 并合并默认值
func (app *App) ParseAdSlots(adsJSON, locale string) map[string]AdSlot {
	slots := app.defaultAdSlots(locale)
	cfg, err := UnmarshalAdConfig(adsJSON)
	if err != nil || cfg == nil {
		return slots
	}
	if IsLegacySeedAdConfig(*cfg) {
		return slots
	}

	for _, incoming := range cfg.Slots {
		code := strings.TrimSpace(incoming.Code)
		if code == "" {
			continue
		}

		slot, ok := slots[code]
		if !ok {
			slot = AdSlot{
				Code:    code,
				CTA:     app.adText(locale, "ad.learn_more", "Learn More"),
				Contact: app.adText(locale, "ad.default_contact", "Business Contact: WeChat GalaxyHub_BD / Email business@galaxyhub.example"),
				Enabled: true,
			}
		}

		if title := strings.TrimSpace(incoming.Title); title != "" {
			slot.Title = title
		}
		if desc := strings.TrimSpace(incoming.Desc); desc != "" {
			slot.Desc = desc
		}
		if link := strings.TrimSpace(incoming.Link); link != "" {
			slot.Link = link
		}
		if imageURL := strings.TrimSpace(incoming.ImageURL); imageURL != "" {
			slot.ImageURL = imageURL
		}
		if cta := strings.TrimSpace(incoming.CTA); cta != "" {
			slot.CTA = cta
		}
		if contact := strings.TrimSpace(incoming.Contact); contact != "" {
			slot.Contact = contact
		}
		if incoming.Enabled != nil {
			slot.Enabled = *incoming.Enabled
		}

		slots[code] = slot
	}

	return slots
}
