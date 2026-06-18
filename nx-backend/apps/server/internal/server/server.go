package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/branding"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/signup"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/system"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

type Server struct {
	env      config.Env
	mux      *http.ServeMux
	db       *sql.DB
	system   *system.Store
	builder  *siteconfig.Builder
	signups  *signup.Store
	uploads  *uploadasset.Store
	uploader storage.ObjectUploader
}

func New(env config.Env, database *sql.DB) http.Handler {
	s := &Server{
		env:      env,
		mux:      http.NewServeMux(),
		db:       database,
		system:   system.NewStore(database),
		builder:  siteconfig.NewBuilder(env.BuildScript, "", time.Duration(env.BuildTimeout)*time.Second),
		signups:  signup.NewStore(database),
		uploads:  uploadasset.NewStore(database),
		uploader: env.ObjectUploader,
	}
	s.routes()
	return s.withCORS(s.mux)
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
	s.mux.HandleFunc("/api/upload", s.method(http.MethodPost, s.requireAuth(s.upload)))
	s.mux.HandleFunc("/api/upload-assets/", s.method(http.MethodGet, s.uploadAsset))
	s.mux.Handle("/api/uploads/", http.StripPrefix("/api/uploads/", http.FileServer(http.Dir(s.env.UploadDir))))
	s.mux.HandleFunc("/api/site-config", s.siteConfig)
	s.mux.HandleFunc("/api/site-config/build-status", s.method(http.MethodGet, s.requireAuth(s.siteBuildStatus)))
	// 公开只读：给官网(website-react)运行时拉取，无需鉴权。
	s.mux.HandleFunc("/api/public/site-config", s.method(http.MethodGet, s.publicSiteConfig))
	s.mux.HandleFunc("/api/public/signups", s.method(http.MethodPost, s.publicSignup))
	// 后台品牌：公开只读（启动屏/登录页在登录前就要用），写入需鉴权。
	s.mux.HandleFunc("/api/public/admin-branding", s.method(http.MethodGet, s.publicAdminBranding))
	s.mux.HandleFunc("/api/admin-branding", s.adminBranding)
	s.mux.HandleFunc("/api/signups/list", s.method(http.MethodGet, s.requireAuth(s.signupList)))
	s.mux.HandleFunc("/api/system/user/list", s.method(http.MethodGet, s.requireAuth(s.system.HandleUsers)))
	s.mux.HandleFunc("/api/system/user", s.requireAuth(s.system.HandleUsers))
	s.mux.HandleFunc("/api/system/user/", s.requireAuth(s.system.HandleUserByID))
	s.mux.HandleFunc("/api/system/role/list", s.method(http.MethodGet, s.requireAuth(s.system.HandleRoles)))
	s.mux.HandleFunc("/api/system/role", s.requireAuth(s.system.HandleRoles))
	s.mux.HandleFunc("/api/system/role/", s.requireAuth(s.system.HandleRoleByID))
	s.mux.HandleFunc("/api/system/menu/list", s.method(http.MethodGet, s.requireAuth(s.system.HandleMenus)))
	s.mux.HandleFunc("/api/system/menu/name-exists", s.method(http.MethodGet, s.requireAuth(s.system.HandleMenuNameExists)))
	s.mux.HandleFunc("/api/system/menu/path-exists", s.method(http.MethodGet, s.requireAuth(s.system.HandleMenuPathExists)))
	s.mux.HandleFunc("/api/system/menu", s.requireAuth(s.system.HandleMenus))
	s.mux.HandleFunc("/api/system/menu/", s.requireAuth(s.system.HandleMenuByID))
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, map[string]string{"service": "nine-xing-vben-go-server", "status": "ok"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "BadRequestException")
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

	user := auth.UserInfo{
		HomePath: "/website/overview",
		ID:       id,
		RealName: nickname,
		Roles:    roleCodes,
		UserID:   fmt.Sprintf("%d", id),
		Username: body.Username,
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
	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(asset.Data)))
	_, _ = w.Write(asset.Data)
}

func (s *Server) objectUploader() (storage.ObjectUploader, error) {
	if s.uploader != nil {
		return s.uploader, nil
	}
	if s.env.OSS.AccessKeyID == "" && s.env.OSS.AccessKeySecret == "" && s.env.OSS.Bucket == "" {
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
		config, err := siteconfig.ReadStore(r.Context(), s.db, s.env.SiteConfig)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, config)
	case http.MethodPut:
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
	httpx.OK(w, config)
}

func (s *Server) publicSignup(w http.ResponseWriter, r *http.Request) {
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
	httpx.OK(w, lead)
}

func (s *Server) signupList(w http.ResponseWriter, r *http.Request) {
	result, err := s.signups.List(r.Context(), queryMap(r))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// publicAdminBranding 后台品牌公开只读：启动屏与登录页在登录前即需要读取。
func (s *Server) publicAdminBranding(w http.ResponseWriter, _ *http.Request) {
	b, err := branding.Read(s.env.AdminConfig)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
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
		if _, ok := s.authorize(w, r); !ok {
			return
		}
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

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.authorize(w, r)
		if !ok {
			return
		}
		next(w, r.WithContext(withUser(r.Context(), user)))
	}
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) (auth.UserInfo, bool) {
	user, err := auth.BearerUser(r.Header.Get("Authorization"), s.env.JWTSecret)
	if err != nil {
		httpx.Fail(w, http.StatusUnauthorized, "Unauthorized Exception")
		return auth.UserInfo{}, false
	}
	return user, true
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
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Language")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
