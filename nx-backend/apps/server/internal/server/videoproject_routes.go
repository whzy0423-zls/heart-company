package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/videoproject"
)

// ========== 视频项目工作台 API ==========

func (s *Server) videoProjectStore() *videoproject.Store {
	return videoproject.NewStore(s.db)
}

func (s *Server) videoProjectGenerator() *videoproject.Generator {
	return videoproject.NewGenerator(
		s.videoProjectStore(),
		s.videoStore(),
		s.uploads,
		s.uploader,
	)
}

func (s *Server) videoProjectBatchGenerator() *videoproject.BatchGenerator {
	return videoproject.NewBatchGenerator(
		s.videoProjectGenerator(),
		s.videoProjectStore(),
	)
}

func (s *Server) videoProjectComposer() *videoproject.ProjectComposer {
	return videoproject.NewProjectComposer(
		s.videoProjectStore(),
		s.uploader,
		s.uploads,
	)
}

// 项目列表
func (s *Server) videoProjectList(w http.ResponseWriter, r *http.Request) {
	result, err := s.videoProjectStore().ListProjects(r.Context(), r.URL.Query())
	if err != nil {
		log.Printf("list video projects failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 创建项目
func (s *Server) createVideoProject(w http.ResponseWriter, r *http.Request) {
	var input videoproject.ProjectInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "参数错误")
		return
	}
	result, err := s.videoProjectStore().CreateProject(r.Context(), input)
	if err != nil {
		log.Printf("create video project failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 项目详情 / 更新 / 删除
func (s *Server) videoProjectByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		result, err := s.videoProjectStore().GetProject(r.Context(), id)
		if err != nil {
			log.Printf("get video project failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodPut:
		var input videoproject.ProjectInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "参数错误")
			return
		}
		result, err := s.videoProjectStore().UpdateProject(r.Context(), id, input)
		if err != nil {
			log.Printf("update video project failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodDelete:
		if err := s.videoProjectStore().DeleteProject(r.Context(), id); err != nil {
			log.Printf("delete video project failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, map[string]bool{"success": true})

	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

// 项目角色列表 / 创建
func (s *Server) videoProjectCharacters(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/projects-characters/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}
	projectID := parts[0]

	switch r.Method {
	case http.MethodGet:
		result, err := s.videoProjectStore().ListCharacters(r.Context(), projectID)
		if err != nil {
			log.Printf("list characters failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodPost:
		var input videoproject.CharacterInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "参数错误")
			return
		}
		result, err := s.videoProjectStore().CreateCharacter(r.Context(), projectID, input)
		if err != nil {
			log.Printf("create character failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodPut:
		if len(parts) < 2 {
			httpx.Fail(w, http.StatusBadRequest, "缺少角色 ID")
			return
		}
		charID := parts[1]
		var input videoproject.CharacterInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "参数错误")
			return
		}
		result, err := s.videoProjectStore().UpdateCharacter(r.Context(), charID, input)
		if err != nil {
			log.Printf("update character failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodDelete:
		if len(parts) < 2 {
			httpx.Fail(w, http.StatusBadRequest, "缺少角色 ID")
			return
		}
		charID := parts[1]
		if err := s.videoProjectStore().DeleteCharacter(r.Context(), charID); err != nil {
			log.Printf("delete character failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, map[string]bool{"success": true})

	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

// 项目场景列表 / 创建 / 更新 / 删除（同角色逻辑）
func (s *Server) videoProjectScenes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/projects-scenes/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}
	projectID := parts[0]

	switch r.Method {
	case http.MethodGet:
		result, err := s.videoProjectStore().ListScenes(r.Context(), projectID)
		if err != nil {
			log.Printf("list scenes failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodPost:
		var input videoproject.SceneInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "参数错误")
			return
		}
		result, err := s.videoProjectStore().CreateScene(r.Context(), projectID, input)
		if err != nil {
			log.Printf("create scene failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodPut:
		if len(parts) < 2 {
			httpx.Fail(w, http.StatusBadRequest, "缺少场景 ID")
			return
		}
		sceneID := parts[1]
		var input videoproject.SceneInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "参数错误")
			return
		}
		result, err := s.videoProjectStore().UpdateScene(r.Context(), sceneID, input)
		if err != nil {
			log.Printf("update scene failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodDelete:
		if len(parts) < 2 {
			httpx.Fail(w, http.StatusBadRequest, "缺少场景 ID")
			return
		}
		sceneID := parts[1]
		if err := s.videoProjectStore().DeleteScene(r.Context(), sceneID); err != nil {
			log.Printf("delete scene failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, map[string]bool{"success": true})

	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

// 分镜列表（独立路由，避免与 shots CRUD 冲突）
func (s *Server) videoProjectShotsList(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/projects-shots/list/")
	projectID := strings.Trim(path, "/")
	if projectID == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}
	result, err := s.videoProjectStore().ListShots(r.Context(), projectID)
	if err != nil {
		log.Printf("list shots failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 分镜创建
func (s *Server) videoProjectShots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/video/projects-shots/")
	projectID := strings.Trim(path, "/")
	if projectID == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}
	var input videoproject.ShotInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "参数错误")
		return
	}
	result, err := s.videoProjectStore().CreateShot(r.Context(), projectID, input)
	if err != nil {
		log.Printf("create shot failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 分镜详情 / 更新 / 删除
func (s *Server) videoShotByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/shots/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		result, err := s.videoProjectStore().GetShot(r.Context(), id)
		if err != nil {
			log.Printf("get shot failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodPut:
		var input videoproject.ShotInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "参数错误")
			return
		}
		result, err := s.videoProjectStore().UpdateShot(r.Context(), id, input)
		if err != nil {
			log.Printf("update shot failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodDelete:
		if err := s.videoProjectStore().DeleteShot(r.Context(), id); err != nil {
			log.Printf("delete shot failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, map[string]bool{"success": true})

	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

// 分镜预览（生成前查看提示词和参考素材）
func (s *Server) videoShotPreview(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/shots-preview/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID")
		return
	}
	builder := videoproject.NewPromptBuilder(s.videoProjectStore())
	preview, err := builder.BuildPreview(r.Context(), id)
	if err != nil {
		log.Printf("build shot preview failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, preview)
}

// 分镜生成（核心：智能提示词+参考素材+自动提取尾帧）
func (s *Server) generateVideoShot(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/shots-generate/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID")
		return
	}
	generation, err := s.videoProjectGenerator().GenerateShot(r.Context(), id)
	if err != nil {
		log.Printf("generate shot failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, generation)
}

// 批量生成项目所有分镜（顺序生成）
func (s *Server) batchGenerateShots(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects-batch-generate/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}

	// 支持两种模式：顺序(sequential)和并行(parallel)
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "sequential" // 默认顺序生成
	}

	var result interface{}
	var err error

	if mode == "parallel" {
		result, err = s.videoProjectBatchGenerator().GenerateAllShotsParallel(r.Context(), id)
	} else {
		result, err = s.videoProjectBatchGenerator().GenerateAllShots(r.Context(), id)
	}

	if err != nil {
		log.Printf("batch generate shots failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 获取批量生成进度
func (s *Server) batchGenerateProgress(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects-batch-progress/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}

	progress, err := s.videoProjectBatchGenerator().GetBatchProgress(r.Context(), id)
	if err != nil {
		log.Printf("get batch progress failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, progress)
}

// 合成项目视频
func (s *Server) composeProjectVideo(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects-compose/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}

	var input videoproject.ComposeProjectInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "参数错误")
		return
	}

	result, err := s.videoProjectComposer().ComposeProject(r.Context(), id, input)
	if err != nil {
		log.Printf("compose project video failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 获取合成状态
func (s *Server) composeStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects-compose-status/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}

	status, err := s.videoProjectComposer().GetComposeStatus(r.Context(), id)
	if err != nil {
		log.Printf("get compose status failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, status)
}
