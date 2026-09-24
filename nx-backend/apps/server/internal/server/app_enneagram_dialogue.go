package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

var enneagramDialogueNames = [...]string{"", "完美型", "给予型", "成就型", "浪漫型", "观察型", "怀疑型", "享乐型", "领导型", "调停型"}
var enneagramDialogueStyles = [...]string{
	"",
	"认真、克制、有条理；先澄清标准，再给可执行步骤。重视原则但不苛责、不说教。",
	"温暖、体贴、关注具体感受；先回应情绪与关系需要，再给帮助。尊重边界，不讨好或替人决定。",
	"干练、积极、目标明确；先抓重点和结果，再给高效行动。兼顾感受，不把人的价值等同成绩。",
	"真诚、细腻、关注独特体验；用自然有画面感的表达回应感受。避免夸张煽情与无依据的共情。",
	"冷静、简洁、好奇；先辨别事实与假设，给清楚解释，保留思考空间。避免疏离与堆砌术语。",
	"审慎、可靠、坦诚；讲清依据、风险和可行预案，帮助确认下一步。避免制造焦虑或绝对保证。",
	"轻快、开阔、充满好奇；提供少量新视角，再落到一件可做的事。不回避痛苦，不强行乐观。",
	"直接、坚定、有担当；明确重点和边界，给有力量的选择。尊重对方自主，不命令、不攻击。",
	"平和、耐心、包容；整合不同视角，帮助表达真实需要，再推动一个温和的行动。不一味附和。",
}

func enneagramDialogueInstructions(ctx context.Context) string {
	mainType := chat.EnneagramType(ctx)
	if mainType == 0 {
		return ""
	}
	return fmt.Sprintf("你是九型对话中的 %d号%s AI 对话伙伴。每一轮保持所选型号的语气、措辞和组织方式：%s\n这是用户主动选择的交流角色，不是用户的测评类型，不据此推断用户身份、经历或心理状态。只使用本会话中用户真实提供的事实，不虚构真实人物身份。用户要求换型号时请建议返回九型对话重新选择，不擅自切换当前角色。用自然简体中文直接回应当前问题，不反复介绍型号、不套模板。除用户明确需要长篇外先简短回应，再给一个贴合上下文的追问。九型只用于自我观察，不作医疗诊断。", mainType, enneagramDialogueNames[mainType], enneagramDialogueStyles[mainType])
}

// The role lives in the authenticated route, never in the user message.
func (s *Server) appEnneagramDialogueRouter(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/app/enneagram/"), "/", 2)
	if len(parts) != 2 {
		httpx.Fail(w, http.StatusNotFound, "对话入口不存在")
		return
	}
	mainType, err := strconv.Atoi(parts[0])
	if err != nil || mainType < 1 || mainType > 9 || parts[0] != strconv.Itoa(mainType) {
		httpx.Fail(w, http.StatusBadRequest, "请选择1至9号")
		return
	}
	path := "/api/app/" + parts[1]
	scoped := r.Clone(chat.WithEnneagramType(r.Context(), mainType))
	scoped.URL.Path = path
	switch {
	case path == "/api/app/chat/sessions" || strings.HasPrefix(path, "/api/app/chat/sessions/"):
		s.appChatRouter(w, scoped)
	case strings.HasPrefix(path, "/api/app/chat/messages/"):
		s.appChatMessageRouter(w, scoped)
	case path == "/api/app/chat/favorites" && r.Method == http.MethodGet:
		s.appChatFavorites(w, scoped)
	case path == "/api/app/chat/search" && r.Method == http.MethodGet:
		s.appChatSearch(w, scoped)
	default:
		httpx.Fail(w, http.StatusNotFound, "对话入口不存在")
	}
}
