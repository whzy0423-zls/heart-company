package server

import (
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/branding"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

func publicSiteAssetURL(raw string) string {
	return publicConfigAssetURL(raw, "/api/public/site-assets/", "/api/public/site-uploads/")
}

func publicAdminBrandingAssetURL(raw string) string {
	return publicConfigAssetURL(raw, "/api/public/admin-branding-assets/", "/api/public/admin-branding-uploads/")
}

func publicConfigAssetURL(raw string, assetPrefix string, uploadPrefix string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if id, ok := uploadAssetIDFromURL(raw); ok {
		return fmt.Sprintf("%s%d", assetPrefix, id)
	}
	if rel, ok := localUploadRelativePath(raw); ok {
		return uploadPrefix + rel
	}
	return raw
}

func rewritePublicSiteConfigAssets(config *siteconfig.SiteConfig) {
	config.Site.Logo = publicSiteAssetURL(config.Site.Logo)
	config.Site.CustomerServiceQr = publicSiteAssetURL(config.Site.CustomerServiceQr)
	for i := range config.Types {
		config.Types[i].Avatar = publicSiteAssetURL(config.Types[i].Avatar)
	}
	if config.Home != nil {
		config.Home = rewritePublicAssetValue(config.Home, publicSiteAssetURL).(map[string]any)
	}
}

func rewritePublicAssetValue(value any, rewrite func(string) string) any {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			node[key] = rewritePublicAssetValue(child, rewrite)
		}
		return node
	case []any:
		for i, child := range node {
			node[i] = rewritePublicAssetValue(child, rewrite)
		}
		return node
	case string:
		return rewrite(node)
	default:
		return value
	}
}

func (s *Server) publicCustomerServiceQR(w http.ResponseWriter, r *http.Request) {
	config, err := siteconfig.ReadStore(r.Context(), s.db, s.env.SiteConfig)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	raw := strings.TrimSpace(config.Site.CustomerServiceQr)
	if raw == "" {
		http.NotFound(w, r)
		return
	}
	if id, ok := referencedUploadAssetID(raw); ok {
		asset, err := s.uploads.Find(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		writePublicUploadAsset(w, asset)
		return
	}
	if rel, ok := referencedLocalUploadRelativePath(raw); ok {
		s.servePublicLocalUpload(w, r, rel)
		return
	}
	s.serveRemoteCustomerServiceQR(w, r, raw)
}

func (s *Server) serveRemoteCustomerServiceQR(w http.ResponseWriter, r *http.Request, raw string) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || !publicRemoteHostAllowed(r, u.Hostname()) {
		http.NotFound(w, r)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, raw, nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.NotFound(w, r)
		return
	}
	limited := io.LimitReader(resp.Body, 8*1024*1024)
	body, err := io.ReadAll(limited)
	if err != nil || len(body) == 0 {
		http.NotFound(w, r)
		return
	}
	contentType := safePublicUploadAssetContentType(resp.Header.Get("Content-Type"))
	if contentType == "application/octet-stream" {
		contentType = safePublicUploadAssetContentType(http.DetectContentType(body))
	}
	if contentType == "application/octet-stream" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	_, _ = w.Write(body)
}

func publicRemoteHostAllowed(r *http.Request, hostname string) bool {
	hostname = strings.TrimSpace(strings.ToLower(hostname))
	if hostname == "" || hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") || strings.HasSuffix(hostname, ".local") {
		return false
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return publicRemoteIPAllowed(ip)
	}
	ctx := r.Context()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, item := range ips {
		if !publicRemoteIPAllowed(item.IP) {
			return false
		}
	}
	return true
}

func publicRemoteIPAllowed(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast()
}

func (s *Server) publicSiteAsset(w http.ResponseWriter, r *http.Request) {
	id := publicAssetIDFromPath(r.URL.Path, "/api/public/site-assets/")
	if id <= 0 {
		http.NotFound(w, r)
		return
	}
	config, err := siteconfig.ReadStore(r.Context(), s.db, s.env.SiteConfig)
	if err != nil || !siteConfigReferencesUploadAsset(config, id) {
		http.NotFound(w, r)
		return
	}
	asset, err := s.uploads.Find(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writePublicUploadAsset(w, asset)
}

func (s *Server) publicSiteUpload(w http.ResponseWriter, r *http.Request) {
	rel := publicUploadRelativePath(r.URL.Path, "/api/public/site-uploads/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	privateURL := "/api/uploads/" + rel
	config, err := siteconfig.ReadStore(r.Context(), s.db, s.env.SiteConfig)
	if err != nil || !siteConfigReferencesLocalUpload(config, privateURL) {
		http.NotFound(w, r)
		return
	}
	s.servePublicLocalUpload(w, r, rel)
}

func (s *Server) publicAdminBrandingAsset(w http.ResponseWriter, r *http.Request) {
	id := publicAssetIDFromPath(r.URL.Path, "/api/public/admin-branding-assets/")
	if id <= 0 {
		http.NotFound(w, r)
		return
	}
	b, err := branding.Read(s.env.AdminConfig)
	if err != nil || !valueReferencesUploadAsset(b.Logo, id) {
		http.NotFound(w, r)
		return
	}
	asset, err := s.uploads.Find(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writePublicUploadAsset(w, asset)
}

func (s *Server) publicAdminBrandingUpload(w http.ResponseWriter, r *http.Request) {
	rel := publicUploadRelativePath(r.URL.Path, "/api/public/admin-branding-uploads/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	privateURL := "/api/uploads/" + rel
	b, err := branding.Read(s.env.AdminConfig)
	if err != nil || !valueReferencesLocalUpload(b.Logo, privateURL) {
		http.NotFound(w, r)
		return
	}
	s.servePublicLocalUpload(w, r, rel)
}

func publicAssetIDFromPath(rawPath string, prefix string) int64 {
	idText := strings.Trim(strings.TrimPrefix(rawPath, prefix), "/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func siteConfigReferencesUploadAsset(config siteconfig.SiteConfig, id int64) bool {
	if valueReferencesUploadAsset(config.Site.Logo, id) {
		return true
	}
	if valueReferencesUploadAsset(config.Site.CustomerServiceQr, id) {
		return true
	}
	for _, item := range config.Types {
		if valueReferencesUploadAsset(item.Avatar, id) {
			return true
		}
	}
	return valueReferencesUploadAsset(config.Home, id)
}

func siteConfigReferencesLocalUpload(config siteconfig.SiteConfig, privateURL string) bool {
	if valueReferencesLocalUpload(config.Site.Logo, privateURL) {
		return true
	}
	if valueReferencesLocalUpload(config.Site.CustomerServiceQr, privateURL) {
		return true
	}
	for _, item := range config.Types {
		if valueReferencesLocalUpload(item.Avatar, privateURL) {
			return true
		}
	}
	return valueReferencesLocalUpload(config.Home, privateURL)
}

func valueReferencesUploadAsset(value any, id int64) bool {
	switch node := value.(type) {
	case map[string]any:
		for _, child := range node {
			if valueReferencesUploadAsset(child, id) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if valueReferencesUploadAsset(child, id) {
				return true
			}
		}
	case string:
		assetID, ok := referencedUploadAssetID(node)
		return ok && assetID == id
	}
	return false
}

func valueReferencesLocalUpload(value any, privateURL string) bool {
	privateRel, ok := localUploadRelativePath(privateURL)
	if !ok {
		return false
	}
	switch node := value.(type) {
	case map[string]any:
		for _, child := range node {
			if valueReferencesLocalUpload(child, privateURL) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if valueReferencesLocalUpload(child, privateURL) {
				return true
			}
		}
	case string:
		rel, ok := referencedLocalUploadRelativePath(node)
		return ok && rel == privateRel
	}
	return false
}

func referencedUploadAssetID(raw string) (int64, bool) {
	if id, ok := uploadAssetIDFromURL(raw); ok {
		return id, true
	}
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" {
		return 0, false
	}
	for _, prefix := range []string{
		"/api/public/admin-branding-assets/",
		"/api/public/article-assets/",
		"/api/public/site-assets/",
	} {
		if strings.HasPrefix(u.Path, prefix) {
			id := publicAssetIDFromPath(u.Path, prefix)
			return id, id > 0
		}
	}
	return 0, false
}

func referencedLocalUploadRelativePath(raw string) (string, bool) {
	if rel, ok := localUploadRelativePath(raw); ok {
		return rel, true
	}
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" {
		return "", false
	}
	for _, prefix := range []string{
		"/api/public/admin-branding-uploads/",
		"/api/public/article-uploads/",
		"/api/public/site-uploads/",
	} {
		if rel := publicUploadRelativePath(u.Path, prefix); rel != "" {
			return rel, true
		}
	}
	return "", false
}

func localUploadRelativePath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.IsAbs() || u.Host != "" {
		return "", false
	}
	const prefix = "/api/uploads/"
	if !strings.HasPrefix(u.Path, prefix) {
		return "", false
	}
	rel := publicUploadRelativePath(u.Path, prefix)
	return rel, rel != ""
}

func publicUploadRelativePath(rawPath string, prefix string) string {
	if !strings.HasPrefix(rawPath, prefix) {
		return ""
	}
	if len(rawPath) > 512 || strings.Contains(rawPath, "\\") {
		return ""
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(rawPath, prefix))
	if cleaned == "/" || strings.HasPrefix(cleaned, "/..") {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

func (s *Server) servePublicLocalUpload(w http.ResponseWriter, r *http.Request, rel string) {
	if strings.TrimSpace(s.env.UploadDir) == "" {
		http.NotFound(w, r)
		return
	}
	root, err := filepath.Abs(s.env.UploadDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	target, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil || !pathInsideRoot(root, target) {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(target)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	contentType := safePublicLocalUploadContentType(target, file)
	w.Header().Set("Content-Type", contentType)
	if contentType == "application/octet-stream" {
		w.Header().Set("Content-Disposition", "attachment")
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(target), info.ModTime(), file)
}

func pathInsideRoot(root string, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+string(os.PathSeparator))
}

func writePublicUploadAsset(w http.ResponseWriter, asset uploadasset.Asset) {
	contentType := safePublicUploadAssetContentType(asset.ContentType)
	w.Header().Set("Content-Type", contentType)
	if contentType == "application/octet-stream" {
		w.Header().Set("Content-Disposition", "attachment")
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(asset.Data)))
	_, _ = w.Write(asset.Data)
}

func safePublicUploadAssetContentType(contentType string) string {
	contentType = strings.TrimSpace(strings.ToLower(contentType))
	if contentType == "" {
		return "application/octet-stream"
	}
	base, _, _ := strings.Cut(contentType, ";")
	base = strings.TrimSpace(base)
	switch {
	case strings.HasPrefix(base, "audio/"):
		return contentType
	case strings.HasPrefix(base, "video/"):
		return contentType
	case strings.HasPrefix(base, "image/") && base != "image/svg+xml":
		return contentType
	case base == "application/pdf":
		return contentType
	default:
		return "application/octet-stream"
	}
}

func safePublicLocalUploadContentType(target string, file *os.File) string {
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(target)))
	if contentType == "" && file != nil {
		var head [512]byte
		n, _ := file.Read(head[:])
		_, _ = file.Seek(0, 0)
		if n > 0 {
			contentType = http.DetectContentType(head[:n])
		}
	}
	return safePublicUploadAssetContentType(contentType)
}
