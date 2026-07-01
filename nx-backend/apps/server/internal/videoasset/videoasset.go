// Package videoasset 资产库:按类型保存可复用的视频生成素材(场景/人物/物品/服装/风格/音频/视频)。
package videoasset

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

// 支持的资产类型。
var allowedTypes = map[string]bool{
	"scene":     true,
	"character": true,
	"prop":      true,
	"outfit":    true,
	"style":     true,
	"audio":     true,
	"video":     true,
}

type Store struct {
	db      *sql.DB
	uploads *uploadasset.Store
}

type Asset struct {
	AssetID    string `json:"assetId"`
	CoverURL   string `json:"coverUrl"`
	CreateTime string `json:"createTime"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Remark     string `json:"remark"`
	Status     string `json:"status"`
	Type       string `json:"type"`
	UpdateTime string `json:"updateTime"`
	URL        string `json:"url"`
}

type PageResult[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type CreateInput struct {
	AssetID  string `json:"assetId"`
	CoverURL string `json:"coverUrl"`
	Name     string `json:"name"`
	Remark   string `json:"remark"`
	Type     string `json:"type"`
	URL      string `json:"url"`
}

func NewStore(database *sql.DB, uploads *uploadasset.Store) *Store {
	return &Store{db: database, uploads: uploads}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

func (s *Store) List(ctx context.Context, query url.Values) (PageResult[Asset], error) {
	page, pageSize := pagination(query)
	where := []string{"1=1"}
	args := []any{}
	if keyword := strings.TrimSpace(query.Get("keyword")); keyword != "" {
		args = append(args, "%"+keyword+"%")
		where = append(where, fmt.Sprintf("(name ILIKE $%d OR remark ILIKE $%d)", len(args), len(args)))
	}
	if assetType := strings.TrimSpace(query.Get("type")); assetType != "" {
		args = append(args, assetType)
		where = append(where, fmt.Sprintf("type=$%d", len(args)))
	}
	if status := strings.TrimSpace(query.Get("status")); status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	condition := strings.Join(where, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM video_assets WHERE "+condition, args...).Scan(&total); err != nil {
		return PageResult[Asset]{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text, type, name, COALESCE(asset_id::text,''), url, cover_url, remark, status, create_time, update_time
		   FROM video_assets
		  WHERE `+condition+`
		  ORDER BY create_time DESC
		  LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return PageResult[Asset]{}, err
	}
	defer rows.Close()

	items := []Asset{}
	for rows.Next() {
		var item Asset
		var createTime, updateTime time.Time
		if err := rows.Scan(&item.ID, &item.Type, &item.Name, &item.AssetID, &item.URL, &item.CoverURL, &item.Remark, &item.Status, &createTime, &updateTime); err != nil {
			return PageResult[Asset]{}, err
		}
		item.CreateTime = formatTime(createTime)
		item.UpdateTime = formatTime(updateTime)
		items = append(items, item)
	}
	return PageResult[Asset]{Items: items, Total: total}, rows.Err()
}

func (s *Store) Create(ctx context.Context, input CreateInput) (Asset, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Asset{}, fmt.Errorf("请输入资产名称")
	}
	assetType := strings.TrimSpace(input.Type)
	if assetType == "" {
		assetType = "scene"
	}
	if !allowedTypes[assetType] {
		return Asset{}, fmt.Errorf("不支持的资产类型: %s", assetType)
	}
	rawURL := strings.TrimSpace(input.URL)
	assetID, err := parseOptionalID(input.AssetID)
	if err != nil {
		return Asset{}, fmt.Errorf("资产标识无效")
	}
	coverURL := strings.TrimSpace(input.CoverURL)
	if assetID > 0 && s.uploads != nil {
		asset, err := s.uploads.Find(ctx, assetID)
		if err != nil {
			return Asset{}, fmt.Errorf("上传资产不存在")
		}
		objectURL := strings.TrimSpace(asset.ObjectURL)
		if !isPublicHTTPURL(objectURL) {
			return Asset{}, fmt.Errorf("该资产没有文件桶公网地址，请先配置 OSS_PUBLIC_URL/文件桶公网访问后重新上传")
		}
		rawURL = objectURL
		if coverURL == "" {
			coverURL = rawURL
		}
	}
	if rawURL == "" {
		return Asset{}, fmt.Errorf("请先上传资产文件")
	}

	var id string
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO video_assets (type, name, asset_id, url, cover_url, remark, status)
		 VALUES ($1,$2,$3,$4,$5,$6,'active')
		 RETURNING id::text`,
		assetType, name, nullInt64(assetID), rawURL, coverURL, strings.TrimSpace(input.Remark),
	).Scan(&id)
	if err != nil {
		return Asset{}, err
	}
	return s.Find(ctx, id)
}

func (s *Store) Find(ctx context.Context, id string) (Asset, error) {
	var item Asset
	var createTime, updateTime time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT id::text, type, name, COALESCE(asset_id::text,''), url, cover_url, remark, status, create_time, update_time
		   FROM video_assets WHERE id=$1`, id,
	).Scan(&item.ID, &item.Type, &item.Name, &item.AssetID, &item.URL, &item.CoverURL, &item.Remark, &item.Status, &createTime, &updateTime)
	if err != nil {
		return Asset{}, err
	}
	item.CreateTime = formatTime(createTime)
	item.UpdateTime = formatTime(updateTime)
	return item, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM video_assets WHERE id=$1`, id)
	return err
}

func pagination(query url.Values) (int, int) {
	page := 1
	pageSize := 20
	if v := strings.TrimSpace(query.Get("page")); v != "" {
		_, _ = fmt.Sscan(v, &page)
	}
	if v := strings.TrimSpace(query.Get("pageSize")); v != "" {
		_, _ = fmt.Sscan(v, &pageSize)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

func parseOptionalID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	var id int64
	_, err := fmt.Sscan(value, &id)
	return id, err
}

func nullInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func isPublicHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !isPrivateOrLocalIP(ip)
	}
	return true
}

func isPrivateOrLocalIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
