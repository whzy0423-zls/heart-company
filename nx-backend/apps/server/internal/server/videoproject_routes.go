package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/video"
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

func workflowData(w http.ResponseWriter, status int, data any) {
	httpx.JSON(w, status, httpx.Response{Code: 0, Data: data, Message: "ok"})
}

func workflowError(w http.ResponseWriter, err error) {
	var active *video.ActiveSubmissionError
	var reconcile *video.ReconciliationTaskConflictError
	var composeActive *videoproject.ComposeActiveJobError
	var transition *video.InvalidSubmissionTransitionError
	switch {
	case errors.As(err, &active), errors.As(err, &reconcile), errors.As(err, &composeActive), errors.As(err, &transition):
		httpx.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, video.ErrInvalidSubmissionRequest):
		httpx.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, video.ErrSubmissionNotFound):
		httpx.Fail(w, http.StatusNotFound, err.Error())
	default:
		httpx.Fail(w, http.StatusUnprocessableEntity, err.Error())
	}
}

func validRequestKey(raw string) bool {
	raw = strings.TrimSpace(raw)
	return len(raw) == 36 && raw[8] == '-' && raw[13] == '-' && raw[18] == '-' && raw[23] == '-'
}

func (s *Server) videoWorkflowGet(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects-workflow/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}
	result, err := s.videoProjectStore().GetWorkflowStatus(r.Context(), id)
	if err != nil {
		workflowError(w, err)
		return
	}
	httpx.OK(w, result)
}

type workflowImportInput struct {
	Items          []videoproject.ScriptParagraph `json:"items"`
	ScriptRevision int                            `json:"scriptRevision"`
}

func (s *Server) videoWorkflowImport(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects-shots/from-script/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}
	var input workflowImportInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	result, err := s.videoProjectStore().CreateShotsFromScript(r.Context(), id, input.ScriptRevision, input.Items)
	if err != nil {
		workflowError(w, err)
		return
	}
	httpx.OK(w, result)
}

type safeGenerateInput struct {
	RequestKey string `json:"requestKey"`
}

func (s *Server) runWorkflowGenerate(w http.ResponseWriter, r *http.Request, id string, input safeGenerateInput) {
	if !validRequestKey(input.RequestKey) {
		httpx.Fail(w, http.StatusBadRequest, "缺少生成请求键或请求键格式错误")
		return
	}
	_, lookupErr := s.videoStore().SubmissionByRequestKey(r.Context(), input.RequestKey)
	reused := lookupErr == nil
	generation, err := s.videoProjectGenerator().GenerateShot(r.Context(), id, input.RequestKey)
	if err != nil {
		var unknown *video.UnknownOutcomeError
		if errors.As(err, &unknown) {
			workflowData(w, http.StatusAccepted, map[string]any{
				"requestKey": unknown.RequestKey,
				"taskId":     unknown.TaskID,
				"status":     video.SubmissionUnknownOutcome,
			})
			return
		}
		workflowError(w, err)
		return
	}
	status := http.StatusAccepted
	if reused {
		status = http.StatusOK
	}
	workflowData(w, status, generation)
}

func (s *Server) videoWorkflowGenerate(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/shots-generate-safe/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID")
		return
	}
	var input safeGenerateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	s.runWorkflowGenerate(w, r, id, input)
}

type safeBatchGenerateInput struct {
	Items []videoproject.SafeBatchGenerateItem `json:"items"`
}

func (s *Server) videoWorkflowBatchGenerate(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects-batch-generate-safe/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}
	var input safeBatchGenerateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Items) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	for _, item := range input.Items {
		if !validRequestKey(item.RequestKey) {
			httpx.Fail(w, http.StatusBadRequest, "缺少生成请求键或请求键格式错误")
			return
		}
	}
	result, err := s.videoProjectBatchGenerator().GenerateSafe(r.Context(), id, input.Items, r.URL.Query().Get("mode") == "parallel")
	if err != nil {
		workflowError(w, err)
		return
	}
	workflowData(w, http.StatusAccepted, result)
}

func (s *Server) videoWorkflowSubmissionStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/generation-submissions/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少提交 ID")
		return
	}
	result, err := s.videoStore().SubmissionByID(r.Context(), id)
	if err != nil {
		workflowError(w, err)
		return
	}
	httpx.OK(w, result)
}

type reconcileSubmissionInput struct {
	TaskID string `json:"taskId"`
}

func (s *Server) videoWorkflowReconcile(w http.ResponseWriter, r *http.Request) {
	requestKey := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/generation-submissions/reconcile/"), "/")
	if !validRequestKey(requestKey) {
		httpx.Fail(w, http.StatusBadRequest, "请求键格式错误")
		return
	}
	var input reconcileSubmissionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.TaskID) == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少 taskId")
		return
	}
	result, err := s.videoStore().ReconcileSubmission(r.Context(), requestKey, input.TaskID)
	if err != nil {
		workflowError(w, err)
		return
	}
	httpx.OK(w, result)
}

func (s *Server) videoWorkflowCompose(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects-compose-safe/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}
	var input videoproject.ComposeProjectInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	result, err := s.videoProjectComposer().StartCompose(r.Context(), id, input)
	if err != nil {
		workflowError(w, err)
		return
	}
	workflowData(w, http.StatusAccepted, result)
}

func (s *Server) videoWorkflowComposeStatus(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects-compose-safe-status/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID 或合成任务 ID")
		return
	}
	result, err := s.videoProjectComposer().GetComposeJob(r.Context(), parts[0], parts[1])
	if err != nil {
		workflowError(w, err)
		return
	}
	httpx.OK(w, result)
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

// 分镜素材列表
func (s *Server) videoShotAssetsList(w http.ResponseWriter, r *http.Request) {
	shotID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/shots-assets/list/"), "/")
	if shotID == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID")
		return
	}
	result, err := s.videoProjectStore().ListShotAssets(r.Context(), shotID)
	if err != nil {
		log.Printf("list shot assets failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 分镜素材创建 / 删除
func (s *Server) videoShotAssets(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/shots-assets/")
	id := strings.Trim(path, "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少 ID")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var input videoproject.ShotAssetInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "参数错误")
			return
		}
		result, err := s.videoProjectStore().CreateShotAsset(r.Context(), id, input)
		if err != nil {
			log.Printf("create shot asset failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, result)

	case http.MethodDelete:
		if err := s.videoProjectStore().DeleteShotAsset(r.Context(), id); err != nil {
			log.Printf("delete shot asset failed: %v", err)
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, map[string]bool{"success": true})

	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "方法不允许")
	}
}

// 分镜视频版本列表
func (s *Server) videoShotVideoVersionsList(w http.ResponseWriter, r *http.Request) {
	shotID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/list/"), "/")
	if shotID == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID")
		return
	}
	result, err := s.videoProjectStore().ListShotVideoVersions(r.Context(), shotID)
	if err != nil {
		log.Printf("list shot video versions failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 视频版本详情：为 liuguang 风格详情弹窗返回分镜、版本和生成参考内容。
func (s *Server) videoShotVideoVersionDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/detail/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID 或视频版本 ID")
		return
	}
	result, err := s.videoProjectStore().GetShotVideoVersionDetail(r.Context(), parts[0], parts[1])
	if err != nil {
		log.Printf("get shot video version detail failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 设置当前分镜视频版本
func (s *Server) setShotVideoVersion(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/set/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID 或视频版本 ID")
		return
	}
	result, err := s.videoProjectStore().SetShotVideoVersion(r.Context(), parts[0], parts[1])
	if err != nil {
		log.Printf("set shot video version failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 标记视频版本已查看，关闭 liuguang 风格未查看标记。
func (s *Server) markShotVideoVersionViewed(w http.ResponseWriter, r *http.Request) {
	generationID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/viewed/"), "/")
	if generationID == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少视频版本 ID")
		return
	}
	if err := s.videoProjectStore().MarkShotVideoVersionViewed(r.Context(), generationID); err != nil {
		log.Printf("mark shot video version viewed failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, map[string]bool{"success": true})
}

type setShotVideoVersionBackupInput struct {
	BackupFlag bool `json:"backupFlag"`
}

// 设置/取消备选视频版本，补齐 liuguang 的“备选视频”标记能力。
func (s *Server) setShotVideoVersionBackup(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/backup/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID 或视频版本 ID")
		return
	}

	var input setShotVideoVersionBackupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err != io.EOF {
		httpx.Fail(w, http.StatusBadRequest, "参数错误")
		return
	}

	result, err := s.videoProjectStore().SetShotVideoVersionBackup(r.Context(), parts[0], parts[1], input.BackupFlag)
	if err != nil {
		log.Printf("set shot video version backup failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 擦除字幕：创建一个 liuguang 风格“无字幕”派生视频版本。
func (s *Server) removeShotVideoVersionSubtitle(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/remove-subtitle/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID 或视频版本 ID")
		return
	}
	result, err := s.videoProjectGenerator().RemoveShotVideoVersionSubtitle(r.Context(), parts[0], parts[1])
	if err != nil {
		log.Printf("remove shot video version subtitle failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

type upscaleShotVideoVersionInput struct {
	Resolution string `json:"resolution"`
}

// 超分辨率：创建一个 liuguang 风格“已超分”派生视频版本。
func (s *Server) upscaleShotVideoVersion(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/upscale/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID 或视频版本 ID")
		return
	}

	input := upscaleShotVideoVersionInput{Resolution: "1080p"}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err != io.EOF {
		httpx.Fail(w, http.StatusBadRequest, "参数错误")
		return
	}

	result, err := s.videoProjectGenerator().UpscaleShotVideoVersion(r.Context(), parts[0], parts[1], input.Resolution)
	if err != nil {
		log.Printf("upscale shot video version failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 刷新分镜视频版本：轮询关联 generation 状态，并把当前版本状态同步回分镜。
func (s *Server) refreshShotVideoVersions(w http.ResponseWriter, r *http.Request) {
	shotID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/refresh/"), "/")
	if shotID == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID")
		return
	}

	store := s.videoProjectStore()
	versions, err := store.ListShotVideoVersions(r.Context(), shotID)
	if err != nil {
		log.Printf("list shot video versions before refresh failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	refreshErrors := []string{}
	for _, version := range versions {
		if strings.TrimSpace(version.ID) == "" {
			continue
		}
		generation, err := s.videoStore().Refresh(r.Context(), version.ID)
		if err != nil {
			log.Printf("refresh shot video generation failed shot=%s generation=%s: %v", shotID, version.ID, err)
			refreshErrors = append(refreshErrors, version.ID+": "+err.Error())
			continue
		}
		if version.IsCurrent {
			if _, err := store.SetShotVideoVersion(r.Context(), shotID, generation.ID); err != nil {
				log.Printf("sync current shot video version failed shot=%s generation=%s: %v", shotID, generation.ID, err)
				httpx.Fail(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	result, err := store.ListShotVideoVersions(r.Context(), shotID)
	if err != nil {
		log.Printf("list shot video versions after refresh failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(refreshErrors) > 0 {
		log.Printf("refresh shot video versions partial errors shot=%s: %s", shotID, strings.Join(refreshErrors, "; "))
	}
	httpx.OK(w, result)
}

// 复制某个分镜视频版本到另一个分镜，并设为目标分镜当前版本。
func (s *Server) copyShotVideoVersion(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/copy/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少源分镜 ID、视频版本 ID 或目标分镜 ID")
		return
	}
	result, err := s.videoProjectStore().CopyShotVideoVersion(r.Context(), parts[0], parts[1], parts[2])
	if err != nil {
		log.Printf("copy shot video version failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 视频抽帧：从某个视频版本抽首帧，作为图片参考素材写回当前分镜。
func (s *Server) extractShotVideoFrame(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/extract-frame/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID 或视频版本 ID")
		return
	}
	result, err := s.videoProjectGenerator().ExtractShotVideoFrame(r.Context(), parts[0], parts[1])
	if err != nil {
		log.Printf("extract shot video frame failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

// 删除分镜视频版本
func (s *Server) videoShotVideoVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httpx.Fail(w, http.StatusMethodNotAllowed, "仅支持 DELETE")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/video/shots-video-versions/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少分镜 ID 或视频版本 ID")
		return
	}
	if err := s.videoProjectStore().DeleteShotVideoVersion(r.Context(), parts[0], parts[1]); err != nil {
		log.Printf("delete shot video version failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, map[string]bool{"success": true})
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
	var input safeGenerateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || !validRequestKey(input.RequestKey) {
		httpx.Fail(w, http.StatusBadRequest, "缺少生成请求键")
		return
	}
	// Legacy callers delegate to the same paid-generation safety contract.
	s.runWorkflowGenerate(w, r, id, input)
}

type batchGenerateInput struct {
	Items []videoproject.SafeBatchGenerateItem `json:"items"`
}

// 批量生成项目所有分镜（顺序生成）
func (s *Server) batchGenerateShots(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/video/projects-batch-generate/"), "/")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "缺少项目 ID")
		return
	}

	var input batchGenerateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err != io.EOF {
		httpx.Fail(w, http.StatusBadRequest, "请求参数错误")
		return
	}

	// 支持两种模式：顺序(sequential)和并行(parallel)
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "sequential" // 默认顺序生成
	}

	if len(input.Items) == 0 {
		httpx.Fail(w, http.StatusBadRequest, "缺少生成请求键")
		return
	}
	for _, item := range input.Items {
		if !validRequestKey(item.RequestKey) {
			httpx.Fail(w, http.StatusBadRequest, "缺少生成请求键")
			return
		}
	}
	result, err := s.videoProjectBatchGenerator().GenerateSafe(r.Context(), id, input.Items, mode == "parallel")
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

	result, err := s.videoProjectComposer().StartCompose(r.Context(), id, input)
	if err != nil {
		log.Printf("compose project video failed: %v", err)
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	workflowData(w, http.StatusAccepted, result)
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
