package skillcatalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type ImportResult struct {
	LibraryKey     string `json:"libraryKey"`
	CategoryCount  int    `json:"categoryCount"`
	SkillCount     int    `json:"skillCount"`
	PublishedCount int    `json:"publishedCount"`
	SkippedCount   int    `json:"skippedCount"`
}

func (s *Store) ApplyLearningGrowthCatalog(ctx context.Context, command CatalogCommand) (ImportResult, error) {
	if err := s.available(); err != nil {
		return ImportResult{}, err
	}
	if err := command.Validate(); err != nil {
		return ImportResult{}, err
	}
	catalog := LearningGrowthBuiltinCatalog()
	if command.Action == "retire" || command.Action == "rollback" {
		return s.applyCatalogVersionState(ctx, catalog, command)
	}
	manifest, manifestDigest, err := LoadReviewManifest(command.ManifestPath)
	if err != nil {
		return ImportResult{}, err
	}
	if err := manifest.Validate(catalog, command.Version); err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{LibraryKey: catalog.Key, CategoryCount: len(catalog.Categories)}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, fmt.Errorf("compile skill catalog: begin: %w", err)
	}
	defer tx.Rollback()

	var libraryID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO app_skill_libraries(key,name,description,icon_key,sort_order,status)
		VALUES($1,$2,$3,$4,0,'enabled')
		ON CONFLICT(key) DO UPDATE SET name=EXCLUDED.name,description=EXCLUDED.description,
		  icon_key=EXCLUDED.icon_key,status='enabled',update_time=now()
		RETURNING id`, catalog.Key, catalog.Name, catalog.Description, catalog.IconKey).Scan(&libraryID)
	if err != nil {
		return ImportResult{}, fmt.Errorf("compile skill catalog: library: %w", err)
	}

	for categoryIndex, category := range catalog.Categories {
		var categoryID int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO app_skill_categories(library_id,key,name,icon_key,color_token,sort_order,status)
			VALUES($1,$2,$3,$4,$5,$6,'enabled')
			ON CONFLICT(library_id,key) DO UPDATE SET name=EXCLUDED.name,icon_key=EXCLUDED.icon_key,
			  color_token=EXCLUDED.color_token,sort_order=EXCLUDED.sort_order,status='enabled',update_time=now()
			RETURNING id`, libraryID, category.Key, category.Name, category.IconKey, category.ColorToken, categoryIndex*100).Scan(&categoryID)
		if err != nil {
			return ImportResult{}, fmt.Errorf("compile skill catalog: category %s: %w", category.Key, err)
		}
		for skillIndex, definition := range category.Skills {
			result.SkillCount++
			decision := manifest.Skills[definition.Key]
			publishDecision := decision.Decision == "publish"
			if publishDecision {
				result.PublishedCount++
			}
			sources, err := loadApprovedSkillSources(filepath.Join(command.SourceDir, definition.Key))
			if err != nil {
				return ImportResult{}, fmt.Errorf("compile skill catalog: read %s: %w", definition.Key, err)
			}
			sourceHash := skillSourcesHash(sources)
			contentHash := reviewedContentHash(sourceHash, manifestDigest, command.Version, decision)
			status := "disabled"
			if command.Action == "publish" && publishDecision {
				status = "enabled"
			}
			var skillID int64
			err = tx.QueryRowContext(ctx, `
				INSERT INTO app_skills(category_id,key,name,summary,description,icon_key,color_token,status,sort_order)
				VALUES($1,$2,$3,$4,$4,$5,$6,$7,$8)
				ON CONFLICT(key) DO UPDATE SET category_id=EXCLUDED.category_id,name=EXCLUDED.name,
				  summary=EXCLUDED.summary,description=EXCLUDED.description,icon_key=EXCLUDED.icon_key,
				  color_token=EXCLUDED.color_token,
				  status=CASE WHEN $9='publish' THEN EXCLUDED.status ELSE app_skills.status END,
				  sort_order=EXCLUDED.sort_order,update_time=now()
				RETURNING id`, categoryID, definition.Key, definition.Name, definition.Summary,
				category.IconKey, category.ColorToken, status, categoryIndex*100+skillIndex, command.Action).Scan(&skillID)
			if err != nil {
				return ImportResult{}, fmt.Errorf("compile skill catalog: skill %s: %w", definition.Key, err)
			}

			var existingVersionID, existingReleaseID int64
			var existingHash, existingStatus string
			err = tx.QueryRowContext(ctx, `
				SELECT id,theory_release_id,content_hash,status FROM app_skill_versions
				WHERE skill_id=$1 AND version=$2`, skillID, command.Version).
				Scan(&existingVersionID, &existingReleaseID, &existingHash, &existingStatus)
			if err == nil {
				if existingHash != contentHash {
					return ImportResult{}, fmt.Errorf("compile skill catalog: version %s content or review decision changed for %s; create a new semantic version", command.Version, definition.Key)
				}
				changed, err := transitionCompiledVersion(ctx, tx, command.Action, publishDecision, skillID, existingVersionID, existingReleaseID, existingStatus)
				if err != nil {
					return ImportResult{}, fmt.Errorf("compile skill catalog: transition %s %s: %w", definition.Key, command.Version, err)
				}
				if !changed {
					result.SkippedCount++
				}
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return ImportResult{}, fmt.Errorf("compile skill catalog: inspect version %s: %w", definition.Key, err)
			}

			releaseID, err := compileSkillTheory(ctx, tx, category, definition, sources, sourceHash, command.Action == "publish" && publishDecision)
			if err != nil {
				return ImportResult{}, err
			}
			opening, _ := json.Marshal(skillOpeningPrompts(definition, category))
			files := make([]string, 0, len(sources))
			for _, source := range sources {
				files = append(files, filepath.ToSlash(filepath.Join(definition.Key, source.RelativePath)))
			}
			behaviorSections := compiledBehaviorSectionNames(sources)
			sourceMetadata, _ := json.Marshal(map[string]any{
				"source":             filepath.ToSlash(filepath.Join(definition.Key, "SKILL.md")),
				"knowledgeFiles":     files,
				"behaviorSections":   behaviorSections,
				"sourceNeeded":       decision.SourceNeeded,
				"riskNotices":        nonEmptyStrings(decision.RiskNotices),
				"reviewDecision":     decision.Decision,
				"reviewPolicy":       manifest.ReviewPolicy,
				"reviewDecisionRef":  manifest.DecisionRef,
				"reviewManifestHash": manifestDigest,
				"sourceContentHash":  sourceHash,
				"compilerPolicy":     "skill-compiler-v2",
			})
			versionStatus := targetVersionStatus(command.Action, publishDecision)
			var publishedAt any
			if versionStatus == "published" {
				publishedAt = "now"
			}
			instructions := compiledSkillInstructions(definition, category, sources)
			var versionID int64
			err = tx.QueryRowContext(ctx, `
				INSERT INTO app_skill_versions(skill_id,version,runtime_version,instructions,opening_prompts,
				  theory_release_id,safety_profile,content_hash,min_app_version,source_metadata,status,published_at)
				VALUES($1,$2,1,$3,$4::jsonb,$5,$6,$7,'1.0.1',$8::jsonb,$9,
				  CASE WHEN $10::text = 'now' THEN now() ELSE NULL END)
				RETURNING id`, skillID, command.Version, instructions, opening, releaseID,
				safetyProfileFor(definition.Key), contentHash, sourceMetadata, versionStatus, publishedAt).Scan(&versionID)
			if err != nil {
				return ImportResult{}, fmt.Errorf("compile skill catalog: version %s: %w", definition.Key, err)
			}
			if versionStatus == "published" {
				_, err = tx.ExecContext(ctx, `UPDATE app_skills SET latest_published_version_id=$2,status='enabled',update_time=now() WHERE id=$1`, skillID, versionID)
			} else if !publishDecision {
				_, err = tx.ExecContext(ctx, `UPDATE app_skills SET latest_published_version_id=NULL,status='disabled',update_time=now() WHERE id=$1`, skillID)
			}
			if err != nil {
				return ImportResult{}, fmt.Errorf("compile skill catalog: publish pointer %s: %w", definition.Key, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("compile skill catalog: commit: %w", err)
	}
	return result, nil
}

func reviewedContentHash(sourceHash, manifestDigest, version string, decision SkillReviewDecision) string {
	raw, _ := json.Marshal(decision)
	digest := sha256.Sum256([]byte(sourceHash + "\x00" + manifestDigest + "\x00" + version + "\x00" + string(raw)))
	return hex.EncodeToString(digest[:])
}

func targetVersionStatus(action string, publishDecision bool) string {
	if !publishDecision {
		return "draft"
	}
	switch action {
	case "ready":
		return "ready"
	case "publish":
		return "published"
	default:
		return "draft"
	}
}

func transitionCompiledVersion(ctx context.Context, tx *sql.Tx, action string, publishDecision bool, skillID, versionID, releaseID int64, status string) (bool, error) {
	target := targetVersionStatus(action, publishDecision)
	if status == target {
		if target == "published" {
			_, err := tx.ExecContext(ctx, `UPDATE app_skills SET status='enabled',latest_published_version_id=$2,update_time=now() WHERE id=$1`, skillID, versionID)
			return false, err
		}
		return false, nil
	}
	allowed := (status == "draft" && (target == "ready" || target == "published")) || (status == "ready" && target == "published")
	if !allowed {
		return false, fmt.Errorf("invalid transition %s to %s", status, target)
	}
	if target == "published" {
		if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET status='retired',update_time=now() WHERE library_id=(SELECT library_id FROM theory_library_releases WHERE id=$1) AND status='active' AND id<>$1`, releaseID); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE theory_cards SET status='superseded',update_time=now() WHERE library_id=(SELECT library_id FROM theory_library_releases WHERE id=$1) AND status='published' AND id NOT IN (SELECT card_id FROM theory_release_cards WHERE release_id=$1)`, releaseID); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE theory_cards SET status='published',published_at=COALESCE(published_at,now()),update_time=now() WHERE id IN (SELECT card_id FROM theory_release_cards WHERE release_id=$1)`, releaseID); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET status='active',activated_at=COALESCE(activated_at,now()),update_time=now() WHERE id=$1`, releaseID); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_skill_versions SET status=$2,published_at=CASE WHEN $2='published' THEN COALESCE(published_at,now()) ELSE published_at END,update_time=now() WHERE id=$1`, versionID, target); err != nil {
		return false, err
	}
	if target == "published" {
		_, err := tx.ExecContext(ctx, `UPDATE app_skills SET status='enabled',latest_published_version_id=$2,update_time=now() WHERE id=$1`, skillID, versionID)
		return true, err
	}
	return true, nil
}

func (s *Store) applyCatalogVersionState(ctx context.Context, catalog BuiltinCatalog, command CatalogCommand) (ImportResult, error) {
	result := ImportResult{LibraryKey: catalog.Key, CategoryCount: len(catalog.Categories), SkillCount: 35, PublishedCount: 32}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, err
	}
	defer tx.Rollback()
	if command.Action == "rollback" {
		rows, err := tx.QueryContext(ctx, `SELECT skill.id,version.id FROM app_skills skill JOIN app_skill_versions version ON version.skill_id=skill.id WHERE version.version=$1 AND version.status='published'`, command.Version)
		if err != nil {
			return ImportResult{}, err
		}
		var pairs [][2]int64
		for rows.Next() {
			var pair [2]int64
			if err := rows.Scan(&pair[0], &pair[1]); err != nil {
				rows.Close()
				return ImportResult{}, err
			}
			pairs = append(pairs, pair)
		}
		rows.Close()
		if len(pairs) != 32 {
			return ImportResult{}, fmt.Errorf("skill catalog: rollback target must contain 32 published skills, got %d", len(pairs))
		}
		for _, pair := range pairs {
			if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET status='retired',update_time=now() WHERE library_id=(SELECT release.library_id FROM app_skill_versions version JOIN theory_library_releases release ON release.id=version.theory_release_id WHERE version.id=$1) AND status='active' AND id<>(SELECT theory_release_id FROM app_skill_versions WHERE id=$1)`, pair[1]); err != nil {
				return ImportResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET status='active',update_time=now() WHERE id=(SELECT theory_release_id FROM app_skill_versions WHERE id=$1)`, pair[1]); err != nil {
				return ImportResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE app_skills SET latest_published_version_id=$2,status='enabled',update_time=now() WHERE id=$1`, pair[0], pair[1]); err != nil {
				return ImportResult{}, err
			}
		}
	} else {
		rows, err := tx.QueryContext(ctx, `SELECT skill.id,version.id FROM app_skills skill JOIN app_skill_versions version ON version.skill_id=skill.id WHERE version.version=$1 AND version.status='published'`, command.Version)
		if err != nil {
			return ImportResult{}, err
		}
		var pairs [][2]int64
		for rows.Next() {
			var pair [2]int64
			if err := rows.Scan(&pair[0], &pair[1]); err != nil {
				rows.Close()
				return ImportResult{}, err
			}
			pairs = append(pairs, pair)
		}
		rows.Close()
		if len(pairs) == 0 {
			return ImportResult{}, fmt.Errorf("skill catalog: published version %s not found", command.Version)
		}
		for _, pair := range pairs {
			var replacement sql.NullInt64
			_ = tx.QueryRowContext(ctx, `SELECT id FROM app_skill_versions WHERE skill_id=$1 AND status='published' AND id<>$2 ORDER BY published_at DESC NULLS LAST,id DESC LIMIT 1`, pair[0], pair[1]).Scan(&replacement)
			if replacement.Valid {
				if _, err := tx.ExecContext(ctx, `UPDATE app_skills SET latest_published_version_id=$2,status='enabled',update_time=now() WHERE id=$1`, pair[0], replacement.Int64); err != nil {
					return ImportResult{}, err
				}
			} else if _, err := tx.ExecContext(ctx, `UPDATE app_skills SET latest_published_version_id=NULL,status='disabled',update_time=now() WHERE id=$1`, pair[0]); err != nil {
				return ImportResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET status='retired',update_time=now() WHERE id=(SELECT theory_release_id FROM app_skill_versions WHERE id=$1)`, pair[1]); err != nil {
				return ImportResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE app_skill_versions SET status='retired',update_time=now() WHERE id=$1`, pair[1]); err != nil {
				return ImportResult{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}

func skillOpeningPrompts(skill BuiltinSkill, category BuiltinCategory) []string {
	return []string{
		"我想用“" + skill.Name + "”梳理一下当前最困扰我的问题。",
		"请从“" + category.Name + "”的角度，帮我把目标拆成一个可行动的小步骤。",
		"“" + skill.Name + "”中有哪些方法适合我现在的处境？",
	}
}

type approvedSkillSource struct {
	RelativePath string
	Content      string
}

func compileSkillTheory(ctx context.Context, tx *sql.Tx, category BuiltinCategory, definition BuiltinSkill, sources []approvedSkillSource, contentHash string, publish bool) (int64, error) {
	theoryKey := "skill-" + definition.Key
	libraryStatus, releaseStatus, cardStatus := "disabled", "ready", "draft"
	if publish {
		libraryStatus, releaseStatus, cardStatus = "enabled", "active", "published"
	}
	var libraryID int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO theory_libraries(key,name,description,status,current_version)
		VALUES($1,$2,$3,$4,0)
		ON CONFLICT(key) DO UPDATE SET name=EXCLUDED.name,description=EXCLUDED.description,
		  status=EXCLUDED.status,update_time=now()
		RETURNING id`, theoryKey, definition.Name, definition.Summary, libraryStatus).Scan(&libraryID)
	if err != nil {
		return 0, fmt.Errorf("compile skill theory: library %s: %w", definition.Key, err)
	}
	var releaseID int64
	var releaseVersion int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(version),0)+1 FROM theory_library_releases WHERE library_id=$1`, libraryID).Scan(&releaseVersion); err != nil {
		return 0, err
	}
	if publish {
		if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET status='retired',update_time=now() WHERE library_id=$1 AND status='active'`, libraryID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE theory_cards SET status='superseded',update_time=now() WHERE library_id=$1 AND status='published'`, libraryID); err != nil {
			return 0, err
		}
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO theory_library_releases(library_id,version,status,retrieval_mode,index_version)
		VALUES($1,$2,$3,'lexical_only',$4)
		RETURNING id`, libraryID, releaseVersion, releaseStatus, contentHash[:16]).Scan(&releaseID)
	if err != nil {
		return 0, fmt.Errorf("compile skill theory: release %s: %w", definition.Key, err)
	}
	var cardID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO theory_cards(library_id,canonical_key,canonical_name,card_kind,summary,definition,
		  epistemic_status,evidence_level,clinical_safety,authority_level,status,version,published_at)
		VALUES($1,$2,$3,'concept',$4,$4,'author_interpretation','unknown',$5,3,$6,$7,
		  CASE WHEN $6='published' THEN now() ELSE NULL END)
		RETURNING id`, libraryID, definition.Key, definition.Name, definition.Summary,
		clinicalSafetyFor(definition.Key), cardStatus, releaseVersion).Scan(&cardID)
	if err != nil {
		return 0, fmt.Errorf("compile skill theory: card %s: %w", definition.Key, err)
	}
	chunks := make([]approvedSkillSource, 0, len(sources)*2)
	for _, source := range sources {
		for index, part := range splitSkillMarkdown(source.Content) {
			chunks = append(chunks, approvedSkillSource{
				RelativePath: fmt.Sprintf("%s#%03d", source.RelativePath, index+1),
				Content:      "来源文件：" + source.RelativePath + "\n\n" + part,
			})
		}
	}
	keywords, _ := json.Marshal([]string{definition.Name, definition.Key, category.Name})
	tags, _ := json.Marshal([]string{"skill", category.Key})
	for index, chunk := range chunks {
		digest := sha256.Sum256([]byte(chunk.Content))
		chunkHash := hex.EncodeToString(digest[:])
		var chunkID int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO theory_chunks(library_id,card_id,chunk_key,chunk_kind,title,content,keywords,tags,
			  authority_level,evidence_level,clinical_safety,token_count,content_hash,status,version)
			VALUES($1,$2,$3,'card',$4,$5,$6::jsonb,$7::jsonb,3,'unknown',$8,$9,$10,'enabled',$11)
			RETURNING id`, libraryID, cardID, fmt.Sprintf("%s-%03d", definition.Key, index+1),
			definition.Name+" · "+chunk.RelativePath, chunk.Content, keywords, tags, clinicalSafetyFor(definition.Key), utf8.RuneCountInString(chunk.Content)/2+1, chunkHash, releaseVersion).Scan(&chunkID)
		if err != nil {
			return 0, fmt.Errorf("compile skill theory: chunk %s: %w", definition.Key, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO theory_release_cards(release_id,card_id,chunk_id) VALUES($1,$2,$3)`, releaseID, cardID, chunkID); err != nil {
			return 0, fmt.Errorf("compile skill theory: mapping %s: %w", definition.Key, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET card_count=1,chunk_count=$2,update_time=now() WHERE id=$1`, releaseID, len(chunks)); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_libraries SET current_version=$2,update_time=now() WHERE id=$1`, libraryID, releaseVersion); err != nil {
		return 0, err
	}
	return releaseID, nil
}

func loadApprovedSkillSources(skillDir string) ([]approvedSkillSource, error) {
	root, err := filepath.Abs(skillDir)
	if err != nil {
		return nil, err
	}
	allowedRoot := map[string]bool{"SKILL.md": true, "glossary.md": true, "patterns.md": true, "cheatsheet.md": true}
	paths := make([]string, 0, 12)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", relative)
		}
		if entry.IsDir() {
			if relative == "chapters" {
				return nil
			}
			if !strings.Contains(relative, "/") {
				return filepath.SkipDir
			}
			return nil
		}
		if allowedRoot[relative] || (strings.HasPrefix(relative, "chapters/") && strings.HasSuffix(strings.ToLower(relative), ".md")) {
			paths = append(paths, relative)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) == 0 || !containsString(paths, "SKILL.md") {
		return nil, errors.New("SKILL.md is required")
	}
	sources := make([]approvedSkillSource, 0, len(paths))
	for _, relative := range paths {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return nil, err
		}
		if len(raw) > 2<<20 {
			return nil, fmt.Errorf("approved markdown exceeds 2 MiB: %s", relative)
		}
		content := strings.TrimSpace(string(raw))
		if content == "" {
			continue
		}
		if err := validateApprovedSkillSource(relative, content); err != nil {
			return nil, err
		}
		sources = append(sources, approvedSkillSource{RelativePath: relative, Content: content})
	}
	if len(sources) == 0 || sources[0].RelativePath != "SKILL.md" {
		return nil, errors.New("SKILL.md is empty")
	}
	return sources, nil
}

func validateApprovedSkillSource(relative, content string) error {
	if !utf8.ValidString(content) || strings.IndexByte(content, 0) >= 0 {
		return fmt.Errorf("approved markdown is not valid text: %s", relative)
	}
	lower := strings.ToLower(content)
	for _, forbidden := range []string{
		"<script", "</script", "<iframe", "<object", "<embed", "<form",
		"javascript:", "data:text/html", "vbscript:", "file://",
	} {
		if strings.Contains(lower, forbidden) {
			return fmt.Errorf("approved markdown contains forbidden active content %q: %s", forbidden, relative)
		}
	}
	return nil
}

func skillSourcesHash(sources []approvedSkillSource) string {
	digest := sha256.New()
	for _, source := range sources {
		_, _ = digest.Write([]byte(source.RelativePath))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(source.Content))
		_, _ = digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func compiledSkillInstructions(skill BuiltinSkill, category BuiltinCategory, sources []approvedSkillSource) string {
	base := "你负责“" + skill.Name + "”这一独立技能。围绕“" + category.Name + "”领域，优先使用当前固定版本的检索资料回答。" +
		"先理解用户的具体处境，再提炼资料中的概念、方法或练习；不要声称资料未提供的作者观点或原文。" +
		"不读取或推断人物卡、九型类型、其他技能、其他会话或长期用户画像。资料不足时明确边界，并只追问一个必要问题。"
	for _, source := range sources {
		if source.RelativePath != "SKILL.md" {
			continue
		}
		rules, _ := extractControlledBehaviorSections(source.Content)
		if rules != "" {
			return base + "\n\n以下是按编译策略提取并固化在当前技能版本中的受控行为规则：\n" + rules
		}
		break
	}
	return base
}

func compiledBehaviorSectionNames(sources []approvedSkillSource) []string {
	for _, source := range sources {
		if source.RelativePath == "SKILL.md" {
			_, sections := extractControlledBehaviorSections(source.Content)
			return sections
		}
	}
	return []string{}
}

func extractControlledBehaviorSections(content string) (string, []string) {
	content = stripSkillFrontMatter(content)
	allowed := func(title string) bool {
		title = strings.TrimSpace(title)
		for _, marker := range []string{"何时使用", "如何使用", "使用方式", "使用范围", "默认工作流", "输出要求", "输出模板", "范围与限制", "当前状态"} {
			if strings.HasPrefix(title, marker) {
				return true
			}
		}
		return false
	}
	var out strings.Builder
	sections := make([]string, 0, 4)
	include := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			title := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			include = allowed(title)
			if include {
				sections = append(sections, title)
				if out.Len() > 0 {
					out.WriteByte('\n')
				}
				out.WriteString("## ")
				out.WriteString(title)
				out.WriteByte('\n')
			}
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			include = false
			continue
		}
		if include {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return strings.TrimSpace(out.String()), sections
}

func stripSkillFrontMatter(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	if end := strings.Index(content[4:], "\n---\n"); end >= 0 {
		return strings.TrimSpace(content[4+end+5:])
	}
	return content
}

func safetyProfileFor(key string) string {
	switch key {
	case "qinmi-guanxi":
		return "sensitive-relationships-v1"
	case "traditional-chinese-health", "brain-and-cognitive-science":
		return "health-information-v1"
	case "sociology-of-human-emotions":
		return "sensitive-guidance-v1"
	default:
		return "general-learning-v1"
	}
}

func clinicalSafetyFor(key string) string {
	if safetyProfileFor(key) != "general-learning-v1" {
		return "caution"
	}
	return "general"
}

func splitSkillMarkdown(content string) []string {
	const maxRunes = 3500
	lines := strings.Split(strings.TrimSpace(content), "\n")
	chunks := make([]string, 0, 4)
	var current strings.Builder
	flush := func() {
		value := strings.TrimSpace(current.String())
		if value != "" {
			chunks = append(chunks, value)
		}
		current.Reset()
	}
	for _, line := range lines {
		lineRunes := utf8.RuneCountInString(line) + 1
		if current.Len() > 0 && (strings.HasPrefix(strings.TrimSpace(line), "## ") || utf8.RuneCountInString(current.String())+lineRunes > maxRunes) {
			flush()
		}
		if utf8.RuneCountInString(line) <= maxRunes {
			current.WriteString(line)
			current.WriteByte('\n')
			continue
		}
		remaining := []rune(line)
		for len(remaining) > 0 {
			take := maxRunes
			if len(remaining) < take {
				take = len(remaining)
			}
			current.WriteString(string(remaining[:take]))
			remaining = remaining[take:]
			flush()
		}
	}
	flush()
	if len(chunks) == 0 {
		return []string{content}
	}
	return chunks
}
