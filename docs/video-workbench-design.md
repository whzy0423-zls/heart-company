# 视频生成工作台 - 智能化设计方案

## 一、现状分析

### 当前模块结构
```
视频生成中心（VideoCenter）
├── 视频生成（generate.vue）- 单次生成，手动输入提示词
├── 资产库（assets.vue）- 场景/人物/物品/服装/风格/音频/视频
├── 视频分析（analysis.vue）- 视频逆向分析，提取场景/人物/音频
├── 分镜设计（storyboard.vue）- 从主题生成多分镜方案
└── 生成概览（overview.vue）- 统计数据和最近记录
```

### 核心痛点
1. **抽卡率高** - 提示词质量不稳定，重复生成浪费成本
2. **资产孤立** - 资产库、分镜、生成三者割裂，无法形成工作流
3. **缺少首尾帧管理** - 无法实现视频连贯性（尤其是系列视频）
4. **提示词工程依赖人工** - 需要手动组织场景、人物、风格描述
5. **批量生成困难** - 分镜方案无法直接批量生成视频

---

## 二、工作台核心设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                    视频生成工作台 (Workbench)                 │
│                                                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ 剧本管理     │  │ 资产组装     │  │ 批量生成     │      │
│  │ Script       │  │ Asset Compose│  │ Batch Gen    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │             智能提示词引擎 (Prompt Engine)            │   │
│  │  - 资产语义化                                         │   │
│  │  - 首尾帧继承                                         │   │
│  │  - 风格一致性保障                                     │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              首尾帧管理 (Frame Manager)               │   │
│  │  - 视频抽帧                                           │   │
│  │  - 首尾帧库                                           │   │
│  │  - 连贯性检测                                         │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
          ↓                    ↓                    ↓
   ┌──────────┐        ┌──────────┐        ┌──────────┐
   │ 资产库   │        │ 分镜设计 │        │ 视频生成 │
   │ Assets   │        │ Storyboard│       │ Generate │
   └──────────┘        └──────────┘        └──────────┘
```

---

## 三、核心功能模块

### 3.1 剧本管理模块

**目标**：从创意到分镜的结构化管理

#### 数据模型
```go
type VideoScript struct {
    ID          string    // 剧本ID
    Title       string    // 剧本标题
    Theme       string    // 主题
    Duration    int       // 总时长（秒）
    Scenes      []Scene   // 场景列表
    Characters  []string  // 涉及角色（关联资产库）
    Style       string    // 整体风格（关联资产库）
    CreateTime  time.Time
}

type Scene struct {
    ID           string   // 场景ID
    ScriptID     string   // 所属剧本
    Order        int      // 场景顺序
    Duration     int      // 场景时长
    Description  string   // 场景描述
    Location     string   // 场景地点（关联资产库scene）
    Characters   []string // 出场角色（关联资产库character）
    Props        []string // 道具（关联资产库prop）
    Action       string   // 动作描述
    Mood         string   // 情绪氛围
    StartFrame   string   // 首帧图片URL（可选）
    EndFrame     string   // 尾帧图片URL（可选，用于下场景衔接）
}
```

#### 功能特性
1. **AI辅助分镜** - 输入主题，AI生成完整剧本（复用现有storyboard逻辑）
2. **资产快速绑定** - 场景拖拽绑定资产库素材
3. **首尾帧预设** - 手动上传或从已生成视频抽帧
4. **连贯性检测** - 检测相邻场景首尾帧风格一致性

---

### 3.2 资产组装模块

**目标**：智能生成高质量即梦提示词，降低抽卡率

#### 提示词模板引擎

```typescript
interface PromptTemplate {
  // 基础结构
  structure: string; // "场景 + 主体 + 动作 + 风格 + 细节"
  
  // 资产映射
  assets: {
    scene?: string;      // 场景资产ID
    characters: string[]; // 角色资产ID列表
    props?: string[];     // 道具资产ID列表
    style?: string;       // 风格资产ID
  };
  
  // 增强参数
  enhancements: {
    lighting?: string;    // 光照：soft/dramatic/natural
    camera?: string;      // 机位：wide/medium/close-up
    movement?: string;    // 运镜：static/pan/zoom
    mood?: string;        // 情绪：peaceful/tense/joyful
  };
  
  // 继承参数
  inherit?: {
    previousEndFrame?: string; // 上一段尾帧作为首帧
    styleConsistency?: boolean; // 风格一致性保障
  };
}
```

#### 提示词生成策略

**策略1：资产语义化**
```
资产库存储格式：
{
  name: "森林小屋",
  type: "scene",
  remark: "被绿色植物环绕的木质小屋，温馨宁静"
}

↓ 提取语义特征

生成提示词片段：
"a cozy wooden cabin surrounded by lush green forest, peaceful atmosphere"
```

**策略2：多资产融合**
```
场景：森林小屋
人物：年轻女孩（长发，白裙）
道具：篮子
风格：宫崎骏动画风格

↓ 智能组合

最终提示词：
"A young girl with long hair in a white dress holding a woven basket, 
standing in front of a cozy wooden cabin surrounded by lush green forest, 
Studio Ghibli animation style, soft natural lighting, peaceful atmosphere, 
medium shot, gentle camera movement"
```

**策略3：首帧继承**
```
上一段尾帧：女孩站在小屋门口
当前场景：女孩进入小屋

↓ 首帧约束

生成参数：
{
  prompt: "...",
  imageUrl: "previous_end_frame.jpg", // 即梦的首帧参考
  videos: []  // 可选：视频参考
}
```

---

### 3.3 首尾帧管理模块

**目标**：实现系列视频连贯性

#### 数据模型
```go
type VideoFrame struct {
    ID          string    // 帧ID
    VideoID     string    // 来源视频ID
    FrameType   string    // "start" | "end" | "keyframe"
    Timestamp   float64   // 视频中的时间戳（秒）
    ImageURL    string    // 帧图片URL
    AssetID     string    // 存储在资产库的ID
    Tags        []string  // 标签：场景、人物、风格
    CreateTime  time.Time
}
```

#### 功能特性

1. **自动抽帧**
   - 生成完成后自动提取首帧（0.1s）和尾帧（倒数0.1s）
   - 存储到首尾帧库

2. **智能推荐**
   - 创建新场景时，推荐相似风格的尾帧作为首帧
   - 基于标签匹配（场景类型、角色、风格）

3. **手动管理**
   - 支持手动上传首尾帧
   - 支持从视频任意时间点截帧

4. **连贯性评分**
   ```typescript
   function calculateContinuityScore(
     endFrame: VideoFrame,
     startFrame: VideoFrame
   ): number {
     // 评估两帧的：
     // 1. 色调相似度
     // 2. 场景连续性
     // 3. 角色位置一致性
     return score; // 0-100
   }
   ```

---

### 3.4 批量生成模块

**目标**：从剧本一键生成多段视频

#### 工作流

```
┌────────────┐
│ 选择剧本   │
└─────┬──────┘
      ↓
┌────────────┐
│ 资产检查   │ ← 检查所有场景是否绑定资产
└─────┬──────┘
      ↓
┌────────────┐
│ 生成提示词 │ ← 每个场景生成优化后的提示词
└─────┬──────┘
      ↓
┌────────────┐
│ 首尾帧处理 │ ← 自动继承上一场景尾帧
└─────┬──────┘
      ↓
┌────────────┐
│ 批量提交   │ ← 按顺序提交到即梦API
└─────┬──────┘
      ↓
┌────────────┐
│ 进度监控   │ ← 实时显示每段生成状态
└────────────┘
```

#### 批量任务管理
```go
type BatchGenerationTask struct {
    ID          string           // 批量任务ID
    ScriptID    string           // 剧本ID
    Scenes      []SceneTask      // 场景任务列表
    Status      string           // queued/running/completed/failed
    Progress    int              // 完成百分比
    CreateTime  time.Time
}

type SceneTask struct {
    SceneID      string          // 场景ID
    GenerationID string          // 视频生成任务ID
    Order        int             // 顺序
    Status       string          // pending/queued/running/completed/failed
    Prompt       string          // 生成的提示词
    StartFrame   string          // 使用的首帧
    VideoURL     string          // 生成结果
}
```

---

## 四、数据库设计

### 新增表

```sql
-- 剧本表
CREATE TABLE video_scripts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(200) NOT NULL,
    theme TEXT,
    duration INTEGER DEFAULT 0,
    style_asset_id UUID REFERENCES video_assets(id),
    status VARCHAR(20) DEFAULT 'draft',
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 场景表
CREATE TABLE video_scenes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    script_id UUID REFERENCES video_scripts(id) ON DELETE CASCADE,
    order_num INTEGER NOT NULL,
    duration INTEGER DEFAULT 15,
    description TEXT,
    action TEXT,
    mood VARCHAR(50),
    location_asset_id UUID REFERENCES video_assets(id),
    start_frame_url TEXT,
    end_frame_url TEXT,
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 场景-资产关联表（人物、道具）
CREATE TABLE video_scene_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scene_id UUID REFERENCES video_scenes(id) ON DELETE CASCADE,
    asset_id UUID REFERENCES video_assets(id),
    asset_role VARCHAR(20), -- 'character' | 'prop'
    order_num INTEGER DEFAULT 0
);

-- 首尾帧库
CREATE TABLE video_frames (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id UUID REFERENCES video_generations(id),
    frame_type VARCHAR(20), -- 'start' | 'end' | 'keyframe'
    timestamp FLOAT,
    image_url TEXT NOT NULL,
    asset_id UUID REFERENCES video_assets(id),
    tags TEXT[], -- 标签数组
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 批量生成任务
CREATE TABLE video_batch_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    script_id UUID REFERENCES video_scripts(id),
    status VARCHAR(20) DEFAULT 'queued',
    progress INTEGER DEFAULT 0,
    total_scenes INTEGER DEFAULT 0,
    completed_scenes INTEGER DEFAULT 0,
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 批量任务-场景关联
CREATE TABLE video_batch_scene_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_task_id UUID REFERENCES video_batch_tasks(id) ON DELETE CASCADE,
    scene_id UUID REFERENCES video_scenes(id),
    generation_id UUID REFERENCES video_generations(id),
    order_num INTEGER NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    prompt TEXT,
    start_frame_url TEXT,
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

---

## 五、前端页面设计

### 5.1 工作台主界面 (workbench.vue)

#### 布局
```
┌─────────────────────────────────────────────────────────────┐
│ 🎬 视频生成工作台                                 [新建剧本]  │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  剧本列表                          当前剧本：春日森林冒险     │
│  ┌──────────────┐                                            │
│  │ 春日森林冒险  │  ───────►    ┌──────────────────────┐   │
│  │ 3场景 未生成 │              │ 场景1: 森林入口       │   │
│  └──────────────┘              │ 📍场景: 森林小径      │   │
│  ┌──────────────┐              │ 👤人物: 女孩         │   │
│  │ 城市夜景系列  │              │ ⏱ 15秒              │   │
│  │ 5场景 已生成 │              │ [编辑资产] [预览]    │   │
│  └──────────────┘              └──────────────────────┘   │
│                                                              │
│                                 ┌──────────────────────┐   │
│                                 │ 场景2: 小屋前        │   │
│                                 │ 📍场景: 森林小屋     │   │
│                                 │ 👤人物: 女孩         │   │
│                                 │ 🔗首帧: ← 场景1尾帧  │   │
│                                 │ [编辑资产] [预览]    │   │
│                                 └──────────────────────┘   │
│                                                              │
│  [智能生成提示词] [批量生成视频] [导出合集]                │
└─────────────────────────────────────────────────────────────┘
```

#### 核心交互

1. **拖拽式资产绑定**
   - 从右侧资产库面板拖拽到场景卡片
   - 自动分类（场景/人物/道具）

2. **智能提示词预览**
   - 点击"智能生成提示词"
   - 弹窗展示每个场景的完整提示词
   - 支持手动微调

3. **首尾帧可视化**
   - 场景卡片显示首帧缩略图
   - 自动标注"继承自上一场景"

---

### 5.2 资产组装面板 (SceneAssetComposer.vue)

#### 组件结构
```vue
<template>
  <Modal title="场景资产组装" width="1200px">
    <Row :gutter="16">
      <!-- 左侧：场景信息 -->
      <Col :span="10">
        <Card title="场景描述">
          <FormItem label="场景描述">
            <TextArea v-model="scene.description" />
          </FormItem>
          <FormItem label="动作">
            <Input v-model="scene.action" />
          </FormItem>
          <FormItem label="情绪">
            <Select v-model="scene.mood">
              <Option value="peaceful">平静</Option>
              <Option value="tense">紧张</Option>
              <Option value="joyful">欢乐</Option>
            </Select>
          </FormItem>
        </Card>

        <!-- 生成的提示词预览 -->
        <Card title="生成的提示词" class="mt-4">
          <Alert type="info">
            {{ generatedPrompt }}
          </Alert>
          <Button @click="copyPrompt">复制提示词</Button>
        </Card>
      </Col>

      <!-- 右侧：资产选择 -->
      <Col :span="14">
        <Tabs>
          <TabPane tab="📍 场景" key="scene">
            <AssetGrid 
              type="scene" 
              :selected="scene.locationAssetId"
              @select="onSelectAsset('location', $event)"
            />
          </TabPane>
          <TabPane tab="👤 人物" key="character">
            <AssetGrid 
              type="character" 
              :selected="scene.characters"
              multiple
              @select="onSelectAsset('characters', $event)"
            />
          </TabPane>
          <TabPane tab="🎨 风格" key="style">
            <AssetGrid 
              type="style" 
              :selected="scene.styleAssetId"
              @select="onSelectAsset('style', $event)"
            />
          </TabPane>
        </Tabs>
      </Col>
    </Row>

    <!-- 首尾帧管理 -->
    <Divider>首尾帧设置</Divider>
    <Row :gutter="16">
      <Col :span="12">
        <Card title="首帧">
          <Radio.Group v-model="startFrameMode">
            <Radio value="auto">自动继承上一场景</Radio>
            <Radio value="library">从首尾帧库选择</Radio>
            <Radio value="upload">上传图片</Radio>
          </Radio.Group>
          <Image v-if="scene.startFrame" :src="scene.startFrame" />
        </Card>
      </Col>
      <Col :span="12">
        <Card title="尾帧（可选）">
          <Upload @change="onUploadEndFrame">
            <Button>上传预期尾帧</Button>
          </Upload>
        </Card>
      </Col>
    </Row>
  </Modal>
</template>
```

---

### 5.3 批量生成监控 (BatchGenerationMonitor.vue)

#### UI设计
```
批量生成进度
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 60% (3/5)

场景1: 森林入口        ✅ 已完成  [预览视频]
场景2: 小屋前          🎬 生成中  进度: 80%
场景3: 小屋内          ⏳ 排队中
场景4: 后院花园        ⏳ 排队中
场景5: 日落时分        ⏳ 排队中

预计剩余时间: 8分钟
[暂停] [取消] [导出已完成]
```

---

## 六、提示词优化策略（降低抽卡率）

### 6.1 即梦 Prompt 最佳实践

```typescript
class JiMengPromptOptimizer {
  // 基础模板
  static readonly TEMPLATE = `
    {主体描述}, {动作}, {场景描述}, 
    {风格标签}, {光照}, {机位}, {情绪氛围}
  `;

  // 质量增强词
  static readonly QUALITY_BOOSTERS = [
    "high quality",
    "detailed",
    "cinematic lighting",
    "professional",
  ];

  // 禁用词（容易导致失败）
  static readonly BLACKLIST = [
    "nsfw", "gore", "violence", 
    "realistic photo", // 即梦更擅长动画风格
  ];

  optimize(assets: AssetCollection, scene: Scene): string {
    let prompt = "";

    // 1. 主体（人物优先）
    if (assets.characters.length > 0) {
      prompt += this.describeCharacters(assets.characters);
    }

    // 2. 动作
    if (scene.action) {
      prompt += `, ${scene.action}`;
    }

    // 3. 场景
    if (assets.scene) {
      prompt += `, ${assets.scene.remark}`;
    }

    // 4. 风格
    if (assets.style) {
      prompt += `, ${assets.style.remark}`;
    }

    // 5. 增强细节
    prompt += `, ${this.getEnhancements(scene)}`;

    // 6. 过滤禁用词
    prompt = this.removeBlacklist(prompt);

    return prompt;
  }

  private getEnhancements(scene: Scene): string {
    const enhancements = [];

    // 光照
    if (scene.mood === "peaceful") {
      enhancements.push("soft natural lighting");
    } else if (scene.mood === "tense") {
      enhancements.push("dramatic lighting");
    }

    // 机位
    enhancements.push("medium shot"); // 中景通常效果最稳定

    // 运镜
    enhancements.push("smooth camera movement");

    return enhancements.join(", ");
  }
}
```

### 6.2 资产库标准化建议

为了让资产更好地服务于提示词生成，建议资产备注字段遵循以下格式：

**场景资产示例**
```json
{
  "name": "森林小屋",
  "type": "scene",
  "remark": "a cozy wooden cabin surrounded by lush green forest, moss-covered stones, wildflowers blooming nearby"
}
```

**人物资产示例**
```json
{
  "name": "春日少女",
  "type": "character",
  "remark": "a young girl with long flowing hair, wearing a light blue dress with floral patterns, gentle smile"
}
```

**风格资产示例**
```json
{
  "name": "宫崎骏风格",
  "type": "style",
  "remark": "Studio Ghibli animation style, soft colors, hand-drawn aesthetic, whimsical atmosphere"
}
```

---

## 七、实施路线图

### Phase 1: 基础架构（1-2周）
- [ ] 数据库表创建
- [ ] Go后端API
  - [ ] 剧本CRUD
  - [ ] 场景CRUD
  - [ ] 场景-资产关联API
- [ ] 前端工作台页面框架

### Phase 2: 智能提示词引擎（1周）
- [ ] 提示词模板引擎
- [ ] 资产语义提取
- [ ] 多资产融合算法
- [ ] 提示词优化器

### Phase 3: 首尾帧管理（1周）
- [ ] 视频抽帧功能
- [ ] 首尾帧库
- [ ] 首帧继承逻辑
- [ ] 连贯性评分

### Phase 4: 批量生成（1周）
- [ ] 批量任务管理
- [ ] 队列调度
- [ ] 进度监控
- [ ] 失败重试机制

### Phase 5: 优化迭代（持续）
- [ ] 提示词效果跟踪
- [ ] 抽卡率统计
- [ ] A/B测试不同策略
- [ ] 用户反馈收集

---

## 八、预期效果

### 降低抽卡率
- **现状**：手动提示词，抽卡率约 40-60%（需重新生成）
- **目标**：智能提示词 + 资产约束，抽卡率降至 15-25%

### 提升效率
- **现状**：单段视频生成需 5-10 分钟人工操作
- **目标**：批量生成5段视频，人工操作时间 < 15 分钟（80% 自动化）

### 系列视频连贯性
- **现状**：无法保证视频间风格一致
- **目标**：首尾帧继承 + 风格锁定，连贯性提升 70%

---

## 九、技术风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 即梦API限流 | 批量生成失败 | 队列速率控制，重试机制 |
| 首帧参考效果不佳 | 连贯性不足 | 提供多种首帧模式（自动/手动） |
| 提示词优化不准确 | 抽卡率仍高 | 支持手动微调，建立效果反馈机制 |
| 资产库质量参差不齐 | 生成效果差 | 资产审核机制，标准化模板 |

---

## 十、后续扩展方向

1. **AI剧本生成** - 输入主题，全自动生成完整剧本+资产推荐
2. **风格迁移** - 一键将现有剧本转换为不同风格（如：写实→动漫）
3. **视频自动剪辑** - 批量生成后自动合成完整视频
4. **提示词学习** - 基于历史数据训练提示词优化模型
5. **协作模式** - 多人协同编辑剧本和资产

---

**总结**：这套方案将零散的资产库、分镜、生成功能整合为一个智能化工作台，通过结构化的剧本管理、智能提示词生成、首尾帧继承三大核心能力，有效降低抽卡率，提升视频生成效率和连贯性。
