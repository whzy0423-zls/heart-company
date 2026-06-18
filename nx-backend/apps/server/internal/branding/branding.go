// Package branding 管理后台自身的品牌配置（名称 / Logo / 启动加载文案），
// 以 JSON 文件持久化，结构与官网 siteconfig 类似但独立。
package branding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Branding 后台品牌配置。
type Branding struct {
	Name        string `json:"name"`        // 后台名称（侧边栏 / 标题 / 登录页）
	Logo        string `json:"logo"`        // Logo 图片地址（站内路径或外链）
	LoadingText string `json:"loadingText"` // 启动加载屏文案；为空则用 Name
}

var mu sync.RWMutex

// Defaults 返回内置默认品牌。
func Defaults() Branding {
	return Branding{
		Name:        "九型芯之力后台",
		Logo:        "/logo.png",
		LoadingText: "",
	}
}

// Read 读取品牌配置；文件不存在时返回默认值（非错误）。
func Read(path string) (Branding, error) {
	mu.RLock()
	defer mu.RUnlock()

	b := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return b, nil
		}
		return b, err
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return Defaults(), err
	}
	// 关键字段缺省回填，避免空值把前端搞乱。
	if b.Name == "" {
		b.Name = Defaults().Name
	}
	if b.Logo == "" {
		b.Logo = Defaults().Logo
	}
	return b, nil
}

// Write 落盘品牌配置（自动创建目录）。
func Write(path string, b Branding) error {
	mu.Lock()
	defer mu.Unlock()

	if b.Name == "" {
		b.Name = Defaults().Name
	}
	if b.Logo == "" {
		b.Logo = Defaults().Logo
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
