package system

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

func (s *Store) HandleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.ListUsers(r.Context(), queryMap(r))
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodPost:
		var input User
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		saved, err := s.SaveUser(r.Context(), input)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, saved)
	case http.MethodDelete:
		// 前端用 DELETE /system/user?id=x
		id := r.URL.Query().Get("id")
		if id == "" {
			httpx.Fail(w, http.StatusBadRequest, "id is required")
			return
		}
		ok, err := s.DeleteUser(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, ok)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Store) HandleUserByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/system/user/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input User
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		input.ID = id
		saved, err := s.SaveUser(r.Context(), input)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, saved)
	case http.MethodDelete:
		ok, err := s.DeleteUser(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, ok)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Store) HandleRoles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.ListRoles(r.Context(), queryMap(r))
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)
	case http.MethodPost:
		var input Role
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		saved, err := s.SaveRole(r.Context(), input)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, saved)
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			httpx.Fail(w, http.StatusBadRequest, "id is required")
			return
		}
		ok, err := s.DeleteRole(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, ok)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Store) HandleRoleByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/system/role/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "id is required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input Role
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		input.ID = id
		saved, err := s.SaveRole(r.Context(), input)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, saved)
	case http.MethodDelete:
		ok, err := s.DeleteRole(r.Context(), id)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, ok)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Store) HandleMenus(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		menus, err := s.ListMenus(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, menus)
	case http.MethodPost:
		var input MenuItem
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		saved, err := s.SaveMenu(r.Context(), input)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.OK(w, saved)
	case http.MethodDelete:
		idText := r.URL.Query().Get("id")
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid menu id")
			return
		}
		ok, derr := s.DeleteMenu(r.Context(), id)
		if derr != nil {
			httpx.Fail(w, http.StatusInternalServerError, derr.Error())
			return
		}
		httpx.OK(w, ok)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Store) HandleMenuByID(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, "/api/system/menu/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid menu id")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input MenuItem
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		input.ID = id
		saved, serr := s.SaveMenu(r.Context(), input)
		if serr != nil {
			httpx.Fail(w, http.StatusBadRequest, serr.Error())
			return
		}
		httpx.OK(w, saved)
	case http.MethodDelete:
		ok, derr := s.DeleteMenu(r.Context(), id)
		if derr != nil {
			httpx.Fail(w, http.StatusInternalServerError, derr.Error())
			return
		}
		httpx.OK(w, ok)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Store) HandleMenuNameExists(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	exists, err := s.MenuNameExists(r.Context(), r.URL.Query().Get("name"), id)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, exists)
}

func (s *Store) HandleMenuPathExists(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	exists, err := s.MenuPathExists(r.Context(), r.URL.Query().Get("path"), id)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, exists)
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
