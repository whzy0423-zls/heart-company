package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/signup"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
)

// miniappRole 标记小程序用户的 JWT 角色，区别于后台 RBAC 用户。
const miniappRole = "miniapp"

// requireMiniapp 校验小程序 JWT（角色含 miniapp），并把 wx_user_id 放入上下文。
func (s *Server) requireMiniapp(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.authorize(w, r)
		if !ok {
			return
		}
		isMini := false
		for _, role := range user.Roles {
			if role == miniappRole {
				isMini = true
				break
			}
		}
		if !isMini || user.ID <= 0 {
			httpx.Fail(w, http.StatusForbidden, "Forbidden")
			return
		}
		next(w, r.WithContext(withUser(r.Context(), user)))
	}
}

// wxLogin 用 wx.login 的 code 换取登录态，签发小程序 JWT。
func (s *Server) wxLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var body struct {
		Code    string `json:"code"`
		Channel string `json:"channel"`
		Scene   string `json:"scene"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	sess, err := s.wx.Code2Session(r.Context(), body.Code)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	uid, err := s.miniapp.UpsertByOpenID(r.Context(), sess.OpenID, sess.UnionID, body.Channel, body.Scene)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	user, err := s.miniapp.GetUser(r.Context(), uid)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, err := auth.Sign(auth.UserInfo{
		ID:       uid,
		UserID:   strconv.FormatInt(uid, 10),
		Username: sess.OpenID,
		RealName: user.Nickname,
		Avatar:   user.Avatar,
		Roles:    []string{miniappRole},
		HomePath: "/pages/index/index",
	}, s.env.JWTSecret)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, map[string]any{
		"accessToken": token,
		"user":        user,
		"devMode":     s.wx.DevMode(),
	})
}

// wxUserInfo GET 读取 / PUT 更新当前小程序用户资料。
func (s *Server) wxUserInfo(w http.ResponseWriter, r *http.Request) {
	uid := userFromRequest(r).ID
	switch r.Method {
	case http.MethodGet:
		user, err := s.miniapp.GetUser(r.Context(), uid)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, user)
	case http.MethodPut:
		var in miniapp.ProfileUpdate
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		user, err := s.miniapp.UpdateUser(r.Context(), uid, in)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, user)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// miniappTestRecords GET 列表 / POST 存档一次测试结果。
func (s *Server) miniappTestRecords(w http.ResponseWriter, r *http.Request) {
	uid := userFromRequest(r).ID
	switch r.Method {
	case http.MethodGet:
		items, err := s.miniapp.ListTestRecords(r.Context(), uid)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, map[string]any{"items": items})
	case http.MethodPost:
		var in miniapp.TestRecordInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		rec, err := s.miniapp.SaveTestRecord(r.Context(), uid, in)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, rec)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// miniappBookings GET 我的预约 / POST 提交预约（同时落后台客户线索）。
func (s *Server) miniappBookings(w http.ResponseWriter, r *http.Request) {
	uid := userFromRequest(r).ID
	switch r.Method {
	case http.MethodGet:
		items, err := s.miniapp.ListBookings(r.Context(), uid)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, map[string]any{"items": items})
	case http.MethodPost:
		var in miniapp.BookingInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		// 同步写一条后台客户线索，运营在「客户管理」可见
		var signupID int64
		lead, lerr := s.signups.Create(r.Context(), signup.LeadInput{
			Name:        in.ContactName,
			Contact:     in.Phone,
			ContactType: "phone",
			Interest:    bookingInterest(in),
			Message:     in.Message,
		}, r)
		if lerr == nil {
			if parsed, perr := strconv.ParseInt(lead.ID, 10, 64); perr == nil {
				signupID = parsed
			}
			s.broadcastSignup(lead)
		}
		booking, err := s.miniapp.CreateBooking(r.Context(), uid, in, signupID)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, booking)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// miniappChat 基于站点内容、九型资料和用户档案做轻量 RAG 问答。
func (s *Server) miniappChat(w http.ResponseWriter, r *http.Request) {
	uid := userFromRequest(r).ID
	if !s.chatLimiter.Allow(uid, time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "提问太频繁了，请稍后再试")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	ctx, cancel := context.WithTimeout(r.Context(), s.chatTimeout)
	defer cancel()

	var body struct {
		Question string        `json:"question"`
		History  []rag.Message `json:"history"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	user, err := s.miniapp.GetUser(ctx, uid)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	docs, err := s.retrieveDocsForQuery(ctx, body.Question, 6)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	service := rag.NewService(docs, rag.WithGenerator(s.ragGen))
	answer, err := service.Ask(ctx, rag.AskInput{
		History:  body.History,
		Question: body.Question,
		UserProfile: rag.UserProfile{
			Nickname: user.Nickname,
			MainType: user.MainType,
		},
	})
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, answer)
}

func (s *Server) miniappRAGDocuments(ctx context.Context) ([]rag.Document, error) {
	return s.ragCache.Get(ctx, func(loadCtx context.Context) ([]rag.Document, error) {
		config, err := siteconfig.ReadStore(loadCtx, s.db, s.env.SiteConfig)
		if err != nil {
			return nil, err
		}
		knowledgeDocs, err := s.ragDocs.EnabledDocuments(loadCtx)
		if err != nil {
			return nil, err
		}
		return mergeMiniappRAGDocuments(miniappRAGDocuments(config), knowledgeDocs), nil
	})
}

func mergeMiniappRAGDocuments(siteDocs []rag.Document, knowledgeDocs []rag.Document) []rag.Document {
	docs := make([]rag.Document, 0, len(siteDocs)+len(knowledgeDocs))
	docs = append(docs, siteDocs...)
	docs = append(docs, knowledgeDocs...)
	return docs
}

func bookingInterest(in miniapp.BookingInput) string {
	label := map[string]string{
		"consult":    "1v1 咨询预约",
		"course":     "课程报名",
		"enterprise": "企业课程咨询",
	}[in.Kind]
	if label == "" {
		label = "小程序预约"
	}
	if in.Intent != "" {
		return label + " · " + in.Intent
	}
	return label
}

func miniappRAGDocuments(config siteconfig.SiteConfig) []rag.Document {
	docs := []rag.Document{}
	for _, item := range config.Types {
		title := strings.TrimSpace(item.ID + "号 " + item.Name)
		content := strings.TrimSpace(item.Description + " " + item.Keywords)
		if content == "" {
			continue
		}
		docs = append(docs, rag.Document{
			ID:      "type-" + item.ID,
			Title:   title,
			Content: content,
			Tags:    []string{item.ID + "号", item.Name, item.Keywords},
		})
	}

	if courses, ok := config.Home["courses"].(map[string]any); ok {
		if items, ok := courses["items"].([]any); ok {
			for i, raw := range items {
				item, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				title := stringValue(item["title"])
				description := stringValue(item["description"])
				if title == "" || description == "" {
					continue
				}
				tags := []string{"课程"}
				if bullets, ok := item["bullets"].([]any); ok {
					for _, bullet := range bullets {
						if text := stringValue(bullet); text != "" {
							tags = append(tags, text)
							description += " " + text
						}
					}
				}
				docs = append(docs, rag.Document{
					ID:      "course-" + strconv.Itoa(i+1),
					Title:   title,
					Content: description,
					Tags:    tags,
				})
			}
		}
	}
	return docs
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
