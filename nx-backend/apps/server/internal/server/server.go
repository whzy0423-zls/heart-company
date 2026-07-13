package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nine-xing/nx-backend/apps/server/internal/analytics"
	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/articlestore"
	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/branding"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/embedding"
	"nine-xing/nx-backend/apps/server/internal/engagement"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/image"
	"nine-xing/nx-backend/apps/server/internal/llm"
	"nine-xing/nx-backend/apps/server/internal/mindquote"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/netguard"
	"nine-xing/nx-backend/apps/server/internal/profilecalibration"
	"nine-xing/nx-backend/apps/server/internal/push"
	"nine-xing/nx-backend/apps/server/internal/quiz"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/ragstore"
	"nine-xing/nx-backend/apps/server/internal/realip"
	"nine-xing/nx-backend/apps/server/internal/signup"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
	"nine-xing/nx-backend/apps/server/internal/sms"
	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/system"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
	"nine-xing/nx-backend/apps/server/internal/video"
	"nine-xing/nx-backend/apps/server/internal/videoanalysis"
	"nine-xing/nx-backend/apps/server/internal/videoasset"
	"nine-xing/nx-backend/apps/server/internal/videostoryboard"
	"nine-xing/nx-backend/apps/server/internal/voice"
	"nine-xing/nx-backend/apps/server/internal/wechat"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

type Server struct {
	env                     config.Env
	mux                     *http.ServeMux
	db                      *sql.DB
	system                  *system.Store
	analytics               *analytics.Store
	auditLogs               *auditlog.Store
	builder                 *siteconfig.Builder
	engagement              *engagement.Store
	signups                 *signup.Store
	uploads                 *uploadasset.Store
	uploader                storage.ObjectUploader
	voices                  *voice.Store
	videos                  *video.Store
	videoAnalysis           *videoanalysis.Store
	videoAssets             *videoasset.Store
	storyboards             *videostoryboard.Store
	images                  *image.Store
	miniapp                 *miniapp.Store
	wx                      *wechat.Client
	pay                     *wxpay.Client
	ragGen                  rag.Generator
	analysisGen             *llm.MiniMaxGenerator
	ragDocs                 ragDocumentStore
	ragVec                  *ragstore.Store
	embedder                *embedding.Client
	ragCache                *miniappRAGCache
	articles                *articlestore.Store
	mindquotes              *mindquote.Store
	quiz                    *quiz.Store
	appDailyQuiz            appDailyQuizService
	appReassessment         appReassessmentService
	appDailyQuizReminders   appDailyQuizReminderService
	appDailyQuizPushAdmin   appDailyQuizPushAdminService
	appDailyQuizBankAdmin   appDailyQuizBankAdminService
	chatLimiter             *fixedWindowRateLimiter
	chatTimeout             time.Duration
	modelConfigProbeTimeout time.Duration
	newChatGenerator        func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error)

	appUsers                 *appuser.Store
	appChat                  appChatStore
	pushStore                *push.Store
	pushSendTimeout          time.Duration
	pushRecoveryInterval     time.Duration
	pushSendSlots            chan struct{}
	smsSender                sms.Sender
	loginLimiter             *strRateLimiter
	loginDBLimiter           *dbRateLimiter
	smsPhoneLimiter          *strRateLimiter
	smsIPLimiter             *strRateLimiter
	smsVerifyPhoneLimiter    *strRateLimiter
	smsVerifyIPLimiter       *strRateLimiter
	publicSignupIPLimiter    *strRateLimiter
	publicAnalyticsIPLimiter *strRateLimiter
	publicGameIPLimiter      *strRateLimiter
	videoAnalysisSlots       chan struct{}
	videoStoryboardSlots     chan struct{}
	trustedProxyCIDRs        []netip.Prefix

	signupMu          sync.Mutex
	signupSubscribers map[chan signup.Lead]struct{}

	// modelMu 保护可在运行时被"模型配置"页面重建的 ragGen / analysisGen / videos。
	modelMu             sync.RWMutex
	modelConfigUpdateMu sync.Mutex
}

var uploadPermissionCodes = []string{
	"RAG:Knowledge:Manage",
	"Reading:Article:Manage",
	"System:Branding",
	"Video:Analysis:Manage",
	"Video:Asset:Manage",
	"Video:Generate:Manage",
	"Video:Storyboard:Manage",
	"Voice:Content:Manage",
	"Voice:Profile:Manage",
	"Website:Write",
}

const defaultModelConfigProbeTimeout = 15 * time.Second

func New(env config.Env, database *sql.DB) http.Handler {
	s := newServer(env, database)
	return s.withCORS(s.mux)
}

func newServer(env config.Env, database *sql.DB) *Server {
	trustedProxyCIDRs, err := realip.ParseTrustedProxyCIDRs(env.TrustedProxyCIDRs)
	if err != nil {
		panic("trusted proxy cidrs: " + err.Error())
	}
	s := &Server{
		env:               env,
		mux:               http.NewServeMux(),
		db:                database,
		system:            system.NewStore(database),
		analytics:         analytics.NewStore(database),
		auditLogs:         auditlog.NewStore(database),
		builder:           siteconfig.NewBuilder(env.BuildScript, "", time.Duration(env.BuildTimeout)*time.Second),
		engagement:        engagement.NewStore(database),
		signups:           signup.NewStore(database),
		uploads:           uploadasset.NewStore(database),
		uploader:          env.ObjectUploader,
		trustedProxyCIDRs: trustedProxyCIDRs,

		signupSubscribers: map[chan signup.Lead]struct{}{},
	}
	if uploader, err := s.objectUploader(); err == nil {
		s.uploader = uploader
	}
	s.voices = voice.NewStore(database, s.uploads, env.MiniMax)
	s.videos = video.NewStore(database, s.uploads, env.Video, s.uploader)
	s.videoAnalysis = videoanalysis.NewStore(database)
	s.videoAssets = videoasset.NewStore(database, s.uploads)
	s.storyboards = videostoryboard.NewStore(database)
	s.images = image.NewStore(s.uploads, env.Image, s.uploader)
	s.miniapp = miniapp.NewStore(database)
	s.wx = newWeChatClient(env)
	s.pay = mustWxPayClient(env)
	// Interactive chat is intentionally unconfigured until a stored compatible
	// provider exists. Legacy MiniMax environment credentials are still used by
	// analysis/voice features, but never as an implicit chat fallback.
	s.ragGen = nil
	s.analysisGen = llm.NewMiniMaxGenerator(modelconfig.Config{}.ApplyAnalysis(env.MiniMax))
	s.ragDocs = ragstore.NewStore(database)
	s.articles = articlestore.NewStore(database)
	s.mindquotes = mindquote.NewStore(database)
	// 听书：复用 voice 的 MiniMax 客户端与 upload-assets 存储生成并缓存音频。
	s.articles.AttachAudioDeps(s.voices, s.uploads, s.voices, "speech-02-hd")
	s.ragVec = ragstore.NewStore(database)
	s.embedder = embedding.NewClient(embedding.Config{
		Provider:  env.Embedding.Provider,
		APIBase:   env.Embedding.APIBase,
		APIKey:    env.Embedding.APIKey,
		Model:     env.Embedding.Model,
		Dimension: env.Embedding.Dimension,
	})
	s.ragCache = newMiniappRAGCache(2 * time.Minute)
	s.chatLimiter = newFixedWindowRateLimiter(env.MiniappChat.RateLimitPerMinute, time.Minute)
	// MINIAPP_CHAT_TIMEOUT_SECONDS 仅是尚未配置 compatible chat 时的旧版业务超时；
	// 一旦加载/保存有效 chat.timeoutSeconds，运行时会在同一把锁下替换它。
	s.chatTimeout = time.Duration(env.MiniappChat.TimeoutSeconds) * time.Second
	if s.chatTimeout <= 0 {
		s.chatTimeout = 28 * time.Second
	}
	s.appUsers = appuser.NewStore(database)
	s.quiz = quiz.NewStore(database)
	if database != nil {
		profileStore := profilecalibration.NewStore(database)
		profileStore.SetDailyQuizQuestionGenerator(serverDailyQuizQuestionGenerator{server: s})
		s.appDailyQuiz = profileStore
		s.appReassessment = profileStore
		s.appDailyQuizReminders = profileStore
		s.appDailyQuizPushAdmin = profileStore
		s.appDailyQuizBankAdmin = profileStore
	}
	s.appChat = chat.NewStore(database)
	s.loginLimiter = newStrRateLimiter(10, time.Minute)
	s.loginDBLimiter = newDBRateLimiter(database, "admin_login", 10, time.Minute)
	s.smsPhoneLimiter = newStrRateLimiter(1, time.Minute)
	s.smsIPLimiter = newStrRateLimiter(10, time.Minute)
	s.smsVerifyPhoneLimiter = newStrRateLimiter(5, time.Minute)
	s.smsVerifyIPLimiter = newStrRateLimiter(20, time.Minute)
	s.publicSignupIPLimiter = newStrRateLimiter(5, time.Minute)
	s.publicAnalyticsIPLimiter = newStrRateLimiter(120, time.Minute)
	s.publicGameIPLimiter = newStrRateLimiter(30, time.Minute)
	s.videoAnalysisSlots = make(chan struct{}, 3)
	s.videoStoryboardSlots = make(chan struct{}, 3)
	s.smsSender = mustSMSSender(env.SMS)
	s.pushStore = push.NewStore(database, push.NewPusher(env.AppEnv, env.JPush.AppKey, env.JPush.MasterSecret))
	s.pushSendSlots = make(chan struct{}, 2)
	// 启动时应用 DB 中保存的模型配置覆盖（若存在），重建对话/视频客户端。
	s.applyStoredModelConfig()
	s.routes()
	if database != nil {
		go s.recoverVideoAsyncTasks()
		go s.runPushAsyncRecoveryLoop(context.Background())
		go s.runProfileCalibrationPushLoop(context.Background())
	}
	return s
}

func newWeChatClient(env config.Env) *wechat.Client {
	if config.IsProduction(env.AppEnv) {
		return wechat.NewClient(env.WeChat.AppID, env.WeChat.Secret, false)
	}
	return wechat.NewClientWithMissingCredentialFallback(env.WeChat.AppID, env.WeChat.Secret, env.WeChat.LoginDev)
}

// applyStoredModelConfig 读取 DB 中持久化的模型配置覆盖（若有），
// 用覆盖后的凭据重建对话/视频客户端。对话不回退到 MiniMax 环境变量。
func (s *Server) applyStoredModelConfig() {
	cfg, ok, err := modelconfig.ReadStore(context.Background(), s.db)
	if err != nil || !ok {
		return
	}
	var chatGenerator llm.ChatGenerator
	if cfg.AssistEnabled() {
		chatGenerator, err = s.buildChatGenerator(cfg)
		if err != nil {
			log.Printf("stored chat model remains unconfigured: %v", err)
		}
	}
	analysisGenerator := llm.NewMiniMaxGenerator(safeStoredAnalysisConfig(cfg, s.env.MiniMax))
	videoStore := video.NewStore(s.db, s.uploads, safeStoredVideoConfig(cfg, s.env.Video), s.uploader)
	imageStore := image.NewStore(s.uploads, safeStoredImageConfig(cfg, s.env.Image), s.uploader)

	s.modelMu.Lock()
	s.ragGen = chatGenerator
	if chatGenerator != nil {
		s.chatTimeout = time.Duration(cfg.EffectiveChat().TimeoutSeconds) * time.Second
	}
	s.analysisGen = analysisGenerator
	s.videos = videoStore
	s.images = imageStore
	s.modelMu.Unlock()
}

// generator 返回当前生效的对话生成器；持读锁以兼容"模型配置"页面运行时重建。
func (s *Server) generator() rag.Generator {
	generator, _ := s.chatRuntime()
	return generator
}

func (s *Server) chatRuntime() (rag.Generator, time.Duration) {
	s.modelMu.RLock()
	defer s.modelMu.RUnlock()
	timeout := s.chatTimeout
	if timeout <= 0 {
		timeout = 28 * time.Second
	}
	return s.ragGen, timeout
}

func (s *Server) chatRequestTimeout() time.Duration {
	_, timeout := s.chatRuntime()
	return timeout
}

func (s *Server) buildChatGenerator(cfg modelconfig.Config) (llm.ChatGenerator, error) {
	chat := cfg.EffectiveChat()
	if err := chat.Validate(); err != nil {
		return nil, err
	}
	factory := s.newChatGenerator
	if factory == nil {
		factory = llm.NewChatGenerator
	}
	return factory(llm.ChatGeneratorConfig{
		Provider:     chat.Provider,
		APIBase:      chat.APIBase,
		APIKey:       chat.APIKey,
		Model:        chat.Model,
		SystemPrompt: cfg.Assist.SystemPrompt,
		Timeout:      time.Duration(chat.TimeoutSeconds) * time.Second,
	})
}

func (s *Server) modelConfigProbeDeadline(providerTimeout time.Duration) time.Duration {
	limit := s.modelConfigProbeTimeout
	if limit <= 0 {
		limit = defaultModelConfigProbeTimeout
	}
	if providerTimeout > 0 && providerTimeout < limit {
		return providerTimeout
	}
	return limit
}

func safeStoredVideoConfig(cfg modelconfig.Config, base config.VideoConfig) config.VideoConfig {
	if err := validateExternalAPIBase("video.apiBase", cfg.Video.APIBase); err != nil {
		log.Printf("ignore unsafe stored video model config: %v", err)
		return base
	}
	return cfg.ApplyVideo(base)
}

func safeStoredImageConfig(cfg modelconfig.Config, base config.ImageConfig) config.ImageConfig {
	if err := validateExternalAPIBase("image.apiBase", cfg.Image.APIBase); err != nil {
		log.Printf("ignore unsafe stored image model config: %v", err)
		return base
	}
	return cfg.ApplyImage(base)
}

func safeStoredAnalysisConfig(cfg modelconfig.Config, base config.MiniMaxConfig) config.MiniMaxConfig {
	if err := validateExternalAPIBase("analysis.apiBase", cfg.Analysis.APIBase); err != nil {
		log.Printf("ignore unsafe stored analysis model config: %v", err)
		return modelconfig.Config{}.ApplyAnalysis(base)
	}
	return cfg.ApplyAnalysis(base)
}

func (s *Server) analysisGenerator() *llm.MiniMaxGenerator {
	s.modelMu.RLock()
	defer s.modelMu.RUnlock()
	return s.analysisGen
}

// videoStore 返回当前生效的视频存储；持读锁以兼容运行时重建。
func (s *Server) videoStore() *video.Store {
	s.modelMu.RLock()
	defer s.modelMu.RUnlock()
	return s.videos
}

// imageStore 返回当前生效的文生图存储；持读锁以兼容运行时重建。
func (s *Server) imageStore() *image.Store {
	s.modelMu.RLock()
	defer s.modelMu.RUnlock()
	return s.images
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/status", s.method(http.MethodGet, s.status))
	s.mux.HandleFunc("/api/auth/login", s.method(http.MethodPost, s.login))
	s.mux.HandleFunc("/api/auth/logout", s.method(http.MethodPost, s.logout))
	s.mux.HandleFunc("/api/auth/refresh", s.method(http.MethodPost, s.requireAuth(s.refresh)))
	s.mux.HandleFunc("/api/user/info", s.method(http.MethodGet, s.requireAuth(s.userInfo)))
	s.mux.HandleFunc("/api/user/profile", s.method(http.MethodPut, s.requireAuth(s.updateUserProfile)))
	s.mux.HandleFunc("/api/auth/codes", s.method(http.MethodGet, s.requireAuth(s.codes)))
	s.mux.HandleFunc("/api/menu/all", s.method(http.MethodGet, s.requireAuth(s.menus)))
	s.mux.HandleFunc("/api/upload", s.method(http.MethodPost, s.requireUploadPermission(s.upload)))
	s.mux.HandleFunc("/api/upload-assets/", s.method(http.MethodGet, s.requireUploadAssetAuth(s.uploadAsset)))
	s.mux.HandleFunc("/api/uploads/", s.getOrHead(s.requireAuth(s.uploadedFile)))
	s.mux.HandleFunc("/api/site-config", s.siteConfig)
	s.mux.HandleFunc("/api/site-config/build-status", s.method(http.MethodGet, s.requirePermission("Website:Write", s.siteBuildStatus)))
	// 公开只读：给官网(website-react)运行时拉取，无需鉴权。
	s.mux.HandleFunc("/api/public/site-config", s.method(http.MethodGet, s.publicSiteConfig))
	s.mux.HandleFunc("/api/public/customer-service-qr", s.method(http.MethodGet, s.publicCustomerServiceQR))
	s.mux.HandleFunc("/api/public/signups", s.method(http.MethodPost, s.publicSignup))
	s.mux.HandleFunc("/api/public/game-results", s.method(http.MethodPost, s.publicGameResult))
	s.mux.HandleFunc("/api/public/site-visits", s.method(http.MethodPost, s.publicSiteVisit))
	s.mux.HandleFunc("/api/public/site-assets/", s.method(http.MethodGet, s.publicSiteAsset))
	s.mux.HandleFunc("/api/public/site-uploads/", s.method(http.MethodGet, s.publicSiteUpload))
	// 阅读 H5：公开只读文章列表 / 详情 / 分类。
	s.mux.HandleFunc("/api/public/articles", s.method(http.MethodGet, s.publicArticles))
	s.mux.HandleFunc("/api/public/articles/", s.method(http.MethodGet, s.publicArticleDetail))
	s.mux.HandleFunc("/api/public/article-assets/", s.method(http.MethodGet, s.publicArticleAsset))
	s.mux.HandleFunc("/api/public/article-uploads/", s.method(http.MethodGet, s.publicArticleUpload))
	s.mux.HandleFunc("/api/public/article-categories", s.method(http.MethodGet, s.publicArticleCategories))
	// 成长心语：官网公开只读（分组聚合 + 单条详情）。
	s.mux.HandleFunc("/api/public/mind-groups", s.method(http.MethodGet, s.publicMindGroups))
	s.mux.HandleFunc("/api/public/mind-quotes/", s.method(http.MethodGet, s.publicMindQuoteDetail))
	// 后台品牌：公开只读（启动屏/登录页在登录前就要用），写入需鉴权。
	s.mux.HandleFunc("/api/public/admin-branding", s.method(http.MethodGet, s.publicAdminBranding))
	s.mux.HandleFunc("/api/public/admin-branding-assets/", s.method(http.MethodGet, s.publicAdminBrandingAsset))
	s.mux.HandleFunc("/api/public/admin-branding-uploads/", s.method(http.MethodGet, s.publicAdminBrandingUpload))
	s.mux.HandleFunc("/api/admin-branding", s.requirePermission("System:Branding", s.adminBranding))
	// 模型配置：读取/保存兼容协议对话模型及视频、图片、分析模型配置，均需登录。
	s.mux.HandleFunc("/api/model-config", s.requirePermission("System:Model:Config", s.modelConfig))
	// 对话模型连通性测试：通过统一 provider 工厂做一次轻量探活，需登录。
	s.mux.HandleFunc("/api/model-config/test-chat", s.requirePermission("System:Model:Config", s.method(http.MethodPost, s.testChatModel)))
	// ===== App API =====
	s.mux.HandleFunc("/api/app/health", s.method(http.MethodGet, s.appHealth))
	s.mux.HandleFunc("/api/app/auth/send-sms", s.method(http.MethodPost, s.appSendSMS))
	s.mux.HandleFunc("/api/app/auth/sms", s.method(http.MethodPost, s.appSendSMS))
	s.mux.HandleFunc("/api/app/auth/sms/send", s.method(http.MethodPost, s.appSendSMS))
	s.mux.HandleFunc("/api/app/auth/verify-sms", s.method(http.MethodPost, s.appVerifySMS))
	s.mux.HandleFunc("/api/app/auth/sms/login", s.method(http.MethodPost, s.appVerifySMS))
	s.mux.HandleFunc("/api/app/auth/refresh", s.method(http.MethodPost, s.appRefreshToken))
	s.mux.HandleFunc("/api/app/auth/logout", s.method(http.MethodPost, s.appLogout))
	s.mux.HandleFunc("/api/app/user/info", s.method(http.MethodGet, s.requireAppAuth(s.appUserInfo)))
	s.mux.HandleFunc("/api/app/me", s.method(http.MethodGet, s.requireAppAuth(s.appUserInfo)))
	// 测评问卷 + 命运卡片
	s.mux.HandleFunc("/api/app/quiz/questions", s.method(http.MethodGet, s.appQuizQuestions))
	s.mux.HandleFunc("/api/app/quiz/submit", s.method(http.MethodPost, s.requireAppAuth(s.appQuizSubmit)))
	s.mux.HandleFunc("/api/app/quiz/submission", s.method(http.MethodGet, s.requireAppAuth(s.appQuizSubmission)))
	s.mux.HandleFunc("/api/app/cards", s.requireAppAuth(s.appCards))
	s.mux.HandleFunc("/api/app/cards/primary", s.method(http.MethodGet, s.requireAppAuth(s.appCardPrimary)))
	s.mux.HandleFunc("/api/app/cards/", s.requireAppAuth(s.appCardByID))
	s.mux.HandleFunc("/api/app/compatibility", s.requireAppAuth(s.appCompatibilityRouter))
	s.mux.HandleFunc("/api/app/compatibility/", s.requireAppAuth(s.appCompatibilityRouter))
	s.mux.HandleFunc("/api/app/daily-quiz/today", s.method(http.MethodGet, s.requireAppAuth(s.appDailyQuizToday)))
	s.mux.HandleFunc("/api/app/daily-quiz/progress", s.method(http.MethodGet, s.requireAppAuth(s.appDailyQuizProgress)))
	s.mux.HandleFunc("/api/app/daily-quiz/answer", s.method(http.MethodPost, s.requireAppAuth(s.appDailyQuizAnswer)))
	s.mux.HandleFunc("/api/app/daily-quiz/complete", s.method(http.MethodPost, s.requireAppAuth(s.appDailyQuizComplete)))
	s.mux.HandleFunc("/api/app/reassessment/latest", s.method(http.MethodGet, s.requireAppAuth(s.appReassessmentLatest)))
	s.mux.HandleFunc("/api/app/reassessment/", s.requireAppAuth(s.appReassessmentRouter))
	s.mux.HandleFunc("/api/app/chat/sessions", s.requireAppAuth(s.appChatRouter))
	s.mux.HandleFunc("/api/app/chat/sessions/", s.requireAppAuth(s.appChatRouter))
	s.mux.HandleFunc("/api/app/chat/messages/", s.requireAppAuth(s.appChatMessageRouter))
	s.mux.HandleFunc("/api/app/chat/favorites", s.method(http.MethodGet, s.requireAppAuth(s.appChatFavorites)))
	s.mux.HandleFunc("/api/app/chat/search", s.method(http.MethodGet, s.requireAppAuth(s.appChatSearch)))
	s.mux.HandleFunc("/api/app/billing/entitlements", s.method(http.MethodGet, s.requireAppAuth(s.appBillingEntitlements)))
	s.mux.HandleFunc("/api/app/billing/products", s.method(http.MethodGet, s.requireAppAuth(s.appBillingProducts)))
	s.mux.HandleFunc("/api/app/billing/orders", s.method(http.MethodPost, s.requireAppAuth(s.appBillingCreateOrder)))
	s.mux.HandleFunc("/api/app/billing/orders/status", s.method(http.MethodGet, s.requireAppAuth(s.appBillingOrderStatus)))
	s.mux.HandleFunc("/api/app/memories/", s.requireAppAuth(s.appMemoryRouter))
	s.mux.HandleFunc("/api/app/privacy/export", s.method(http.MethodPost, s.requireAppAuth(s.appPrivacyExport)))
	s.mux.HandleFunc("/api/app/privacy/memories", s.method(http.MethodDelete, s.requireAppAuth(s.appPrivacyDeleteMemories)))
	s.mux.HandleFunc("/api/app/privacy/account", s.method(http.MethodDelete, s.requireAppAuth(s.appPrivacyDeleteAccount)))
	s.mux.HandleFunc("/api/app/privacy/policy", s.method(http.MethodGet, s.appPrivacyPolicy))
	s.mux.HandleFunc("/api/app/analytics/event", s.method(http.MethodPost, s.requireAppAuth(s.appAnalyticsEvent)))
	// 每日成长练习 + 打卡
	s.mux.HandleFunc("/api/app/daily/practice", s.method(http.MethodGet, s.requireAppAuth(s.appDailyPractice)))
	s.mux.HandleFunc("/api/app/daily/checkin", s.method(http.MethodPost, s.requireAppAuth(s.appDailyCheckin)))
	// 成长周报
	s.mux.HandleFunc("/api/app/reports", s.method(http.MethodGet, s.requireAppAuth(s.appReportList)))
	s.mux.HandleFunc("/api/app/reports/", s.requireAppAuth(s.appReportRouter))
	// 推送设备令牌注册/注销
	s.mux.HandleFunc("/api/app/push/register", s.method(http.MethodPost, s.requireAppAuth(s.appPushRegister)))
	s.mux.HandleFunc("/api/app/push/unregister", s.method(http.MethodPost, s.requireAppAuth(s.appPushUnregister)))
	// 语音识别
	s.mux.HandleFunc("/api/app/voice/recognize", s.method(http.MethodPost, s.requireAppAuth(s.appVoiceRecognize)))

	// ===== 小程序（微信）=====
	s.mux.HandleFunc("/api/wx/login", s.method(http.MethodPost, s.wxLogin))
	s.mux.HandleFunc("/api/wx/userinfo", s.requireMiniapp(s.wxUserInfo))
	s.mux.HandleFunc("/api/miniapp/test-records", s.requireMiniapp(s.miniappTestRecords))
	s.mux.HandleFunc("/api/miniapp/bookings", s.requireMiniapp(s.miniappBookings))
	s.mux.HandleFunc("/api/miniapp/chat", s.method(http.MethodPost, s.requireMiniapp(s.miniappChat)))
	// 付费解锁：下单（鉴权）→ 微信回调（公开）→ 解锁状态/报告正文（鉴权）
	s.mux.HandleFunc("/api/miniapp/report/order", s.method(http.MethodPost, s.requireMiniapp(s.createReportOrder)))
	s.mux.HandleFunc("/api/miniapp/report/dev-pay", s.method(http.MethodPost, s.requireMiniapp(s.devPayReportOrder)))
	s.mux.HandleFunc("/api/miniapp/report/status", s.method(http.MethodGet, s.requireMiniapp(s.reportStatus)))
	s.mux.HandleFunc("/api/miniapp/report/content", s.method(http.MethodGet, s.requireMiniapp(s.reportContent)))
	s.mux.HandleFunc("/api/pay/notify", s.method(http.MethodPost, s.payNotify))
	s.mux.HandleFunc("/api/analytics/overview", s.method(http.MethodGet, s.requirePermission("Analytics:Overview", s.analyticsOverview)))
	s.mux.HandleFunc("/api/app-analytics/overview", s.method(http.MethodGet, s.requirePermission("Analytics:App:Overview", s.appAnalyticsOverview)))
	s.mux.HandleFunc("/api/game-results/overview", s.method(http.MethodGet, s.requirePermission("Analytics:GameResults", s.gameOverview)))
	s.mux.HandleFunc("/api/messages/list", s.method(http.MethodGet, s.requirePermission("Message:Manage:List", s.messagesList)))
	s.mux.HandleFunc("/api/messages/read", s.method(http.MethodPut, s.requirePermission("Message:Manage:List", s.markMessages)))
	// 推送管理（admin）
	s.mux.HandleFunc("/api/push/list", s.method(http.MethodGet, s.requirePermission("Push:Manage", s.adminPushList)))
	s.mux.HandleFunc("/api/push/send", s.method(http.MethodPost, s.requirePermission("Push:Manage", s.adminPushSend)))
	s.mux.HandleFunc("/api/push/audience-count", s.method(http.MethodGet, s.requirePermission("Push:Manage", s.adminPushAudienceCount)))
	s.mux.HandleFunc("/api/push/daily-quiz/stats", s.method(http.MethodGet, s.requirePermission("ProfileCalibration:DailyQuiz:Manage", s.adminDailyQuizPushStats)))
	s.mux.HandleFunc("/api/push/daily-quiz/records", s.method(http.MethodGet, s.requirePermission("ProfileCalibration:DailyQuiz:Manage", s.adminDailyQuizPushRecords)))
	s.mux.HandleFunc("/api/daily-quiz/admin/sets/today", s.method(http.MethodGet, s.requirePermission("ProfileCalibration:DailyQuiz:Manage", s.adminDailyQuizSetToday)))
	s.mux.HandleFunc("/api/daily-quiz/admin/sets", s.method(http.MethodGet, s.requirePermission("ProfileCalibration:DailyQuiz:Manage", s.adminDailyQuizSet)))
	s.mux.HandleFunc("/api/daily-quiz/admin/sets/generate", s.method(http.MethodPost, s.requirePermission("ProfileCalibration:DailyQuiz:Manage", s.adminDailyQuizSetGenerate)))
	s.mux.HandleFunc("/api/daily-quiz/admin/sets/", s.requirePermission("ProfileCalibration:DailyQuiz:Manage", s.adminDailyQuizSetRouter))
	s.mux.HandleFunc("/api/signups/list", s.method(http.MethodGet, s.requirePermission("Customer:Signup:List", s.signupList)))
	s.mux.HandleFunc("/api/signups/detail", s.method(http.MethodGet, s.requirePermission("Customer:Signup:List", s.signupDetail)))
	s.mux.HandleFunc("/api/signups/follow", s.method(http.MethodPut, s.requirePermission("Customer:Signup:List", s.signupFollow)))
	s.mux.HandleFunc("/api/signups/events", s.method(http.MethodGet, s.requirePermission("Customer:Signup:List", s.signupEvents)))
	s.mux.HandleFunc("/api/voice/profiles/list", s.method(http.MethodGet, s.requireAnyPermission([]string{"Voice:Profile:Manage", "Voice:Test:Manage"}, s.voiceProfiles)))
	s.mux.HandleFunc("/api/voice/profiles", s.method(http.MethodPost, s.requirePermission("Voice:Profile:Manage", s.createVoiceProfile)))
	s.mux.HandleFunc("/api/voice/profiles/", s.requirePermission("Voice:Profile:Manage", s.voiceProfileByID))
	s.mux.HandleFunc("/api/voice/options", s.method(http.MethodGet, s.requireAnyPermission([]string{"Voice:Profile:Manage", "Voice:Test:Manage", "Voice:Content:Manage", "Reading:Article:Manage"}, s.voiceOptions)))
	s.mux.HandleFunc("/api/voice/generate", s.method(http.MethodPost, s.requirePermission("Voice:Test:Manage", s.generateVoice)))
	s.mux.HandleFunc("/api/voice/generations/list", s.method(http.MethodGet, s.requirePermission("Voice:Test:Manage", s.voiceGenerations)))
	s.mux.HandleFunc("/api/voice/content/generate", s.method(http.MethodPost, s.requirePermission("Voice:Content:Manage", s.generateVoiceContent)))
	s.mux.HandleFunc("/api/voice/content/list", s.method(http.MethodGet, s.requirePermission("Voice:Content:Manage", s.voiceContentJobs)))
	s.mux.HandleFunc("/api/video/generate", s.method(http.MethodPost, s.requirePermission("Video:Generate:Manage", s.generateVideo)))
	s.mux.HandleFunc("/api/video/generations/list", s.method(http.MethodGet, s.requirePermission("Video:Generate:Manage", s.videoGenerations)))
	s.mux.HandleFunc("/api/video/generations/overview", s.method(http.MethodGet, s.requirePermission("Video:Generation:Overview", s.videoGenerationsOverview)))
	s.mux.HandleFunc("/api/video/generations/", s.requirePermission("Video:Generate:Manage", s.videoGenerationByID))
	s.mux.HandleFunc("/api/video/analysis", s.method(http.MethodPost, s.requirePermission("Video:Analysis:Manage", s.createVideoAnalysis)))
	s.mux.HandleFunc("/api/video/analysis/list", s.method(http.MethodGet, s.requireAnyPermission([]string{"Video:Analysis:Manage", "Video:Storyboard:Manage"}, s.videoAnalysisList)))
	s.mux.HandleFunc("/api/video/analysis/", s.requirePermission("Video:Analysis:Manage", s.videoAnalysisByID))
	s.mux.HandleFunc("/api/video/storyboards", s.requirePermission("Video:Storyboard:Manage", s.videoStoryboards))
	s.mux.HandleFunc("/api/video/storyboards/list", s.method(http.MethodGet, s.requirePermission("Video:Storyboard:Manage", s.videoStoryboardList)))
	s.mux.HandleFunc("/api/video/storyboards/", s.requirePermission("Video:Storyboard:Manage", s.videoStoryboardByID))
	s.mux.HandleFunc("/api/video/assets/list", s.method(http.MethodGet, s.requirePermission("Video:Asset:Manage", s.videoAssetList)))
	s.mux.HandleFunc("/api/video/assets/generate-image", s.method(http.MethodPost, s.requirePermission("Video:Asset:Manage", s.generateImageAsset)))
	s.mux.HandleFunc("/api/video/assets/polish-prompt", s.method(http.MethodPost, s.requireAnyPermission([]string{"Video:Asset:Manage", "Video:Generate:Manage"}, s.polishPrompt)))
	s.mux.HandleFunc("/api/video/assets", s.method(http.MethodPost, s.requirePermission("Video:Asset:Manage", s.createVideoAsset)))
	s.mux.HandleFunc("/api/video/assets/", s.requirePermission("Video:Asset:Manage", s.videoAssetByID))
	// 视频项目工作台（降低抽卡率的项目制工作流）
	s.mux.HandleFunc("/api/video/projects/list", s.method(http.MethodGet, s.requirePermission("Video:Project:Manage", s.videoProjectList)))
	s.mux.HandleFunc("/api/video/projects", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.createVideoProject)))
	s.mux.HandleFunc("/api/video/projects/", s.requirePermission("Video:Project:Manage", s.videoProjectByID))
	s.mux.HandleFunc("/api/video/projects-characters/", s.requirePermission("Video:Project:Manage", s.videoProjectCharacters))
	s.mux.HandleFunc("/api/video/projects-scenes/", s.requirePermission("Video:Project:Manage", s.videoProjectScenes))
	s.mux.HandleFunc("/api/video/projects-shots/list/", s.requirePermission("Video:Project:Manage", s.videoProjectShotsList))
	s.mux.HandleFunc("/api/video/projects-shots/", s.requirePermission("Video:Project:Manage", s.videoProjectShots))
	s.mux.HandleFunc("/api/video/shots/", s.requirePermission("Video:Project:Manage", s.videoShotByID))
	s.mux.HandleFunc("/api/video/shots-assets/list/", s.method(http.MethodGet, s.requirePermission("Video:Project:Manage", s.videoShotAssetsList)))
	s.mux.HandleFunc("/api/video/shots-assets/", s.requirePermission("Video:Project:Manage", s.videoShotAssets))
	s.mux.HandleFunc("/api/video/shots-video-versions/list/", s.method(http.MethodGet, s.requirePermission("Video:Project:Manage", s.videoShotVideoVersionsList)))
	s.mux.HandleFunc("/api/video/shots-video-versions/detail/", s.method(http.MethodGet, s.requirePermission("Video:Project:Manage", s.videoShotVideoVersionDetail)))
	s.mux.HandleFunc("/api/video/shots-video-versions/set/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.setShotVideoVersion)))
	s.mux.HandleFunc("/api/video/shots-video-versions/viewed/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.markShotVideoVersionViewed)))
	s.mux.HandleFunc("/api/video/shots-video-versions/backup/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.setShotVideoVersionBackup)))
	s.mux.HandleFunc("/api/video/shots-video-versions/remove-subtitle/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.removeShotVideoVersionSubtitle)))
	s.mux.HandleFunc("/api/video/shots-video-versions/upscale/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.upscaleShotVideoVersion)))
	s.mux.HandleFunc("/api/video/shots-video-versions/refresh/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.refreshShotVideoVersions)))
	s.mux.HandleFunc("/api/video/shots-video-versions/copy/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.copyShotVideoVersion)))
	s.mux.HandleFunc("/api/video/shots-video-versions/extract-frame/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.extractShotVideoFrame)))
	s.mux.HandleFunc("/api/video/shots-video-versions/", s.requirePermission("Video:Project:Manage", s.videoShotVideoVersions))
	s.mux.HandleFunc("/api/video/shots-preview/", s.method(http.MethodGet, s.requirePermission("Video:Project:Manage", s.videoShotPreview)))
	s.mux.HandleFunc("/api/video/shots-generate/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.generateVideoShot)))
	// 批量生成和视频合成
	s.mux.HandleFunc("/api/video/projects-batch-generate/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.batchGenerateShots)))
	s.mux.HandleFunc("/api/video/projects-batch-progress/", s.method(http.MethodGet, s.requirePermission("Video:Project:Manage", s.batchGenerateProgress)))
	s.mux.HandleFunc("/api/video/projects-compose/", s.method(http.MethodPost, s.requirePermission("Video:Project:Manage", s.composeProjectVideo)))
	s.mux.HandleFunc("/api/video/projects-compose-status/", s.method(http.MethodGet, s.requirePermission("Video:Project:Manage", s.composeStatus)))
	s.mux.HandleFunc("/api/rag/documents", s.requirePermission("RAG:Knowledge:Manage", s.ragDocuments))
	s.mux.HandleFunc("/api/rag/documents/", s.requirePermission("RAG:Knowledge:Manage", s.ragDocumentByID))
	s.mux.HandleFunc("/api/articles", s.requirePermission("Reading:Article:Manage", s.adminArticles))
	s.mux.HandleFunc("/api/articles/", s.requirePermission("Reading:Article:Manage", s.adminArticleByID))
	// 测评题库管理 + 命运卡片查看（后台）
	s.mux.HandleFunc("/api/quiz/questions", s.requirePermission("Website:Write", s.adminQuizQuestions))
	s.mux.HandleFunc("/api/quiz/questions/", s.requirePermission("Website:Write", s.adminQuizQuestionByID))
	s.mux.HandleFunc("/api/quiz/cards", s.method(http.MethodGet, s.requireAnyPermission([]string{"Website:Read", "Customer:UserInsights:List"}, s.adminQuizCards)))
	s.mux.HandleFunc("/api/mind-groups", s.requirePermission("Website:Write", s.adminMindGroups))
	s.mux.HandleFunc("/api/mind-quotes", s.requirePermission("Website:Write", s.adminMindQuotes))
	s.mux.HandleFunc("/api/mind-quotes/", s.requirePermission("Website:Write", s.adminMindQuoteByID))
	s.mux.HandleFunc("/api/reading/settings", s.requirePermission("Reading:Article:Manage", s.readingSettings))
	s.mux.HandleFunc("/api/rag/reindex", s.method(http.MethodPost, s.requirePermission("RAG:Knowledge:Manage", s.ragReindex)))
	s.mux.HandleFunc("/api/audit-logs/list", s.method(http.MethodGet, s.requirePermission("System:Audit:List", s.adminAuditLogs)))
	s.mux.HandleFunc("/api/system/user/list", s.method(http.MethodGet, s.requirePermission("System:User:List", s.system.HandleUsers)))
	s.mux.HandleFunc("/api/system/user", s.requireMethodPermission(map[string]string{
		http.MethodDelete: "System:User:Delete",
		http.MethodGet:    "System:User:List",
		http.MethodPost:   "System:User:Create",
	}, s.system.HandleUsers))
	s.mux.HandleFunc("/api/system/user/", s.requireMethodPermission(map[string]string{
		http.MethodDelete: "System:User:Delete",
		http.MethodPut:    "System:User:Update",
	}, s.system.HandleUserByID))
	s.mux.HandleFunc("/api/app-users/list", s.method(http.MethodGet, s.requirePermission("Customer:App:List", s.appUsers.HandleAppUsers)))
	s.mux.HandleFunc("/api/app-orders/list", s.method(http.MethodGet, s.requirePermission("Customer:AppOrders:List", s.adminAppOrders)))
	s.mux.HandleFunc("/api/app-orders/", s.method(http.MethodPost, s.requirePermission("Customer:AppOrders:Write", s.adminAppOrderGrant)))
	s.mux.HandleFunc("/api/app-chat/messages/list", s.method(http.MethodGet, s.requirePermission("Customer:AppChat:List", s.adminAppChatMessages)))
	s.mux.HandleFunc("/api/app-memories/list", s.method(http.MethodGet, s.requirePermission("Customer:AppMemory:List", s.adminAppMemories)))
	s.mux.HandleFunc("/api/app-memories/", s.method(http.MethodPut, s.requirePermission("Customer:AppMemory:Write", s.adminAppMemoryStatus)))
	s.mux.HandleFunc("/api/app-users/insights", s.method(http.MethodGet, s.requirePermission("Customer:UserInsights:List", s.appUsers.HandleAppUserInsights)))
	s.mux.HandleFunc("/api/app-users/", s.adminAppUserByID)
	s.mux.HandleFunc("/api/system/role/list", s.method(http.MethodGet, s.requirePermission("System:Role:List", s.system.HandleRoles)))
	s.mux.HandleFunc("/api/system/role", s.requireMethodPermission(map[string]string{
		http.MethodDelete: "System:Role:Delete",
		http.MethodGet:    "System:Role:List",
		http.MethodPost:   "System:Role:Create",
	}, s.system.HandleRoles))
	s.mux.HandleFunc("/api/system/role/", s.requireMethodPermission(map[string]string{
		http.MethodDelete: "System:Role:Delete",
		http.MethodPut:    "System:Role:Update",
	}, s.system.HandleRoleByID))
	s.mux.HandleFunc("/api/system/menu/list", s.method(http.MethodGet, s.requirePermission("System:Menu:List", s.system.HandleMenus)))
	s.mux.HandleFunc("/api/system/menu/name-exists", s.method(http.MethodGet, s.requirePermission("System:Menu:List", s.system.HandleMenuNameExists)))
	s.mux.HandleFunc("/api/system/menu/path-exists", s.method(http.MethodGet, s.requirePermission("System:Menu:List", s.system.HandleMenuPathExists)))
	s.mux.HandleFunc("/api/system/menu", s.requireMethodPermission(map[string]string{
		http.MethodDelete: "System:Menu:Delete",
		http.MethodGet:    "System:Menu:List",
		http.MethodPost:   "System:Menu:Create",
	}, s.system.HandleMenus))
	s.mux.HandleFunc("/api/system/menu/", s.requireMethodPermission(map[string]string{
		http.MethodDelete: "System:Menu:Delete",
		http.MethodPut:    "System:Menu:Update",
	}, s.system.HandleMenuByID))
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, map[string]string{"service": "nine-xing-vben-go-server", "status": "ok"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	var body struct {
		Password string `json:"password"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "BadRequestException")
		return
	}
	if !s.allowLoginAttempt(body.Username, s.clientIP(r), time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "TooManyRequestsException")
		return
	}

	id, nickname, roleCodes, ok, err := s.system.AuthUser(r.Context(), body.Username, body.Password)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		httpx.Fail(w, http.StatusForbidden, "Username or password is incorrect.")
		return
	}

	homePath, herr := s.system.DefaultHomePathForUser(r.Context(), id, roleCodes)
	if herr != nil {
		httpx.Fail(w, http.StatusInternalServerError, herr.Error())
		return
	}

	user := auth.UserInfo{
		HomePath:  homePath,
		ID:        id,
		RealName:  nickname,
		Roles:     roleCodes,
		TokenKind: auth.TokenKindBackend,
		UserID:    fmt.Sprintf("%d", id),
		Username:  body.Username,
	}
	if version, verr := s.system.TokenVersion(r.Context(), id); verr == nil {
		user.TokenVersion = version
	}
	token, err := auth.Sign(user, s.env.JWTSecret)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	payload := map[string]any{
		"accessToken": token,
		"homePath":    user.HomePath,
		"id":          user.ID,
		"realName":    user.RealName,
		"roles":       user.Roles,
		"userId":      user.UserID,
		"username":    user.Username,
	}
	httpx.OK(w, payload)
}

func (s *Server) allowLoginAttempt(username, ip string, now time.Time) bool {
	if s == nil || s.loginLimiter == nil {
		return true
	}
	key := strings.ToLower(strings.TrimSpace(username)) + "|" + strings.TrimSpace(ip)
	if s.loginDBLimiter != nil && s.db != nil {
		if allowed, err := s.loginDBLimiter.allow(context.Background(), key, now); err == nil {
			return allowed
		}
	}
	return s.loginLimiter.Allow(key, now)
}

func (s *Server) logout(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, true)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	user := userFromRequest(r)
	token, err := auth.Sign(user, s.env.JWTSecret)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, token)
}

func (s *Server) userInfo(w http.ResponseWriter, r *http.Request) {
	user := userFromRequest(r)
	profile, err := s.system.CurrentUserProfile(r.Context(), user.ID, user.HomePath)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, profile)
}

func (s *Server) updateUserProfile(w http.ResponseWriter, r *http.Request) {
	user := userFromRequest(r)
	var body system.ProfileUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "BadRequestException")
		return
	}
	profile, err := s.system.UpdateCurrentUserProfile(r.Context(), user.ID, body, user.HomePath)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, profile)
}

func (s *Server) codes(w http.ResponseWriter, r *http.Request) {
	user := userFromRequest(r)
	codes, err := s.system.AuthCodesForUser(r.Context(), user.ID, user.Roles)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, codes)
}

func (s *Server) menus(w http.ResponseWriter, r *http.Request) {
	user := userFromRequest(r)
	menus, err := s.system.MenusForUser(r.Context(), user.ID, user.Roles)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, menus)
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	uploader, err := s.objectUploader()
	if err != nil {
		httpx.Fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	maxBytes := s.env.UploadMaxBytes
	if maxBytes <= 0 {
		maxBytes = 20 * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		if isTooLarge(err) {
			httpx.Fail(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds %d bytes", maxBytes))
			return
		}
		httpx.Fail(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	if header.Size > maxBytes {
		httpx.Fail(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds %d bytes", maxBytes))
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(header.Filename))
	}
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "read upload file failed")
		return
	}
	if int64(len(content)) > maxBytes {
		httpx.Fail(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds %d bytes", maxBytes))
		return
	}
	if uploadDirAllowsSelfService(r.URL.Query().Get("dir")) && !isImageUpload(contentType, content) {
		httpx.Fail(w, http.StatusBadRequest, "user avatar upload must be an image")
		return
	}

	result, err := uploader.Upload(r.Context(), storage.UploadInput{
		ContentType: contentType,
		Dir:         r.URL.Query().Get("dir"),
		Filename:    header.Filename,
		Reader:      bytes.NewReader(content),
		Size:        int64(len(content)),
	})
	if err != nil {
		httpx.Fail(w, http.StatusBadGateway, err.Error())
		return
	}
	if s.db != nil {
		objectKey := result.Key
		objectURL := result.URL
		asset, err := s.uploads.Create(r.Context(), uploadasset.CreateInput{
			ContentType: result.ContentType,
			Data:        content,
			Dir:         r.URL.Query().Get("dir"),
			Name:        result.Name,
			ObjectKey:   objectKey,
			ObjectURL:   objectURL,
			Size:        int64(len(content)),
		})
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		result.AssetID = asset.ID
		result.AssetKey = asset.Key
		result.Key = asset.Key
		result.ObjectKey = objectKey
		result.ObjectURL = objectURL
		result.URL = "/api/upload-assets/" + fmt.Sprintf("%d", asset.ID)
	}
	httpx.OK(w, result)
}

func (s *Server) uploadAsset(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, "/api/upload-assets/")
	id, err := strconv.ParseInt(strings.Trim(idText, "/"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	asset, err := s.uploads.Find(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeUploadAsset(w, asset)
}

func (s *Server) uploadedFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.StripPrefix("/api/uploads/", http.FileServer(noDirListingFileSystem{fs: http.Dir(s.env.UploadDir)})).ServeHTTP(w, r)
}

func writeUploadAsset(w http.ResponseWriter, asset uploadasset.Asset) {
	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(asset.Data)))
	_, _ = w.Write(asset.Data)
}

func (s *Server) objectUploader() (storage.ObjectUploader, error) {
	if s.uploader != nil {
		return s.uploader, nil
	}
	if s.env.OSS.AccessKeyID == "" && s.env.OSS.AccessKeySecret == "" && s.env.OSS.Bucket == "" && s.env.OSS.Region == "" {
		s.uploader = storage.NewLocalUploader(s.env.UploadDir, "/api/uploads")
		return s.uploader, nil
	}
	uploader, err := storage.NewOSSUploader(s.env.OSS)
	if err != nil {
		return nil, err
	}
	s.uploader = uploader
	return s.uploader, nil
}

type noDirListingFileSystem struct {
	fs http.FileSystem
}

func (f noDirListingFileSystem) Open(name string) (http.File, error) {
	file, err := f.fs.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if stat.IsDir() {
		_ = file.Close()
		return nil, os.ErrNotExist
	}
	return file, nil
}

func isTooLarge(err error) bool {
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError) || strings.Contains(strings.ToLower(err.Error()), "too large")
}

func (s *Server) siteConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}

	user, ok := s.authorize(w, r)
	if !ok {
		return
	}
	r = r.WithContext(withUser(r.Context(), user))

	switch r.Method {
	case http.MethodGet:
		allowed, err := s.hasAnyPermission(r.Context(), user, "Website:Read", "Website:Write")
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !allowed {
			httpx.Fail(w, http.StatusForbidden, "Forbidden")
			return
		}
		config, err := siteconfig.ReadStore(r.Context(), s.db, s.env.SiteConfig)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, config)
	case http.MethodPut:
		allowed, err := s.hasAnyPermission(r.Context(), user, "Website:Write")
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !allowed {
			httpx.Fail(w, http.StatusForbidden, "Forbidden")
			return
		}
		var config siteconfig.SiteConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		if err := siteconfig.WriteStore(r.Context(), s.db, s.env.SiteConfig, config); err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		// 配置已落盘，异步触发官网重新构建+发布（非阻塞）。
		s.builder.Trigger()
		httpx.OK(w, config)
	}
}

func (s *Server) siteBuildStatus(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, s.builder.Status())
}

// publicSiteConfig 给官网运行时拉取站点配置，公开只读、无需登录。
func (s *Server) publicSiteConfig(w http.ResponseWriter, r *http.Request) {
	config, err := siteconfig.ReadStore(r.Context(), s.db, s.env.SiteConfig)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	rewritePublicSiteConfigAssets(&config)
	httpx.OK(w, config)
}

func (s *Server) publicSignup(w http.ResponseWriter, r *http.Request) {
	if !s.allowPublicWrite(w, r, s.publicSignupIPLimiter) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var body signup.LeadInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	lead, err := s.signups.Create(r.Context(), body, r)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.broadcastSignup(lead)
	httpx.OK(w, lead)
}

func (s *Server) publicSiteVisit(w http.ResponseWriter, r *http.Request) {
	if !s.allowPublicWrite(w, r, s.publicAnalyticsIPLimiter) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var body analytics.VisitInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := s.analytics.TrackVisit(r.Context(), body, r); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, true)
}

func (s *Server) publicGameResult(w http.ResponseWriter, r *http.Request) {
	if !s.allowPublicWrite(w, r, s.publicGameIPLimiter) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body engagement.GameResultInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	result, err := s.engagement.TrackGameResult(r.Context(), body, r)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) allowPublicWrite(w http.ResponseWriter, r *http.Request, limiter *strRateLimiter) bool {
	if limiter == nil {
		return true
	}
	if limiter.Allow(s.clientIP(r), time.Now()) {
		return true
	}
	httpx.Fail(w, http.StatusTooManyRequests, "TooManyRequestsException")
	return false
}

func (s *Server) analyticsOverview(w http.ResponseWriter, r *http.Request) {
	result, err := s.analytics.Overview(r.Context(), r.URL.Query())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) gameOverview(w http.ResponseWriter, r *http.Request) {
	result, err := s.engagement.GameOverview(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) messagesList(w http.ResponseWriter, r *http.Request) {
	result, err := s.engagement.Messages(r.Context(), r.URL.Query())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) markMessages(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs  []string `json:"ids"`
		Read bool     `json:"read"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := s.engagement.MarkMessages(r.Context(), body.IDs, body.Read); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, true)
}

func (s *Server) signupList(w http.ResponseWriter, r *http.Request) {
	result, err := s.signups.List(r.Context(), queryMap(r))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) signupDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	result, err := s.signups.Detail(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "signup not found")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) signupFollow(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	var body signup.FollowInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	user := userFromRequest(r)
	operator := strings.TrimSpace(user.RealName)
	if operator == "" {
		operator = strings.TrimSpace(user.Username)
	}
	lead, err := s.signups.Follow(r.Context(), id, body, operator)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "signup not found")
			return
		}
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, lead)
}

func (s *Server) signupEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Fail(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan signup.Lead, 8)
	s.addSignupSubscriber(ch)
	defer s.removeSignupSubscriber(ch)

	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": ping\n\n")
			flusher.Flush()
		case lead := <-ch:
			payload, err := json.Marshal(lead)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: signup\ndata: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func (s *Server) voiceProfiles(w http.ResponseWriter, r *http.Request) {
	result, err := s.voices.ListProfiles(r.Context(), r.URL.Query())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) createVoiceProfile(w http.ResponseWriter, r *http.Request) {
	var body voice.CreateProfileInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	result, err := s.voices.CreateProfile(r.Context(), body)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) voiceProfileByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/voice/profiles/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	switch r.Method {
	case http.MethodPost:
		result, err := s.voices.CloneProfile(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodDelete:
		if err := s.voices.DeleteProfile(r.Context(), id); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, true)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// videoAssetList 资产库列表：按类型/关键字筛选可复用的视频生成素材。
func (s *Server) videoAssetList(w http.ResponseWriter, r *http.Request) {
	result, err := s.videoAssets.List(r.Context(), r.URL.Query())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) createVideoAsset(w http.ResponseWriter, r *http.Request) {
	var body videoasset.CreateInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	result, err := s.videoAssets.Create(r.Context(), body)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, result)
}

// generateImageAsset 通过 gpt-image-2 文生图网关生成图片，并将结果登记为资产库素材。
func (s *Server) generateImageAsset(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body struct {
		Prompt string `json:"prompt"`
		Size   string `json:"size"`
		Model  string `json:"model"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Remark string `json:"remark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	gen, err := s.imageStore().Generate(r.Context(), image.GenerateInput{
		Model:  body.Model,
		Prompt: body.Prompt,
		Size:   body.Size,
	})
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = strings.TrimSpace(body.Prompt)
	}
	result, err := s.videoAssets.Create(r.Context(), videoasset.CreateInput{
		AssetID:  fmt.Sprint(gen.AssetID),
		Name:     name,
		Type:     body.Type,
		Remark:   body.Remark,
		URL:      gen.URL,
		CoverURL: firstNonEmpty(gen.PreviewURL, gen.URL),
	})
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, result)
}

// polishPrompt 复用当前兼容协议对话模型把用户给出的方向或草稿润色成高质量提示词。
func (s *Server) polishPrompt(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body struct {
		Prompt string `json:"prompt"`
		Kind   string `json:"kind"`
		Type   string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	kind := strings.TrimSpace(body.Kind)
	if kind == "" {
		kind = strings.TrimSpace(body.Type)
	}
	gen, ok := s.generator().(interface {
		PolishPrompt(context.Context, string, string) (string, error)
	})
	if !ok || gen == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "AI 润色未启用，请先在模型配置中配置对话模型")
		return
	}
	polished, err := gen.PolishPrompt(r.Context(), body.Prompt, kind)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, map[string]string{"prompt": polished})
}

func (s *Server) videoAssetByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/assets/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := s.videoAssets.Delete(r.Context(), id); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, true)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) voiceOptions(w http.ResponseWriter, r *http.Request) {
	result, err := s.voices.VoiceOptions(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) generateVoice(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var body voice.GenerateInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	result, err := s.voices.Generate(r.Context(), body)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) generateVoiceContent(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
	var body voice.ContentGenerateInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	result, err := s.voices.GenerateContent(r.Context(), body)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) voiceGenerations(w http.ResponseWriter, r *http.Request) {
	result, err := s.voices.ListGenerations(r.Context(), r.URL.Query())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) voiceContentJobs(w http.ResponseWriter, r *http.Request) {
	result, err := s.voices.ListContentJobs(r.Context(), r.URL.Query())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) generateVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body video.GenerateInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := s.normalizeVideoReferenceURLs(r.Context(), &body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.videoStore().Generate(r.Context(), body)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, result)
}

// publicBaseURL 返回后端对外可达的根地址（无尾斜杠）。
// 外部视频网关只允许使用显式 PUBLIC_BASE_URL，避免请求 Host/X-Forwarded-Host
// 影响模型网关拉取目标；为空时返回空字符串并让后续公网 URL 校验 fail-closed。
func (s *Server) publicBaseURL(_ *http.Request) string {
	if base := strings.TrimRight(strings.TrimSpace(s.env.PublicBaseURL), "/"); base != "" {
		return base
	}
	return ""
}

// absoluteURL 把以 / 开头的相对地址补全为 base 下的绝对地址；
// 已是 http(s):// 或 data: 的地址原样返回，base 为空时也原样返回。
func absoluteURL(base, raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" || base == "" {
		return u
	}
	lower := strings.ToLower(u)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "data:") {
		return u
	}
	if strings.HasPrefix(u, "/") {
		return base + u
	}
	return u
}

func absoluteURLs(base string, raws []string) []string {
	if len(raws) == 0 {
		return raws
	}
	out := make([]string, 0, len(raws))
	for _, raw := range raws {
		out = append(out, absoluteURL(base, raw))
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Server) normalizeVideoReferenceURLs(ctx context.Context, body *video.GenerateInput) error {
	if body == nil {
		return nil
	}
	imageURL, err := s.videoReferenceURL(ctx, "参考图片", body.ImageURL)
	if err != nil {
		return err
	}
	images, err := s.videoReferenceURLs(ctx, "参考图片", body.Images)
	if err != nil {
		return err
	}
	videos, err := s.videoReferenceURLs(ctx, "参考视频", body.Videos)
	if err != nil {
		return err
	}
	audios, err := s.videoReferenceURLs(ctx, "参考音频", body.Audios)
	if err != nil {
		return err
	}
	body.ImageURL = imageURL
	body.Images = images
	body.Videos = videos
	body.Audios = audios
	return nil
}

func (s *Server) videoReferenceURLs(ctx context.Context, label string, raws []string) ([]string, error) {
	if len(raws) == 0 {
		return raws, nil
	}
	out := make([]string, 0, len(raws))
	for _, raw := range raws {
		u, err := s.videoReferenceURL(ctx, label, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

func (s *Server) videoReferenceURL(ctx context.Context, label string, raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", nil
	}
	if assetID, ok := uploadAssetIDFromURL(u); ok {
		asset, err := s.uploads.Find(ctx, assetID)
		if err != nil {
			return "", fmt.Errorf("%s对应的上传资产不存在，请重新上传或重新选择: %s", label, u)
		}
		objectURL := strings.TrimSpace(asset.ObjectURL)
		if isPublicHTTPURL(objectURL) {
			return objectURL, nil
		}
		if backfilledURL, err := backfillUploadAssetObjectURL(ctx, s.uploads, s.uploader, assetID, asset); err == nil {
			return backfilledURL, nil
		}
		return "", fmt.Errorf("%s需要文件桶公网 http(s) 地址，该资产没有可供外部视频网关访问的 objectUrl，请配置 OSS_PUBLIC_URL/文件桶公网访问后重新上传: %s", label, u)
	}
	if !isPublicHTTPURL(u) {
		return "", fmt.Errorf("%s需要文件桶公网 http(s) 地址，请使用上传返回的 objectUrl；当前地址外部视频网关无法访问: %s", label, u)
	}
	return u, nil
}

type uploadAssetObjectUpdater interface {
	UpdateObjectMetadata(ctx context.Context, id int64, objectKey string, objectURL string) error
}

func backfillUploadAssetObjectURL(ctx context.Context, updater uploadAssetObjectUpdater, uploader storage.ObjectUploader, id int64, asset uploadasset.Asset) (string, error) {
	if updater == nil || uploader == nil {
		return "", fmt.Errorf("object storage uploader is not configured")
	}
	if objectURL := strings.TrimSpace(asset.ObjectURL); isPublicHTTPURL(objectURL) {
		return objectURL, nil
	}
	if len(asset.Data) == 0 {
		return "", fmt.Errorf("upload asset has no data to reupload")
	}
	filename := strings.TrimSpace(asset.Name)
	if filename == "" {
		filename = fmt.Sprintf("upload-asset-%d", id)
	}
	result, err := uploader.Upload(ctx, storage.UploadInput{
		ContentType: asset.ContentType,
		Dir:         "video/reference",
		Filename:    filename,
		Reader:      bytes.NewReader(asset.Data),
		Size:        int64(len(asset.Data)),
	})
	if err != nil {
		return "", err
	}
	objectURL := strings.TrimSpace(result.URL)
	if !isPublicHTTPURL(objectURL) {
		return "", fmt.Errorf("object storage did not return public object url")
	}
	if err := updater.UpdateObjectMetadata(ctx, id, result.Key, objectURL); err != nil {
		return "", err
	}
	return objectURL, nil
}

func uploadAssetIDFromURL(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return 0, false
	}
	if u.IsAbs() || u.Host != "" {
		return 0, false
	}
	path := u.Path
	if path == "" && strings.HasPrefix(raw, "/") {
		path = raw
	}
	const prefix = "/api/upload-assets/"
	if !strings.HasPrefix(path, prefix) {
		return 0, false
	}
	idText := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func uploadDirAllowsSelfService(dir string) bool {
	return strings.Trim(strings.TrimSpace(dir), "/") == "user-avatars"
}

func isImageUpload(contentType string, content []byte) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/") {
		return true
	}
	detected := http.DetectContentType(content)
	return strings.HasPrefix(strings.ToLower(detected), "image/")
}

func (s *Server) videoGenerations(w http.ResponseWriter, r *http.Request) {
	result, err := s.videoStore().ListGenerations(r.Context(), r.URL.Query())
	if err != nil {
		log.Printf("video generations list failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) videoGenerationsOverview(w http.ResponseWriter, r *http.Request) {
	result, err := s.videoStore().Overview(r.Context())
	if err != nil {
		log.Printf("video generations overview failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// videoGenerationByID 处理单条视频任务：GET 返回详情，POST 触发刷新（轮询网关状态并在完成时落库）。
func (s *Server) videoGenerationByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/generations/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := s.videoStore().Generation(r.Context(), id)
		if err != nil {
			log.Printf("video generation get failed id=%s: %v", id, err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodPost:
		result, err := s.videoStore().Refresh(r.Context(), id)
		if err != nil {
			log.Printf("video generation refresh failed id=%s: %v", id, err)
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, result)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) createVideoAnalysis(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body videoanalysis.CreateInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	base := s.publicBaseURL(r)
	body.VideoURL = absoluteURL(base, body.VideoURL)
	if err := s.ensureAnalysisVideoObjectURL(r.Context(), &body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	job, err := s.videoAnalysis.Create(r.Context(), body)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.startVideoAnalysisTask(job.ID)
	httpx.OK(w, job)
}

func (s *Server) videoAnalysisList(w http.ResponseWriter, r *http.Request) {
	result, err := s.videoAnalysis.List(r.Context(), r.URL.Query())
	if err != nil {
		log.Printf("video analysis list failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) ensureAnalysisVideoObjectURL(ctx context.Context, body *videoanalysis.CreateInput) error {
	if body == nil {
		return fmt.Errorf("请先上传视频")
	}
	assetID := strings.TrimSpace(body.VideoAssetID)
	if assetID != "" {
		id, err := strconv.ParseInt(assetID, 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("视频资产标识无效")
		}
		asset, err := s.uploads.Find(ctx, id)
		if err != nil {
			return fmt.Errorf("未找到上传的视频资产")
		}
		if strings.TrimSpace(asset.ObjectURL) == "" {
			return fmt.Errorf("该视频没有文件桶公网地址，请先配置 OSS_PUBLIC_URL/文件桶公网访问后重新上传")
		}
		body.VideoURL = strings.TrimSpace(asset.ObjectURL)
		if strings.TrimSpace(body.VideoName) == "" {
			body.VideoName = asset.Name
		}
	}
	if !isPublicHTTPURL(body.VideoURL) {
		return fmt.Errorf("视频分析需要文件桶公网 http(s) 地址，当前地址模型无法访问，请重新上传到文件桶")
	}
	return nil
}

func (s *Server) analysisVideoURL(ctx context.Context, asset uploadasset.Asset) string {
	objectURL := strings.TrimSpace(asset.ObjectURL)
	objectKey := strings.TrimSpace(asset.ObjectKey)
	if objectKey == "" {
		return objectURL
	}
	uploader, err := s.objectUploader()
	if err != nil {
		log.Printf("video analysis object signer unavailable: %v", err)
		return objectURL
	}
	signer, ok := uploader.(storage.ObjectSigner)
	if !ok {
		return objectURL
	}
	signedURL, err := signer.PresignGetURL(ctx, objectKey, 30*time.Minute)
	if err != nil {
		log.Printf("video analysis presign failed object_key=%s: %v", objectKey, err)
		return objectURL
	}
	if strings.TrimSpace(signedURL) == "" {
		return objectURL
	}
	return signedURL
}

func isPublicHTTPURL(raw string) bool {
	return netguard.IsPublicHTTPURL(raw)
}

func (s *Server) videoAnalysisByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/analysis/"), "/")
	id, action, _ := strings.Cut(rest, "/")
	if id == "" || action != "retry" {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	job, err := s.videoAnalysis.Retry(r.Context(), id)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	body := videoanalysis.CreateInput{
		VideoAssetID: job.VideoAssetID,
		VideoName:    job.VideoName,
		VideoURL:     job.VideoURL,
	}
	if err := s.ensureAnalysisVideoObjectURL(r.Context(), &body); err != nil {
		_ = s.videoAnalysis.Fail(r.Context(), id, err.Error())
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.VideoURL) != strings.TrimSpace(job.VideoURL) {
		if err := s.videoAnalysis.UpdateVideoURL(r.Context(), id, body.VideoURL); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.startVideoAnalysisTask(job.ID)
	httpx.OK(w, job)
}

func tryAcquireVideoAsyncSlot(slots chan struct{}) bool {
	if slots == nil {
		return true
	}
	select {
	case slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseVideoAsyncSlot(slots chan struct{}) {
	if slots == nil {
		return
	}
	select {
	case <-slots:
	default:
	}
}

func (s *Server) startVideoAnalysisTask(id string) {
	if !tryAcquireVideoAsyncSlot(s.videoAnalysisSlots) {
		s.failVideoAnalysisTask(id, "视频分析任务繁忙，请稍后重试")
		return
	}
	go func() {
		defer releaseVideoAsyncSlot(s.videoAnalysisSlots)
		s.runVideoAnalysis(id)
	}()
}

func (s *Server) runVideoAnalysis(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("video analysis task panic id=%s: %v", id, recovered)
			s.failVideoAnalysisTask(id, fmt.Sprintf("视频分析任务异常：%v", recovered))
		}
	}()
	if err := s.videoAnalysis.MarkRunning(ctx, id); err != nil {
		log.Printf("video analysis mark running failed id=%s: %v", id, err)
		return
	}
	job, err := s.videoAnalysis.Find(ctx, id)
	if err != nil {
		log.Printf("video analysis find failed id=%s: %v", id, err)
		return
	}
	gen := s.analysisGenerator()
	if gen == nil {
		s.failVideoAnalysisTask(id, "AI 视频分析未启用，请先在模型配置中配置对话模型")
		return
	}
	analysisURL := s.analysisJobVideoURL(ctx, job)
	analysis, err := gen.AnalyzeVideo(ctx, analysisURL, job.VideoName)
	if err != nil {
		s.failVideoAnalysisTask(id, err.Error())
		return
	}
	completeCtx, completeCancel := taskPersistContext()
	defer completeCancel()
	if err := s.videoAnalysis.Complete(completeCtx, id, videoanalysis.Result{
		Assets:         analysis.Assets,
		AudioSummary:   analysis.AudioSummary,
		Characters:     analysis.Characters,
		HasSpeech:      analysis.HasSpeech,
		RawResult:      analysis.RawResult,
		Scenes:         analysis.Scenes,
		SeedancePrompt: analysis.SeedancePrompt,
		SpeechKeywords: analysis.SpeechKeywords,
		SpeechOutline:  analysis.SpeechOutline,
		SpeechTopics:   analysis.SpeechTopics,
	}); err != nil {
		log.Printf("video analysis complete failed id=%s: %v", id, err)
	}
}

func (s *Server) failVideoAnalysisTask(id string, message string) {
	ctx, cancel := taskPersistContext()
	defer cancel()
	if err := s.videoAnalysis.Fail(ctx, id, message); err != nil {
		log.Printf("video analysis fail update failed id=%s: %v", id, err)
	}
}

func (s *Server) analysisJobVideoURL(ctx context.Context, job videoanalysis.Job) string {
	assetID := strings.TrimSpace(job.VideoAssetID)
	if assetID == "" || s.uploads == nil {
		return strings.TrimSpace(job.VideoURL)
	}
	id, err := strconv.ParseInt(assetID, 10, 64)
	if err != nil || id <= 0 {
		log.Printf("video analysis invalid asset id=%s", assetID)
		return strings.TrimSpace(job.VideoURL)
	}
	asset, err := s.uploads.Find(ctx, id)
	if err != nil {
		log.Printf("video analysis asset lookup failed id=%s: %v", assetID, err)
		return strings.TrimSpace(job.VideoURL)
	}
	if strings.TrimSpace(asset.ObjectURL) == "" {
		return strings.TrimSpace(job.VideoURL)
	}
	return s.analysisVideoURL(ctx, asset)
}

func (s *Server) videoStoryboards(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var body videostoryboard.CreateInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		analysis, err := s.videoAnalysis.Find(r.Context(), body.AnalysisJobID)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, "请选择有效的视频分析记录")
			return
		}
		if analysis.Status != "completed" {
			httpx.Fail(w, http.StatusBadRequest, "请选择已完成的视频分析记录")
			return
		}
		job, err := s.storyboards.Create(r.Context(), body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		s.startVideoStoryboardTask(job.ID)
		httpx.OK(w, job)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) videoStoryboardList(w http.ResponseWriter, r *http.Request) {
	result, err := s.storyboards.List(r.Context(), r.URL.Query())
	if err != nil {
		log.Printf("video storyboard list failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) videoStoryboardByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/storyboards/"), "/")
	id, action, hasAction := strings.Cut(rest, "/")
	if id == "" {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	if hasAction {
		if action != "retry" || r.Method != http.MethodPost {
			httpx.Fail(w, http.StatusNotFound, "Not Found")
			return
		}
		job, err := s.storyboards.Retry(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		s.startVideoStoryboardTask(job.ID)
		httpx.OK(w, job)
		return
	}
	switch r.Method {
	case http.MethodGet:
		job, err := s.storyboards.Find(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusNotFound, "Not Found")
			return
		}
		httpx.OK(w, job)
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
		var body videostoryboard.UpdateInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		job, err := s.storyboards.Update(r.Context(), id, body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, job)
	case http.MethodDelete:
		if err := s.storyboards.Delete(r.Context(), id); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, true)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) startVideoStoryboardTask(id string) {
	if !tryAcquireVideoAsyncSlot(s.videoStoryboardSlots) {
		s.failVideoStoryboardTask(id, "分镜任务繁忙，请稍后重试")
		return
	}
	go func() {
		defer releaseVideoAsyncSlot(s.videoStoryboardSlots)
		s.runVideoStoryboard(id)
	}()
}

func (s *Server) runVideoStoryboard(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("video storyboard task panic id=%s: %v", id, recovered)
			s.failVideoStoryboardTask(id, fmt.Sprintf("分镜任务异常：%v", recovered))
		}
	}()
	if err := s.storyboards.MarkRunning(ctx, id); err != nil {
		log.Printf("video storyboard mark running failed id=%s: %v", id, err)
		return
	}
	job, err := s.storyboards.Find(ctx, id)
	if err != nil {
		log.Printf("video storyboard find failed id=%s: %v", id, err)
		return
	}
	analysis, err := s.videoAnalysis.Find(ctx, job.AnalysisJobID)
	if err != nil {
		s.failVideoStoryboardTask(id, "关联的视频分析记录不存在")
		return
	}
	if analysis.Status != "completed" {
		s.failVideoStoryboardTask(id, "关联的视频分析记录尚未完成")
		return
	}
	gen := s.analysisGenerator()
	if gen == nil {
		s.failVideoStoryboardTask(id, "AI 分镜设计未启用，请先在模型配置中配置对话模型")
		return
	}
	input := llm.VideoStoryboardInput{
		AnalysisID:     analysis.ID,
		Assets:         analysis.Assets,
		AudioSummary:   analysis.AudioSummary,
		Characters:     analysis.Characters,
		Scenes:         analysis.Scenes,
		SeedancePrompt: analysis.SeedancePrompt,
		SpeechKeywords: analysis.SpeechKeywords,
		SpeechOutline:  analysis.SpeechOutline,
		SpeechTopics:   analysis.SpeechTopics,
		Theme:          job.Theme,
		VideoName:      analysis.VideoName,
	}
	result, err := generateVideoStoryboardWithRetry(ctx, func(attemptCtx context.Context) (llm.VideoStoryboardResult, error) {
		return gen.GenerateVideoStoryboard(attemptCtx, input)
	})
	if err != nil {
		s.failVideoStoryboardTaskWithRawResult(id, err.Error(), llm.StoryboardRawResultFromError(err))
		return
	}
	completeCtx, completeCancel := taskPersistContext()
	defer completeCancel()
	if err := s.storyboards.Complete(completeCtx, id, videostoryboard.Result{
		GlobalPrompt: result.GlobalPrompt,
		RawResult:    result.RawResult,
		Shots:        storyboardShotsFromLLM(result.Shots),
		StyleGuide:   result.StyleGuide,
		Title:        result.Title,
	}); err != nil {
		log.Printf("video storyboard complete failed id=%s: %v", id, err)
	}
}

func generateVideoStoryboardWithRetry(ctx context.Context, generate func(context.Context) (llm.VideoStoryboardResult, error)) (llm.VideoStoryboardResult, error) {
	var failures []string
	var lastRawResult string
	for attempt := 1; attempt <= 2; attempt++ {
		result, err := generate(ctx)
		if err == nil {
			return result, nil
		}
		if raw := llm.StoryboardRawResultFromError(err); raw != "" {
			lastRawResult = raw
		}
		failures = append(failures, fmt.Sprintf("第 %d 次失败：%v", attempt, err))
	}
	message := fmt.Sprintf("分镜设计自动重试后仍失败：%s", strings.Join(failures, "；"))
	if lastRawResult != "" {
		return llm.VideoStoryboardResult{}, llm.NewStoryboardRawResultError(message, lastRawResult)
	}
	return llm.VideoStoryboardResult{}, fmt.Errorf("%s", message)
}

func (s *Server) failVideoStoryboardTask(id string, message string) {
	s.failVideoStoryboardTaskWithRawResult(id, message, "")
}

func (s *Server) failVideoStoryboardTaskWithRawResult(id string, message string, rawResult string) {
	ctx, cancel := taskPersistContext()
	defer cancel()
	if err := s.storyboards.FailWithRawResult(ctx, id, message, rawResult); err != nil {
		log.Printf("video storyboard fail update failed id=%s: %v", id, err)
	}
}

func taskPersistContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func (s *Server) recoverVideoAsyncTasks() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if count, err := s.videoAnalysis.RecoverRunningAsFailed(ctx, "服务重启或任务超时，视频分析任务已中止，请重试"); err != nil {
		log.Printf("video analysis running recovery failed: %v", err)
	} else if count > 0 {
		log.Printf("video analysis recovered %d running task(s) as failed", count)
	}
	analysisIDs, err := s.videoAnalysis.QueuedIDs(ctx, 50)
	if err != nil {
		log.Printf("video analysis queued recovery failed: %v", err)
	} else {
		for _, id := range analysisIDs {
			s.startVideoAnalysisTask(id)
		}
	}

	if count, err := s.storyboards.RecoverRunningAsFailed(ctx, "服务重启或任务超时，分镜任务已中止，请重试"); err != nil {
		log.Printf("video storyboard running recovery failed: %v", err)
	} else if count > 0 {
		log.Printf("video storyboard recovered %d running task(s) as failed", count)
	}
	storyboardIDs, err := s.storyboards.QueuedIDs(ctx, 50)
	if err != nil {
		log.Printf("video storyboard queued recovery failed: %v", err)
		return
	}
	for _, id := range storyboardIDs {
		s.startVideoStoryboardTask(id)
	}
}

func storyboardShotsFromLLM(values []llm.VideoStoryboardShot) []videostoryboard.Shot {
	out := make([]videostoryboard.Shot, 0, len(values))
	for _, shot := range values {
		out = append(out, videostoryboard.Shot{
			Action:         shot.Action,
			Assets:         shot.Assets,
			Audio:          shot.Audio,
			Camera:         shot.Camera,
			Characters:     shot.Characters,
			Composition:    shot.Composition,
			Dialogue:       shot.Dialogue,
			Duration:       shot.Duration,
			Index:          shot.Index,
			Lighting:       shot.Lighting,
			Scene:          shot.Scene,
			SeedancePrompt: shot.SeedancePrompt,
			Title:          shot.Title,
		})
	}
	return out
}

func (s *Server) ragDocuments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.ragDocs.ListDocuments(r.Context(), queryMap(r))
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		var body ragstore.Document
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		result, err := s.ragDocs.SaveDocument(r.Context(), body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		s.ragCache.Invalidate()
		httpx.OK(w, result)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) ragDocumentByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/rag/documents/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		var body ragstore.Document
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		body.ID = id
		result, err := s.ragDocs.SaveDocument(r.Context(), body)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		s.ragCache.Invalidate()
		httpx.OK(w, result)
	case http.MethodDelete:
		ok, err := s.ragDocs.DeleteDocument(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		if ok {
			s.ragCache.Invalidate()
		}
		httpx.OK(w, ok)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) addSignupSubscriber(ch chan signup.Lead) {
	s.signupMu.Lock()
	defer s.signupMu.Unlock()
	s.signupSubscribers[ch] = struct{}{}
}

func (s *Server) removeSignupSubscriber(ch chan signup.Lead) {
	s.signupMu.Lock()
	defer s.signupMu.Unlock()
	delete(s.signupSubscribers, ch)
	close(ch)
}

func (s *Server) broadcastSignup(lead signup.Lead) {
	s.signupMu.Lock()
	defer s.signupMu.Unlock()
	for ch := range s.signupSubscribers {
		select {
		case ch <- lead:
		default:
		}
	}
}

// publicAdminBranding 后台品牌公开只读：启动屏与登录页在登录前即需要读取。
func (s *Server) publicAdminBranding(w http.ResponseWriter, _ *http.Request) {
	b, err := branding.Read(s.env.AdminConfig)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.Logo = publicAdminBrandingAssetURL(b.Logo)
	httpx.OK(w, b)
}

// adminBranding 读取/保存后台品牌配置；保存需登录。
func (s *Server) adminBranding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}

	switch r.Method {
	case http.MethodGet:
		b, err := branding.Read(s.env.AdminConfig)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, b)
	case http.MethodPut:
		var b branding.Branding
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		if err := branding.Write(s.env.AdminConfig, b); err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, _ := branding.Read(s.env.AdminConfig)
		httpx.OK(w, saved)
	}
}

// modelConfigView 是返回给前端的模型配置视图：密钥永不回传，
// 仅以布尔位告知当前是否已配置密钥（用于前端 placeholder 提示）。
type modelConfigView struct {
	Chat struct {
		Provider       string `json:"provider"`
		APIBase        string `json:"apiBase"`
		Model          string `json:"model"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
		APIKeySet      bool   `json:"apiKeySet"`
	} `json:"chat"`
	Video struct {
		APIBase   string `json:"apiBase"`
		Model     string `json:"model"`
		APIKeySet bool   `json:"apiKeySet"`
	} `json:"video"`
	Image struct {
		APIBase   string `json:"apiBase"`
		Model     string `json:"model"`
		APIKeySet bool   `json:"apiKeySet"`
	} `json:"image"`
	Analysis struct {
		APIBase   string `json:"apiBase"`
		GroupID   string `json:"groupId"`
		Model     string `json:"model"`
		APIKeySet bool   `json:"apiKeySet"`
	} `json:"analysis"`
	Admin struct {
		Provider       string `json:"provider"`
		APIBase        string `json:"apiBase"`
		GroupID        string `json:"groupId,omitempty"`
		Model          string `json:"model"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
		APIKeySet      bool   `json:"apiKeySet"`
	} `json:"admin"`
	DailyQuiz struct {
		Provider       string `json:"provider"`
		APIBase        string `json:"apiBase"`
		GroupID        string `json:"groupId,omitempty"`
		Model          string `json:"model"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
		APIKeySet      bool   `json:"apiKeySet"`
	} `json:"dailyQuiz"`
	// Assist 为 AI 辅助开关与系统提示词；提示词非密钥，可明文回显。
	Assist struct {
		Enabled      bool   `json:"enabled"`
		SystemPrompt string `json:"systemPrompt"`
	} `json:"assist"`
}

func modelConfigAuditSnapshot(cfg modelconfig.Config) map[string]any {
	return map[string]any{
		"assistEnabled": cfg.AssistEnabled(),
		"chat": map[string]any{
			"provider":       cfg.Chat.Provider,
			"apiBase":        cfg.Chat.APIBase,
			"apiKeySet":      cfg.Chat.APIKey != "",
			"model":          cfg.Chat.Model,
			"timeoutSeconds": cfg.Chat.TimeoutSeconds,
		},
		"video": map[string]any{
			"apiBase":   cfg.Video.APIBase,
			"apiKeySet": cfg.Video.APIKey != "",
			"model":     cfg.Video.Model,
		},
		"image": map[string]any{
			"apiBase":   cfg.Image.APIBase,
			"apiKeySet": cfg.Image.APIKey != "",
			"model":     cfg.Image.Model,
		},
		"analysis": map[string]any{
			"apiBase":   cfg.Analysis.APIBase,
			"apiKeySet": cfg.Analysis.APIKey != "",
			"groupId":   cfg.Analysis.GroupID,
			"model":     cfg.Analysis.Model,
		},
		"admin": map[string]any{
			"provider":       cfg.Admin.Provider,
			"apiBase":        cfg.Admin.APIBase,
			"apiKeySet":      cfg.Admin.APIKey != "",
			"groupId":        cfg.Admin.GroupID,
			"model":          cfg.Admin.Model,
			"timeoutSeconds": cfg.Admin.TimeoutSeconds,
		},
		"dailyQuiz": map[string]any{
			"provider":       cfg.DailyQuiz.Provider,
			"apiBase":        cfg.DailyQuiz.APIBase,
			"apiKeySet":      cfg.DailyQuiz.APIKey != "",
			"groupId":        cfg.DailyQuiz.GroupID,
			"model":          cfg.DailyQuiz.Model,
			"timeoutSeconds": cfg.DailyQuiz.TimeoutSeconds,
		},
	}
}

// buildModelConfigView 根据 env+DB 覆盖后的生效配置构造脱敏视图。
// dailyQuiz 返回的是每日题的单独覆盖值；留空字段在实际生成时继承 admin。
// stored 用于回显 AI 辅助开关与系统提示词（提示词非密钥，明文回显）。
func buildModelConfigView(chat modelconfig.ChatConfig, vid config.VideoConfig, img config.ImageConfig, analysis config.MiniMaxConfig, admin modelconfig.AdminModelConfig, dailyQuiz modelconfig.CompatibleModelConfig, stored modelconfig.Config) modelConfigView {
	var view modelConfigView
	view.Chat.Provider = chat.Provider
	view.Chat.APIBase = chat.APIBase
	view.Chat.Model = chat.Model
	view.Chat.TimeoutSeconds = chat.TimeoutSeconds
	view.Chat.APIKeySet = strings.TrimSpace(chat.APIKey) != ""
	view.Video.APIBase = vid.APIBase
	view.Video.Model = vid.Model
	view.Video.APIKeySet = strings.TrimSpace(vid.APIKey) != ""
	view.Image.APIBase = img.APIBase
	view.Image.Model = img.Model
	view.Image.APIKeySet = strings.TrimSpace(img.APIKey) != ""
	view.Analysis.APIBase = analysis.APIBase
	view.Analysis.GroupID = analysis.GroupID
	view.Analysis.Model = analysis.Model
	view.Analysis.APIKeySet = strings.TrimSpace(analysis.APIKey) != ""
	view.Admin.Provider = admin.Provider
	view.Admin.APIBase = admin.APIBase
	view.Admin.GroupID = admin.GroupID
	view.Admin.Model = admin.Model
	view.Admin.TimeoutSeconds = admin.TimeoutSeconds
	view.Admin.APIKeySet = strings.TrimSpace(admin.APIKey) != ""
	view.DailyQuiz.Provider = dailyQuiz.Provider
	view.DailyQuiz.APIBase = dailyQuiz.APIBase
	view.DailyQuiz.GroupID = dailyQuiz.GroupID
	view.DailyQuiz.Model = dailyQuiz.Model
	view.DailyQuiz.TimeoutSeconds = dailyQuiz.TimeoutSeconds
	view.DailyQuiz.APIKeySet = strings.TrimSpace(dailyQuiz.APIKey) != ""
	view.Assist.Enabled = stored.AssistEnabled()
	view.Assist.SystemPrompt = stored.Assist.SystemPrompt
	return view
}

func validateModelConfigBases(cfg modelconfig.Config) error {
	if err := validateExternalAPIBase("chat.apiBase", cfg.Chat.APIBase); err != nil {
		return err
	}
	return validateNonChatModelConfigBases(cfg)
}

func validateNonChatModelConfigBases(cfg modelconfig.Config) error {
	for label, apiBase := range map[string]string{
		"analysis.apiBase":  cfg.Analysis.APIBase,
		"admin.apiBase":     cfg.Admin.APIBase,
		"dailyQuiz.apiBase": cfg.DailyQuiz.APIBase,
		"image.apiBase":     cfg.Image.APIBase,
		"video.apiBase":     cfg.Video.APIBase,
	} {
		if err := validateExternalAPIBase(label, apiBase); err != nil {
			return err
		}
	}
	return nil
}

func validateExternalAPIBase(label string, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%s must be an http(s) URL", label)
	}
	if !netguard.IsPublicHTTPURL(raw) {
		return fmt.Errorf("%s must not point to a private or local address", label)
	}
	return nil
}

// modelConfig 读取/保存对话(MiniMax)与视频模型配置；GET/PUT 均需登录（已由 requireAuth 包裹）。
// GET 返回 env+DB 覆盖后的"生效"配置，但密钥一律脱敏为布尔位；
// PUT 仅持久化覆盖值，密钥留空表示"不修改"，保存后在写锁下重建运行时客户端。
func (s *Server) modelConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}

	switch r.Method {
	case http.MethodGet:
		stored, _, err := modelconfig.ReadStore(r.Context(), s.db)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		chat := stored.EffectiveChat()
		admin := stored.ApplyAdmin(s.env.MiniMax)
		httpx.OK(w, buildModelConfigView(chat, stored.ApplyVideo(s.env.Video), stored.ApplyImage(s.env.Image), stored.ApplyAnalysis(s.env.MiniMax), admin, stored.DailyQuiz, stored))

	case http.MethodPut:
		// 序列化完整 read→merge→probe→persist→swap 流程，避免并发页面保存
		// 丢字段或让 DB 与运行时指向不同版本。不要复用 modelMu 跨网络探活。
		s.modelConfigUpdateMu.Lock()
		defer s.modelConfigUpdateMu.Unlock()

		var incoming modelconfig.Config
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		stored, _, err := modelconfig.ReadStore(r.Context(), s.db)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		// 合并：同一 provider 的密钥留空表示沿用已存值；切换 provider 必须提交新密钥。
		merged := stored.MergeIncoming(incoming)
		storedChat := stored.EffectiveChat()
		mergedChat := merged.EffectiveChat()
		chatFieldsChanged := storedChat != mergedChat
		promptChanged := strings.TrimSpace(stored.Assist.SystemPrompt) != strings.TrimSpace(merged.Assist.SystemPrompt)
		assistWasEnabled := stored.AssistEnabled()
		assistEnabled := merged.AssistEnabled()
		assistChanged := assistWasEnabled != assistEnabled
		needBuild := chatFieldsChanged || (!assistWasEnabled && assistEnabled) || (promptChanged && assistEnabled)
		needProbe := chatFieldsChanged || (!assistWasEnabled && assistEnabled)
		if err := validateNonChatModelConfigBases(merged); err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}

		var candidate llm.ChatGenerator
		if needBuild {
			candidate, err = s.buildChatGenerator(merged)
			if err != nil {
				httpx.Fail(w, http.StatusBadRequest, err.Error())
				return
			}
			if needProbe {
				probeCtx, cancel := context.WithTimeout(r.Context(), s.modelConfigProbeDeadline(time.Duration(mergedChat.TimeoutSeconds)*time.Second))
				result := candidate.Ping(probeCtx)
				cancel()
				if !result.OK {
					httpx.Fail(w, http.StatusBadRequest, firstNonEmpty(result.Message, "对话模型连通性测试失败"))
					return
				}
			}
		}

		// Candidate 已验证后才持久化；只有持久化成功才做无失败运行时交换。
		if err := modelconfig.UpsertStore(r.Context(), s.db, merged); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		chat := merged.EffectiveChat()
		vid := merged.ApplyVideo(s.env.Video)
		img := merged.ApplyImage(s.env.Image)
		analysis := merged.ApplyAnalysis(s.env.MiniMax)
		admin := merged.ApplyAdmin(s.env.MiniMax)
		// 写锁下做不再失败的指针交换，使新配置立即生效。
		s.modelMu.Lock()
		if chatFieldsChanged || promptChanged || assistChanged {
			if assistEnabled {
				s.ragGen = candidate
			} else {
				s.ragGen = nil
			}
			if candidate != nil {
				s.chatTimeout = time.Duration(mergedChat.TimeoutSeconds) * time.Second
			}
		}
		s.analysisGen = llm.NewMiniMaxGenerator(analysis)
		s.videos = video.NewStore(s.db, s.uploads, vid, s.uploader)
		s.images = image.NewStore(s.uploads, img, s.uploader)
		s.modelMu.Unlock()

		s.recordAdminAudit(r, auditlog.Entry{
			Action:     "model_config.update",
			TargetType: "model_config",
			TargetID:   "global",
			Before:     modelConfigAuditSnapshot(stored),
			After:      modelConfigAuditSnapshot(merged),
			Summary:    "更新模型配置",
		})

		httpx.OK(w, buildModelConfigView(chat, vid, img, analysis, admin, merged.DailyQuiz, merged))
	}
}

// testChatModel 通过统一兼容协议工厂做一次对话模型连通性探活（POST，需登录）。
// 入参可选：前端表单当前填写的 provider/地址/密钥/模型名。密钥留空仅在 provider 不变时沿用，
// 这样用户既能测"刚输入还没保存"的配置，也能直接测"当前已保存"的配置。
// 返回结构化探活结果（ok/message/延迟），密钥一律不回传。
func (s *Server) testChatModel(w http.ResponseWriter, r *http.Request) {
	// 入参与 PUT 同结构，但仅取 chat 段；允许空 body（直接测已存配置）。
	var incoming modelconfig.Config
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil && !errors.Is(err, io.EOF) {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
	}

	stored, _, err := modelconfig.ReadStore(r.Context(), s.db)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 合并：密钥留空仅在 provider 不变时沿用已存值。
	merged := stored.MergeIncoming(incoming)
	chat := merged.EffectiveChat()
	generator, err := s.buildChatGenerator(merged)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(chat.TimeoutSeconds)*time.Second)
	defer cancel()
	result := generator.Ping(ctx)

	httpx.OK(w, result)
}

func (s *Server) recordAdminAudit(r *http.Request, entry auditlog.Entry) {
	if s == nil || s.auditLogs == nil || r == nil {
		return
	}
	user := userFromRequest(r)
	if user.ID > 0 || user.Username != "" || user.UserID != "" {
		entry.OperatorID = user.ID
		entry.OperatorName = firstNonEmpty(strings.TrimSpace(user.RealName), strings.TrimSpace(user.Username), strings.TrimSpace(user.UserID))
	}
	if entry.OperatorName == "" {
		entry.OperatorName = "unknown"
	}
	entry.IP = s.clientIP(r)
	entry.UserAgent = r.UserAgent()
	if err := s.auditLogs.Record(r.Context(), entry); err != nil {
		log.Printf("admin audit log failed action=%s target=%s/%s: %v", entry.Action, entry.TargetType, entry.TargetID, err)
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.authorize(w, r)
		if !ok {
			return
		}
		next(w, r.WithContext(withUser(r.Context(), user)))
	}
}

func (s *Server) requireUploadAssetAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.authorizeUploadAsset(w, r)
		if !ok {
			return
		}
		next(w, r.WithContext(withUser(r.Context(), user)))
	}
}

func (s *Server) requirePermission(code string, next http.HandlerFunc) http.HandlerFunc {
	return s.requireAnyPermission([]string{code}, next)
}

func (s *Server) requireUploadPermission(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.authorize(w, r)
		if !ok {
			return
		}
		if uploadDirAllowsSelfService(r.URL.Query().Get("dir")) {
			next(w, r.WithContext(withUser(r.Context(), user)))
			return
		}
		allowed, err := s.hasAnyPermission(r.Context(), user, uploadPermissionCodes...)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !allowed {
			httpx.Fail(w, http.StatusForbidden, "Forbidden")
			return
		}
		next(w, r.WithContext(withUser(r.Context(), user)))
	}
}

func (s *Server) requireMethodPermission(codes map[string]string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code, ok := codes[r.Method]
		if !ok {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		s.requirePermission(code, next)(w, r)
	}
}

func (s *Server) requireAnyPermission(codes []string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.authorize(w, r)
		if !ok {
			return
		}
		allowed, err := s.hasAnyPermission(r.Context(), user, codes...)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !allowed {
			httpx.Fail(w, http.StatusForbidden, "Forbidden")
			return
		}
		next(w, r.WithContext(withUser(r.Context(), user)))
	}
}

func (s *Server) hasAnyPermission(ctx context.Context, user auth.UserInfo, codes ...string) (bool, error) {
	if hasRole(user.Roles, "admin") {
		return true, nil
	}
	granted, err := s.system.AuthCodesForUser(ctx, user.ID, user.Roles)
	if err != nil {
		return false, err
	}
	if len(codes) == 0 {
		return len(granted) > 0, nil
	}
	allowed := make(map[string]struct{}, len(granted))
	for _, item := range granted {
		allowed[item] = struct{}{}
	}
	for _, code := range codes {
		if _, ok := allowed[code]; ok {
			return true, nil
		}
	}
	return false, nil
}

func hasRole(roles []string, expected string) bool {
	for _, role := range roles {
		if role == expected {
			return true
		}
	}
	return false
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) (auth.UserInfo, bool) {
	authorization := r.Header.Get("Authorization")
	return s.authorizeAuthorization(w, r, authorization)
}

func (s *Server) authorizeUploadAsset(w http.ResponseWriter, r *http.Request) (auth.UserInfo, bool) {
	return s.authorize(w, r)
}

func (s *Server) authorizeAuthorization(w http.ResponseWriter, r *http.Request, authorization string) (auth.UserInfo, bool) {
	tokenUser, err := auth.BearerUserWithKind(authorization, s.env.JWTSecret, auth.TokenKindBackend)
	if err != nil {
		httpx.Fail(w, http.StatusUnauthorized, "Unauthorized Exception")
		return auth.UserInfo{}, false
	}
	if s.db == nil {
		httpx.Fail(w, http.StatusUnauthorized, "Unauthorized Exception")
		return auth.UserInfo{}, false
	}
	currentVersion, verr := s.system.TokenVersion(r.Context(), tokenUser.ID)
	if verr != nil || tokenUser.TokenVersion == 0 || tokenUser.TokenVersion != currentVersion {
		httpx.Fail(w, http.StatusUnauthorized, "Unauthorized Exception")
		return auth.UserInfo{}, false
	}
	profile, err := s.system.CurrentUserProfile(r.Context(), tokenUser.ID, tokenUser.HomePath)
	if err != nil {
		httpx.Fail(w, http.StatusUnauthorized, "Unauthorized Exception")
		return auth.UserInfo{}, false
	}
	return auth.UserInfo{
		Avatar:       profile.Avatar,
		Email:        profile.Email,
		HomePath:     profile.HomePath,
		ID:           profile.ID,
		Phone:        profile.Phone,
		RealName:     profile.RealName,
		Remark:       profile.Remark,
		Roles:        profile.Roles,
		TokenKind:    auth.TokenKindBackend,
		TokenVersion: currentVersion,
		UserID:       profile.UserID,
		Username:     profile.Username,
	}, true
}

func mustSMSSender(cfg config.SMSConfig) sms.Sender {
	switch cfg.Provider {
	case "":
		return nil
	case "aliyun":
		sender, err := sms.NewAliyunSender(cfg.APIKey, cfg.APISecret, cfg.SignName, cfg.TemplateID)
		if err != nil {
			panic("sms init: " + err.Error())
		}
		return sender
	case "spug":
		sender, err := sms.NewSpugSender(sms.SpugOptions{
			APIBase:      cfg.SpugAPIBase,
			TemplateCode: cfg.SpugTemplateCode,
			TemplateName: cfg.SpugTemplateName,
			Timeout:      time.Duration(cfg.SpugTimeoutSeconds) * time.Second,
		})
		if err != nil {
			panic("sms init: " + err.Error())
		}
		return sender
	default:
		panic("sms init: unsupported provider " + cfg.Provider)
	}
}

func (s *Server) method(method string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		handler(w, r)
	}
}

func (s *Server) getOrHead(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		handler(w, r)
	}
}

func queryMap(r *http.Request) map[string]string {
	result := map[string]string{}
	for key, value := range r.URL.Query() {
		if len(value) > 0 {
			result[key] = value[0]
		}
	}
	return result
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if s.corsOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Language")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) corsOriginAllowed(origin string) bool {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin == "" {
		return false
	}
	if len(s.env.CORSAllowedOrigins) == 0 {
		return !config.IsProduction(s.env.AppEnv)
	}
	for _, allowed := range s.env.CORSAllowedOrigins {
		if strings.EqualFold(origin, strings.TrimRight(strings.TrimSpace(allowed), "/")) {
			return true
		}
	}
	return false
}
