package server

import (
	"fmt"
	"net/http"
	"strings"
)

// publicStorySkillCover supplies a stable, dependency-free default cover for
// story skills. Editors can later replace this URL with uploaded artwork.
func (s *Server) publicStorySkillCover(w http.ResponseWriter, r *http.Request) {
	key := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/public/story-skill-covers/"), "/")
	key = strings.TrimSuffix(key, ".svg")
	if key == "" || strings.ContainsAny(key, "/\\") {
		http.NotFound(w, r)
		return
	}
	category := "story"
	for _, item := range storySkillCategories {
		if strings.HasPrefix(key, item.key+"-") {
			category = item.name
			break
		}
	}
	colors := map[string][2]string{
		"神话":    {"#4338CA", "#C4B5FD"},
		"民间":    {"#166534", "#86EFAC"},
		"童话":    {"#9D174D", "#F9A8D4"},
		"小说":    {"#1D4ED8", "#93C5FD"},
		"现实":    {"#92400E", "#FCD34D"},
		"story": {"#334155", "#CBD5E1"},
	}
	pair := colors[category]
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360" role="img" aria-label="%s故事技能封面"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient></defs><rect width="640" height="360" rx="28" fill="url(#g)"/><circle cx="530" cy="84" r="92" fill="#fff" fill-opacity=".13"/><circle cx="104" cy="300" r="140" fill="#000" fill-opacity=".08"/><path d="M320 72l22 50 54 6-41 35 12 53-47-28-47 28 12-53-41-35 54-6z" fill="#fff" fill-opacity=".86"/><text x="44" y="304" fill="#fff" font-family="-apple-system,BlinkMacSystemFont,'PingFang SC',sans-serif" font-size="34" font-weight="700">%s</text></svg>`, category, pair[0], pair[1], category)
}
