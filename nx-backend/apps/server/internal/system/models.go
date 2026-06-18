package system

type PageResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// User 对齐前端 SystemUser 契约（nickname / roleIds）。
type User struct {
	Avatar     string   `json:"avatar"`
	CreateTime string   `json:"createTime"`
	Email      string   `json:"email"`
	ID         string   `json:"id"`
	Nickname   string   `json:"nickname"`
	Password   string   `json:"password,omitempty"`
	Phone      string   `json:"phone"`
	Remark     string   `json:"remark"`
	RoleIds    []string `json:"roleIds"`
	Status     int      `json:"status"`
	Username   string   `json:"username"`
}

type CurrentUserProfile struct {
	Avatar   string   `json:"avatar"`
	Email    string   `json:"email"`
	HomePath string   `json:"homePath"`
	ID       int64    `json:"id"`
	Phone    string   `json:"phone"`
	RealName string   `json:"realName"`
	Remark   string   `json:"remark"`
	Roles    []string `json:"roles"`
	UserID   string   `json:"userId"`
	Username string   `json:"username"`
}

type ProfileUpdate struct {
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	RealName string `json:"realName"`
	Remark   string `json:"remark"`
	Username string `json:"username"`
}

// Role 对齐前端 SystemRole 契约（menuIds）。
type Role struct {
	Code       string  `json:"code"`
	CreateTime string  `json:"createTime"`
	ID         string  `json:"id"`
	MenuIds    []int64 `json:"menuIds"`
	Name       string  `json:"name"`
	Remark     string  `json:"remark"`
	Status     int     `json:"status"`
}

// MenuItem 对齐前端 SystemMenu 契约。
type MenuItem struct {
	AuthCode  string         `json:"authCode,omitempty"`
	Children  []MenuItem     `json:"children,omitempty"`
	Component string         `json:"component,omitempty"`
	ID        int64          `json:"id"`
	Meta      map[string]any `json:"meta"`
	Name      string         `json:"name"`
	Path      string         `json:"path,omitempty"`
	PID       int64          `json:"pid,omitempty"`
	Sort      int            `json:"sort,omitempty"`
	Status    int            `json:"status"`
	Type      string         `json:"type"`
}
