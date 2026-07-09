# 视频生成流程重设计 - 实战方案

## 一、核心目标

1. **降低抽卡率**：从 40-60% 降至 20-30%
2. **保持人物一致性**：同一角色在多段视频中风格统一
3. **保留现有 API 逻辑**：不改变 `generateVideoApi` 和 `refreshVideoGenerationApi` 的调用方式

---

## 二、即梦视频生成核心要点（基于实践经验）

### 2.1 即梦 Prompt 最佳实践

**结构公式**：
```
主体 + 动作 + 环境 + 风格 + 质量词 + 镜头参数
```

**示例**：
```
A young girl with long flowing brown hair wearing a white dress,
walking slowly through a sunlit forest path,
surrounded by tall pine trees and wildflowers,
Studio Ghibli animation style,
soft natural lighting, warm color palette,
medium shot, smooth camera movement,
high quality, detailed
```

**关键点**：
1. **主体描述要具体** - 不要只说"女孩"，要描述发型、服装、年龄特征
2. **动作要明确** - "walking slowly" 比 "moving" 效果好
3. **环境细节** - 具体到"松树林"比"森林"更好
4. **风格锁定** - 明确指定动画风格（如宫崎骏、皮克斯等）
5. **镜头语言** - medium shot（中景）最稳定，wide shot（远景）次之
6. **避免冲突词** - 不要同时要求"realistic"和"animation style"

### 2.2 人物一致性保障策略

**方法1：图片参考（最有效）**
```javascript
{
  prompt: "...",
  images: [
    "character_reference.jpg",  // 人物标准照
    "scene_reference.jpg"       // 场景参考
  ]
}
```
- 第一张图必须是**人物标准照**
- 标准照要求：正面或45度，清晰特征，单一背景

**方法2：详细描述模板**
```javascript
const characterTemplate = {
  name: "春日少女",
  description: "A young girl, approximately 12 years old, with long flowing brown hair reaching her waist, bright green eyes, fair skin, wearing a white cotton dress with blue floral patterns on the hem"
};
```
- 每次生成时，完整复用这段描述
- 存储在资产库的 `remark` 字段

**方法3：首帧继承**
```javascript
{
  prompt: "...",
  images: ["previous_video_end_frame.jpg"]  // 上一段视频的尾帧
}
```
- 适用于连续场景
- 自动提取上一段视频的最后一帧

---

## 三、新流程设计

### 3.1 核心概念：项目制工作流

```
项目 (Project)
  ├── 角色库 (Characters)
  │     ├── 主角
  │     └── 配角
  ├── 场景库 (Scenes)
  │     ├── 森林
  │     └── 小屋
  ├── 风格设定 (Style Guide)
  └── 分镜列表 (Shots)
        ├── Shot 1
        ├── Shot 2
        └── Shot 3
```

**每个 Shot 生成时，自动组装**：
```
Prompt = 角色描述 + Shot动作 + 场景描述 + 风格 + 质量词
Images = 角色参考图 + 场景参考图 + 上一Shot尾帧(如有)
```

### 3.2 数据库设计

```sql
-- 项目表
CREATE TABLE video_projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) NOT NULL,
    description TEXT,
    style_guide TEXT,  -- 全局风格描述
    status VARCHAR(20) DEFAULT 'active',
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 项目角色表
CREATE TABLE video_project_characters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES video_projects(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,  -- 详细的英文描述
    reference_image_url TEXT,  -- 角色标准照
    asset_id UUID REFERENCES video_assets(id),  -- 关联资产库
    is_main BOOLEAN DEFAULT false,  -- 是否主角
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 项目场景表
CREATE TABLE video_project_scenes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES video_projects(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,  -- 详细的英文描述
    reference_image_url TEXT,  -- 场景参考图
    asset_id UUID REFERENCES video_assets(id),
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 分镜表
CREATE TABLE video_shots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES video_projects(id) ON DELETE CASCADE,
    order_num INTEGER NOT NULL,
    name VARCHAR(200),
    action_description TEXT NOT NULL,  -- 动作描述（中文即可）
    duration INTEGER DEFAULT 15,
    aspect_ratio VARCHAR(10) DEFAULT '16:9',
    
    -- 关联的角色和场景
    character_ids UUID[],  -- 出场角色ID数组
    scene_id UUID REFERENCES video_project_scenes(id),
    
    -- 生成结果
    generation_id UUID REFERENCES video_generations(id),
    generated_prompt TEXT,  -- 实际使用的完整提示词
    start_frame_url TEXT,  -- 使用的首帧
    end_frame_url TEXT,  -- 生成后提取的尾帧
    
    status VARCHAR(20) DEFAULT 'draft',  -- draft/ready/generating/completed/failed
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 视频生成表（现有表，新增字段）
ALTER TABLE video_generations ADD COLUMN project_id UUID REFERENCES video_projects(id);
ALTER TABLE video_generations ADD COLUMN shot_id UUID REFERENCES video_shots(id);
```

### 3.3 提示词生成引擎

```go
// PromptBuilder 智能提示词构建器
type PromptBuilder struct {
    project *Project
    shot    *Shot
}

func (b *PromptBuilder) Build() (prompt string, images []string, err error) {
    var parts []string
    var imageRefs []string

    // 1. 角色描述（从project_characters获取）
    if len(b.shot.CharacterIDs) > 0 {
        chars := b.getCharacters(b.shot.CharacterIDs)
        for _, char := range chars {
            parts = append(parts, char.Description)
            if char.ReferenceImageURL != "" {
                imageRefs = append(imageRefs, char.ReferenceImageURL)
            }
        }
    }

    // 2. 动作（用户输入的中文描述 → 翻译为英文）
    action := b.translateAction(b.shot.ActionDescription)
    parts = append(parts, action)

    // 3. 场景描述
    if b.shot.SceneID != "" {
        scene := b.getScene(b.shot.SceneID)
        parts = append(parts, scene.Description)
        if scene.ReferenceImageURL != "" {
            imageRefs = append(imageRefs, scene.ReferenceImageURL)
        }
    }

    // 4. 全局风格
    if b.project.StyleGuide != "" {
        parts = append(parts, b.project.StyleGuide)
    }

    // 5. 固定增强词
    parts = append(parts, b.getQualityEnhancements())

    // 6. 首帧继承（如果是连续镜头）
    if b.shot.OrderNum > 1 {
        prevShot := b.getPreviousShot()
        if prevShot != nil && prevShot.EndFrameURL != "" {
            imageRefs = append([]string{prevShot.EndFrameURL}, imageRefs...)
        }
    }

    prompt = strings.Join(parts, ", ")
    return prompt, imageRefs, nil
}

func (b *PromptBuilder) getQualityEnhancements() string {
    // 根据镜头类型选择最佳参数
    enhancements := []string{
        "medium shot",           // 中景最稳定
        "smooth camera movement",
        "cinematic lighting",
        "high quality",
        "detailed",
    }
    
    // 根据场景类型添加光照
    if b.shot.SceneID != "" {
        scene := b.getScene(b.shot.SceneID)
        if strings.Contains(scene.Description, "forest") {
            enhancements = append(enhancements, "soft natural lighting, dappled sunlight")
        } else if strings.Contains(scene.Description, "night") {
            enhancements = append(enhancements, "moonlight, atmospheric lighting")
        }
    }
    
    return strings.Join(enhancements, ", ")
}
```

---

## 四、前端页面重设计

### 4.1 新页面结构

```
/video/projects         - 项目列表（新增）
/video/projects/:id     - 项目工作台（新增，替代原有generate.vue）
  ├── 角色管理 Tab
  ├── 场景管理 Tab
  ├── 分镜管理 Tab（核心）
  └── 生成监控 Tab
/video/assets          - 资产库（保留，作为素材来源）
/video/overview        - 生成概览（保留）
```

### 4.2 项目工作台界面

#### 布局草图
```
┌─────────────────────────────────────────────────────────────┐
│ 📽️ 项目：春日森林冒险                         [项目设置]    │
├─────────────────────────────────────────────────────────────┤
│ [角色] [场景] [分镜] [监控]  ← Tabs                          │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│ ▼ 分镜列表                                                    │
│                                                               │
│ ┌─ Shot 1: 女孩走进森林 ─────────────────────┐              │
│ │ 🎬 15秒 | 16:9                              │              │
│ │ 👤 主角: 春日少女                            │              │
│ │ 📍 场景: 松树林                              │              │
│ │ 📝 动作: 女孩好奇地东张西望，轻轻拨开树枝   │              │
│ │                                              │              │
│ │ 🤖 生成的提示词:                             │              │
│ │ A young girl, approximately 12 years old... │              │
│ │ [复制] [编辑]                                │              │
│ │                                              │              │
│ │ 📊 状态: ✅ 已完成  [预览视频] [重新生成]    │              │
│ └──────────────────────────────────────────────┘              │
│                                                               │
│ ┌─ Shot 2: 发现小屋 ──────────────────────────┐             │
│ │ 🎬 15秒 | 16:9                              │              │
│ │ 👤 主角: 春日少女                            │              │
│ │ 📍 场景: 森林小屋                            │              │
│ │ 📝 动作: 女孩眼睛一亮,加快脚步向小屋走去    │              │
│ │ 🔗 首帧: ← 继承 Shot 1 尾帧                 │              │
│ │                                              │              │
│ │ 📊 状态: 草稿  [生成此镜头]                  │              │
│ └──────────────────────────────────────────────┘              │
│                                                               │
│ [+ 新增分镜]          [批量生成全部] [导出项目]              │
└─────────────────────────────────────────────────────────────┘
```

#### 角色管理 Tab
```
┌─────────────────────────────────────────┐
│ 角色库                    [+ 添加角色]  │
├─────────────────────────────────────────┤
│ ┌─────────────────────────────────────┐ │
│ │ 👧 春日少女 (主角)                  │ │
│ │                                     │ │
│ │ [参考图]                            │ │
│ │  🖼️ character_ref.jpg               │ │
│ │                                     │ │
│ │ 📝 详细描述:                        │ │
│ │ A young girl, approximately 12...   │ │
│ │                                     │ │
│ │ [编辑] [删除] [从资产库导入]       │ │
│ └─────────────────────────────────────┘ │
│                                         │
│ ┌─────────────────────────────────────┐ │
│ │ 👴 森林老人 (配角)                  │ │
│ │ ...                                 │ │
│ └─────────────────────────────────────┘ │
└─────────────────────────────────────────┘
```

---

## 五、实施步骤

### Step 1: 数据库迁移（30分钟）
```sql
-- 创建上述新表
-- 执行脚本: migrations/add_video_projects.sql
```

### Step 2: 后端 API（2-3小时）

#### 新增接口
```go
// 项目管理
POST   /api/video/projects              - 创建项目
GET    /api/video/projects              - 项目列表
GET    /api/video/projects/:id          - 项目详情
PUT    /api/video/projects/:id          - 更新项目
DELETE /api/video/projects/:id          - 删除项目

// 角色管理
POST   /api/video/projects/:id/characters       - 添加角色
GET    /api/video/projects/:id/characters       - 角色列表
PUT    /api/video/projects/:id/characters/:cid  - 更新角色
DELETE /api/video/projects/:id/characters/:cid  - 删除角色

// 场景管理
POST   /api/video/projects/:id/scenes           - 添加场景
GET    /api/video/projects/:id/scenes           - 场景列表
PUT    /api/video/projects/:id/scenes/:sid      - 更新场景
DELETE /api/video/projects/:id/scenes/:sid      - 删除场景

// 分镜管理
POST   /api/video/projects/:id/shots            - 添加分镜
GET    /api/video/projects/:id/shots            - 分镜列表
PUT    /api/video/projects/:id/shots/:shotId    - 更新分镜
DELETE /api/video/projects/:id/shots/:shotId    - 删除分镜

// 核心：智能生成
POST   /api/video/shots/:shotId/generate        - 生成单个分镜视频
POST   /api/video/projects/:id/generate-all     - 批量生成全部分镜
```

#### 核心逻辑：`/api/video/shots/:shotId/generate`
```go
func (s *Server) generateShot(w http.ResponseWriter, r *http.Request) {
    shotID := getPathParam(r, "shotId")
    
    // 1. 获取Shot信息
    shot := s.getShot(shotID)
    project := s.getProject(shot.ProjectID)
    
    // 2. 构建提示词和参考图
    builder := NewPromptBuilder(project, shot)
    prompt, images, err := builder.Build()
    if err != nil {
        httpx.Fail(w, 400, err.Error())
        return
    }
    
    // 3. 调用现有的视频生成API（复用现有逻辑）
    input := video.GenerateInput{
        Prompt:      prompt,
        Images:      images,
        Model:       shot.Model,
        Seconds:     shot.Duration,
        AspectRatio: shot.AspectRatio,
    }
    generation, err := s.videoStore().Generate(r.Context(), input)
    if err != nil {
        httpx.Fail(w, 500, err.Error())
        return
    }
    
    // 4. 更新Shot记录
    s.db.Exec(`
        UPDATE video_shots 
        SET generation_id = $1, 
            generated_prompt = $2, 
            status = 'generating',
            update_time = now()
        WHERE id = $3
    `, generation.ID, prompt, shotID)
    
    // 5. 异步监控：生成完成后提取尾帧
    go s.monitorShotGeneration(shotID, generation.ID)
    
    httpx.OK(w, generation)
}

// 监控生成状态，完成后提取尾帧
func (s *Server) monitorShotGeneration(shotID, generationID string) {
    for {
        time.Sleep(5 * time.Second)
        
        gen, err := s.videoStore().Refresh(context.Background(), generationID)
        if err != nil {
            continue
        }
        
        if gen.Status == "completed" {
            // 提取尾帧
            endFrameURL := s.extractEndFrame(gen.VideoURL)
            
            // 更新Shot
            s.db.Exec(`
                UPDATE video_shots 
                SET end_frame_url = $1, status = 'completed'
                WHERE id = $2
            `, endFrameURL, shotID)
            break
        }
        
        if gen.Status == "failed" {
            s.db.Exec(`UPDATE video_shots SET status = 'failed' WHERE id = $1`, shotID)
            break
        }
    }
}
```

### Step 3: 前端页面（4-5小时）

#### 3.1 项目列表页 (`projects.vue`)
- 展示所有项目
- 创建新项目

#### 3.2 项目工作台 (`project-workbench.vue`)
- Tabs: 角色/场景/分镜/监控
- 核心是**分镜管理**页面

#### 3.3 分镜编辑器 (`ShotEditor.vue`)
```vue
<template>
  <Card>
    <Form>
      <FormItem label="镜头名称">
        <Input v-model="shot.name" />
      </FormItem>
      
      <FormItem label="出场角色">
        <Select v-model="shot.characterIds" mode="multiple">
          <Option 
            v-for="char in projectCharacters" 
            :key="char.id" 
            :value="char.id"
          >
            {{ char.name }}
          </Option>
        </Select>
      </FormItem>
      
      <FormItem label="场景">
        <Select v-model="shot.sceneId">
          <Option 
            v-for="scene in projectScenes" 
            :key="scene.id" 
            :value="scene.id"
          >
            {{ scene.name }}
          </Option>
        </Select>
      </FormItem>
      
      <FormItem label="动作描述">
        <TextArea 
          v-model="shot.actionDescription" 
          placeholder="中文即可，例如：女孩好奇地四处张望"
          :rows="3"
        />
      </FormItem>
      
      <FormItem label="时长">
        <Select v-model="shot.duration">
          <Option :value="5">5秒</Option>
          <Option :value="10">10秒</Option>
          <Option :value="15">15秒</Option>
        </Select>
      </FormItem>
      
      <Divider>智能生成预览</Divider>
      
      <Alert type="info">
        <template #message>
          <div class="generated-prompt">
            <strong>将生成的提示词：</strong>
            <p>{{ previewPrompt }}</p>
          </div>
          <div class="reference-images">
            <strong>参考图片：</strong>
            <Image.PreviewGroup>
              <Image 
                v-for="(img, i) in previewImages" 
                :key="i" 
                :src="img" 
                :width="80"
              />
            </Image.PreviewGroup>
          </div>
        </template>
      </Alert>
      
      <Button type="primary" @click="saveAndGenerate" :loading="generating">
        保存并生成视频
      </Button>
    </Form>
  </Card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';

const generating = ref(false);

// 实时预览提示词
const previewPrompt = computed(() => {
  const parts = [];
  
  // 角色描述
  shot.value.characterIds.forEach(id => {
    const char = projectCharacters.value.find(c => c.id === id);
    if (char) parts.push(char.description);
  });
  
  // 动作
  if (shot.value.actionDescription) {
    parts.push(translateAction(shot.value.actionDescription));
  }
  
  // 场景
  const scene = projectScenes.value.find(s => s.id === shot.value.sceneId);
  if (scene) parts.push(scene.description);
  
  // 风格
  if (project.value.styleGuide) {
    parts.push(project.value.styleGuide);
  }
  
  // 质量词
  parts.push('medium shot, smooth camera movement, cinematic lighting, high quality, detailed');
  
  return parts.join(', ');
});

// 预览参考图
const previewImages = computed(() => {
  const images = [];
  
  // 上一镜头尾帧（如果是连续镜头）
  if (shot.value.orderNum > 1) {
    const prevShot = getPreviousShot(shot.value.orderNum - 1);
    if (prevShot?.endFrameUrl) {
      images.push(prevShot.endFrameUrl);
    }
  }
  
  // 角色参考图
  shot.value.characterIds.forEach(id => {
    const char = projectCharacters.value.find(c => c.id === id);
    if (char?.referenceImageUrl) {
      images.push(char.referenceImageUrl);
    }
  });
  
  // 场景参考图
  const scene = projectScenes.value.find(s => s.id === shot.value.sceneId);
  if (scene?.referenceImageUrl) {
    images.push(scene.referenceImageUrl);
  }
  
  return images;
});

async function saveAndGenerate() {
  generating.value = true;
  try {
    // 调用后端 API
    const result = await generateShotApi(shot.value.id);
    message.success('视频生成已提交，请在监控页查看进度');
    emit('generated', result);
  } catch (error: any) {
    message.error(error.message || '生成失败');
  } finally {
    generating.value = false;
  }
}

// 简单的中译英（实际可以调用翻译API）
function translateAction(text: string): string {
  const map: Record<string, string> = {
    '走': 'walking',
    '跑': 'running',
    '看': 'looking',
    '笑': 'smiling',
    '四处张望': 'looking around curiously',
    '加快脚步': 'walking faster',
    // ... 可扩展
  };
  
  // 简单匹配
  for (const [cn, en] of Object.entries(map)) {
    if (text.includes(cn)) {
      text = text.replace(cn, en);
    }
  }
  
  return text;
}
</script>
```

---

## 六、预期效果对比

### 现有流程
```
用户手动输入提示词 
  → 可能不够详细/结构混乱 
  → 抽卡率 40-60%
  → 人物不一致
```

### 新流程
```
1. 项目初始化：定义角色、场景、风格
   ↓
2. 创建分镜：选择角色+场景，描述动作
   ↓
3. 自动组装：系统生成结构化提示词
   ↓
4. 参考图约束：角色标准照 + 场景参考 + 上一帧
   ↓
5. 生成视频：调用即梦API
   ↓
6. 自动提取尾帧：存储供下一镜头使用
```

**结果**：
- ✅ 提示词结构统一，质量稳定
- ✅ 角色参考图强制一致性
- ✅ 首帧继承保证连贯性
- ✅ 抽卡率降至 20-30%

---

## 七、快速启动路线图

### 第1天：基础架构
- [ ] 创建数据库表
- [ ] 项目/角色/场景的 CRUD API
- [ ] 前端项目列表页

### 第2天：核心功能
- [ ] 提示词构建引擎
- [ ] 分镜生成 API
- [ ] 前端分镜编辑器

### 第3天：监控和优化
- [ ] 生成监控页面
- [ ] 尾帧自动提取
- [ ] 批量生成功能

### 第4天：测试和调优
- [ ] 创建测试项目
- [ ] 对比新旧流程抽卡率
- [ ] 调整提示词模板

---

## 八、核心优势总结

1. **结构化管理** - 项目→角色→场景→分镜，层次清晰
2. **智能提示词** - 自动组装，无需人工编写
3. **强制一致性** - 角色参考图 + 详细描述模板
4. **自动化流程** - 首尾帧提取、批量生成
5. **保留现有API** - 底层调用逻辑不变，降低风险

---

**下一步：你确认后，我立即开始实施！**
