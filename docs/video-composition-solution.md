# 视频智能剪辑成片方案

## 一、需求分析

**目标**：将项目中生成的多个分镜视频，自动剪辑合成为一个完整视频

**输入**：
- 项目的所有 Shot（按 order_num 排序）
- 每个 Shot 对应一个已生成的视频文件

**输出**：
- 一个完整的合成视频
- 可选：添加转场效果、背景音乐、字幕

---

## 二、技术方案对比

### 方案1：FFmpeg 命令行（推荐 ⭐⭐⭐⭐⭐）

**优点**：
- ✅ 免费开源，无额外成本
- ✅ 性能极高，服务器端处理
- ✅ 功能强大：转场、滤镜、音频混合
- ✅ 你已经在用（提取首尾帧需要 FFmpeg）

**缺点**：
- ❌ 需要学习 FFmpeg 命令

**适用场景**：
- 简单拼接：直接合并视频
- 进阶剪辑：添加转场、音乐、字幕

---

### 方案2：云端视频剪辑 API

**选项**：
- 阿里云视频编辑 API
- 腾讯云视频剪辑 API
- Cloudinary Video API

**优点**：
- ✅ API 调用简单
- ✅ 云端处理，不占服务器资源

**缺点**：
- ❌ 按调用次数收费（成本较高）
- ❌ 需要上传/下载视频文件

---

### 方案3：前端 WebCodecs / FFmpeg.wasm

**优点**：
- ✅ 纯前端处理，不占服务器资源

**缺点**：
- ❌ 性能差，大文件会卡顿
- ❌ 浏览器兼容性问题

---

## 三、推荐方案：FFmpeg 服务端剪辑

### 3.1 核心功能设计

#### 功能1：简单拼接（基础版）

```go
// 将项目的所有 Shot 视频按顺序拼接
func (s *VideoComposer) ComposeProject(projectID string) (string, error) {
    // 1. 获取项目的所有已完成的 Shot（按 order_num 排序）
    shots := s.getCompletedShots(projectID)
    
    // 2. 下载所有视频文件到临时目录
    videoFiles := []string{}
    for _, shot := range shots {
        localPath := s.downloadVideo(shot.VideoURL)
        videoFiles = append(videoFiles, localPath)
    }
    
    // 3. 生成 FFmpeg concat 文件
    concatFile := s.generateConcatFile(videoFiles)
    
    // 4. 执行 FFmpeg 拼接
    outputFile := fmt.Sprintf("/tmp/project_%s_output.mp4", projectID)
    cmd := exec.Command("ffmpeg",
        "-f", "concat",
        "-safe", "0",
        "-i", concatFile,
        "-c", "copy",  // 直接复制流，最快
        outputFile,
    )
    if err := cmd.Run(); err != nil {
        return "", err
    }
    
    // 5. 上传到 OSS
    finalURL := s.uploadToOSS(outputFile)
    
    // 6. 保存到数据库
    s.db.Exec(`
        UPDATE video_projects 
        SET final_video_url = $1, status = 'completed'
        WHERE id = $2
    `, finalURL, projectID)
    
    return finalURL, nil
}

// 生成 FFmpeg concat 文件
func (s *VideoComposer) generateConcatFile(videoFiles []string) string {
    content := ""
    for _, file := range videoFiles {
        content += fmt.Sprintf("file '%s'\n", file)
    }
    
    concatPath := "/tmp/concat_list.txt"
    os.WriteFile(concatPath, []byte(content), 0644)
    return concatPath
}
```

**FFmpeg 命令示例**：
```bash
# concat_list.txt 内容：
# file '/tmp/shot_1.mp4'
# file '/tmp/shot_2.mp4'
# file '/tmp/shot_3.mp4'

ffmpeg -f concat -safe 0 -i concat_list.txt -c copy output.mp4
```

**速度**：3个15秒视频拼接 < 5秒

---

#### 功能2：添加转场效果（进阶版）

**常用转场**：
- 淡入淡出（fade）
- 交叉溶解（xfade）
- 擦除（wipe）
- 缩放（zoom）

```go
// 添加转场效果的拼接
func (s *VideoComposer) ComposeWithTransitions(projectID string, transitionType string) (string, error) {
    shots := s.getCompletedShots(projectID)
    
    // FFmpeg xfade 滤镜实现转场
    filterComplex := s.buildTransitionFilter(shots, transitionType)
    
    cmd := exec.Command("ffmpeg",
        // 输入所有视频
        "-i", shots[0].VideoPath,
        "-i", shots[1].VideoPath,
        "-i", shots[2].VideoPath,
        // 应用转场滤镜
        "-filter_complex", filterComplex,
        // 输出
        outputFile,
    )
    
    cmd.Run()
    return s.uploadToOSS(outputFile), nil
}

// 构建转场滤镜
func (s *VideoComposer) buildTransitionFilter(shots []Shot, transType string) string {
    // 示例：交叉溶解转场（1秒）
    filter := ""
    for i := 0; i < len(shots)-1; i++ {
        offset := shots[i].Duration - 1.0  // 在倒数1秒开始转场
        filter += fmt.Sprintf(
            "[%d:v][%d:v]xfade=transition=%s:duration=1:offset=%.1f[v%d];",
            i, i+1, transType, offset, i,
        )
    }
    return filter
}
```

**FFmpeg 转场命令示例**：
```bash
ffmpeg \
  -i shot1.mp4 -i shot2.mp4 -i shot3.mp4 \
  -filter_complex "\
    [0:v][1:v]xfade=transition=fade:duration=1:offset=14[v01];\
    [v01][2:v]xfade=transition=fade:duration=1:offset=28[vout]" \
  -map "[vout]" output.mp4
```

---

#### 功能3：添加背景音乐（进阶版）

```go
func (s *VideoComposer) ComposeWithMusic(projectID string, musicURL string) (string, error) {
    // 1. 拼接视频（无音频）
    videoFile := s.composeVideoOnly(projectID)
    
    // 2. 下载音乐文件
    musicFile := s.downloadAudio(musicURL)
    
    // 3. 获取视频总时长
    duration := s.getVideoDuration(videoFile)
    
    // 4. FFmpeg 添加背景音乐
    cmd := exec.Command("ffmpeg",
        "-i", videoFile,
        "-i", musicFile,
        "-t", fmt.Sprintf("%.1f", duration),  // 裁剪音乐到视频时长
        "-c:v", "copy",  // 视频流直接复制
        "-c:a", "aac",   // 音频重新编码
        "-b:a", "128k",
        "-map", "0:v:0", // 使用第一个输入的视频
        "-map", "1:a:0", // 使用第二个输入的音频
        outputFile,
    )
    
    cmd.Run()
    return s.uploadToOSS(outputFile), nil
}
```

**FFmpeg 命令示例**：
```bash
ffmpeg -i video.mp4 -i music.mp3 \
  -t 45 \  # 音乐裁剪到45秒
  -c:v copy -c:a aac -b:a 128k \
  -map 0:v:0 -map 1:a:0 \
  output.mp4
```

---

#### 功能4：添加字幕（可选）

```go
// 从 Shot 的 dialogue 字段生成字幕文件
func (s *VideoComposer) ComposeWithSubtitles(projectID string) (string, error) {
    shots := s.getCompletedShots(projectID)
    
    // 1. 生成 SRT 字幕文件
    srtFile := s.generateSRT(shots)
    
    // 2. 拼接视频
    videoFile := s.composeVideoOnly(projectID)
    
    // 3. FFmpeg 烧录字幕
    cmd := exec.Command("ffmpeg",
        "-i", videoFile,
        "-vf", fmt.Sprintf("subtitles=%s:force_style='FontSize=24,PrimaryColour=&HFFFFFF'", srtFile),
        outputFile,
    )
    
    cmd.Run()
    return s.uploadToOSS(outputFile), nil
}

// 生成 SRT 字幕文件
func (s *VideoComposer) generateSRT(shots []Shot) string {
    srt := ""
    startTime := 0.0
    
    for i, shot := range shots {
        if shot.Dialogue == "" {
            continue
        }
        
        endTime := startTime + shot.Duration
        
        srt += fmt.Sprintf("%d\n", i+1)
        srt += fmt.Sprintf("%s --> %s\n", 
            formatSRTTime(startTime), 
            formatSRTTime(endTime),
        )
        srt += fmt.Sprintf("%s\n\n", shot.Dialogue)
        
        startTime = endTime
    }
    
    srtPath := "/tmp/subtitles.srt"
    os.WriteFile(srtPath, []byte(srt), 0644)
    return srtPath
}

func formatSRTTime(seconds float64) string {
    hours := int(seconds / 3600)
    minutes := int((seconds - float64(hours*3600)) / 60)
    secs := int(seconds) % 60
    millis := int((seconds - float64(int(seconds))) * 1000)
    return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, secs, millis)
}
```

**SRT 字幕格式示例**：
```
1
00:00:00,000 --> 00:00:15,000
女孩走进森林，好奇地四处张望

2
00:00:15,000 --> 00:00:30,000
她发现了一座小屋，眼睛一亮
```

---

## 四、数据库设计

```sql
-- 项目表新增字段
ALTER TABLE video_projects ADD COLUMN final_video_url TEXT;
ALTER TABLE video_projects ADD COLUMN final_video_asset_id UUID REFERENCES upload_assets(id);
ALTER TABLE video_projects ADD COLUMN compose_status VARCHAR(20) DEFAULT 'pending';
-- pending/composing/completed/failed

-- 合成任务表（可选，用于异步处理）
CREATE TABLE video_compose_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES video_projects(id) ON DELETE CASCADE,
    status VARCHAR(20) DEFAULT 'queued',  -- queued/processing/completed/failed
    options JSONB,  -- 合成选项：转场类型、音乐URL等
    final_video_url TEXT,
    error_message TEXT,
    create_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

---

## 五、前端交互设计

### 5.1 项目详情页新增"合成视频"区域

```vue
<template>
  <Card title="成片合成" class="mt-4">
    <Alert v-if="!allShotsCompleted" type="warning" show-icon>
      <template #message>
        还有 {{ pendingShotsCount }} 个分镜未完成生成，请等待全部完成后再合成
      </template>
    </Alert>
    
    <Form v-else layout="vertical">
      <FormItem label="合成选项">
        <Space direction="vertical" style="width: 100%">
          <!-- 转场效果 -->
          <Card size="small" title="转场效果">
            <Radio.Group v-model="composeOptions.transition">
              <Radio value="none">无转场（直接拼接）</Radio>
              <Radio value="fade">淡入淡出</Radio>
              <Radio value="wipeleft">左侧擦除</Radio>
              <Radio value="circleopen">圆形展开</Radio>
            </Radio.Group>
          </Card>
          
          <!-- 背景音乐 -->
          <Card size="small" title="背景音乐">
            <Radio.Group v-model="composeOptions.musicMode">
              <Radio value="none">无背景音乐</Radio>
              <Radio value="library">从资产库选择</Radio>
              <Radio value="upload">上传音乐</Radio>
            </Radio.Group>
            
            <div v-if="composeOptions.musicMode === 'library'" class="mt-2">
              <Button @click="openMusicPicker">选择音乐</Button>
              <span v-if="selectedMusic" class="ml-2">
                已选：{{ selectedMusic.name }}
              </span>
            </div>
            
            <Upload 
              v-if="composeOptions.musicMode === 'upload'"
              accept="audio/*"
              :before-upload="handleMusicUpload"
              class="mt-2"
            >
              <Button>上传音乐文件</Button>
            </Upload>
          </Card>
          
          <!-- 字幕 -->
          <Card size="small" title="字幕">
            <Checkbox v-model:checked="composeOptions.enableSubtitles">
              添加字幕（基于分镜的对白字段）
            </Checkbox>
          </Card>
        </Space>
      </FormItem>
      
      <FormItem>
        <Space>
          <Button 
            type="primary" 
            size="large"
            :loading="composing"
            @click="startCompose"
          >
            <template #icon>
              <IconifyIcon icon="lucide:film" />
            </template>
            开始合成成片
          </Button>
          
          <Tooltip title="预计处理时间：约 2-5 分钟">
            <Button type="link" size="small">
              <IconifyIcon icon="lucide:info" />
            </Button>
          </Tooltip>
        </Space>
      </FormItem>
    </Form>
    
    <!-- 合成进度 -->
    <div v-if="composeJob" class="compose-progress">
      <Divider>合成进度</Divider>
      <Progress 
        :percent="composeProgress" 
        :status="composeJob.status === 'failed' ? 'exception' : 'active'"
      />
      <p class="mt-2">
        {{ composeStatusText }}
      </p>
      
      <!-- 成功后显示下载 -->
      <div v-if="composeJob.status === 'completed'" class="mt-4">
        <Alert type="success" show-icon>
          <template #message>
            成片合成完成！
          </template>
        </Alert>
        <Space class="mt-2">
          <Button type="primary" @click="previewFinalVideo">
            <template #icon>
              <IconifyIcon icon="lucide:play" />
            </template>
            预览成片
          </Button>
          <Button @click="downloadFinalVideo">
            <template #icon>
              <IconifyIcon icon="lucide:download" />
            </template>
            下载成片
          </Button>
          <Button @click="shareFinalVideo">
            <template #icon>
              <IconifyIcon icon="lucide:share-2" />
            </template>
            分享链接
          </Button>
        </Space>
      </div>
    </div>
  </Card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { message } from 'ant-design-vue';

const composing = ref(false);
const composeJob = ref(null);

const composeOptions = ref({
  transition: 'fade',
  musicMode: 'none',
  musicUrl: '',
  enableSubtitles: false,
});

const composeProgress = computed(() => {
  if (!composeJob.value) return 0;
  if (composeJob.value.status === 'completed') return 100;
  if (composeJob.value.status === 'processing') return 50;
  return 0;
});

const composeStatusText = computed(() => {
  if (!composeJob.value) return '';
  const statusMap = {
    queued: '等待处理中...',
    processing: '正在合成视频，请稍候...',
    completed: '合成完成！',
    failed: '合成失败：' + composeJob.value.errorMessage,
  };
  return statusMap[composeJob.value.status] || '';
});

async function startCompose() {
  composing.value = true;
  try {
    // 调用后端 API
    const result = await composeProjectApi(props.projectId, {
      transition: composeOptions.value.transition,
      musicUrl: composeOptions.value.musicUrl,
      enableSubtitles: composeOptions.value.enableSubtitles,
    });
    
    composeJob.value = result;
    message.success('合成任务已提交，请稍候...');
    
    // 轮询检查进度
    pollComposeStatus();
  } catch (error: any) {
    message.error(error.message || '合成失败');
  } finally {
    composing.value = false;
  }
}

function pollComposeStatus() {
  const timer = setInterval(async () => {
    try {
      const status = await getComposeJobStatusApi(composeJob.value.id);
      composeJob.value = status;
      
      if (status.status === 'completed' || status.status === 'failed') {
        clearInterval(timer);
      }
    } catch (error) {
      clearInterval(timer);
    }
  }, 3000);  // 每3秒轮询一次
}
</script>
```

---

## 六、后端 API 设计

```go
// 路由
POST   /api/video/projects/:id/compose     - 提交合成任务
GET    /api/video/compose-jobs/:jobId      - 查询合成进度

// 实现
func (s *Server) composeProject(w http.ResponseWriter, r *http.Request) {
    projectID := getPathParam(r, "id")
    
    var input struct {
        Transition       string `json:"transition"`
        MusicURL         string `json:"musicUrl"`
        EnableSubtitles  bool   `json:"enableSubtitles"`
    }
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        httpx.Fail(w, 400, "参数错误")
        return
    }
    
    // 创建合成任务
    jobID := uuid.New().String()
    s.db.Exec(`
        INSERT INTO video_compose_jobs (id, project_id, status, options)
        VALUES ($1, $2, 'queued', $3)
    `, jobID, projectID, toJSON(input))
    
    // 异步执行合成
    go s.executeCompose(jobID, projectID, input)
    
    httpx.OK(w, map[string]string{"jobId": jobID, "status": "queued"})
}

func (s *Server) executeCompose(jobID, projectID string, options ComposeOptions) {
    // 更新状态为 processing
    s.db.Exec(`UPDATE video_compose_jobs SET status='processing' WHERE id=$1`, jobID)
    
    composer := NewVideoComposer(s.db, s.videoStore())
    
    var finalURL string
    var err error
    
    // 根据选项执行不同的合成策略
    if options.MusicURL != "" {
        finalURL, err = composer.ComposeWithMusic(projectID, options.MusicURL)
    } else if options.Transition != "none" {
        finalURL, err = composer.ComposeWithTransitions(projectID, options.Transition)
    } else {
        finalURL, err = composer.ComposeProject(projectID)
    }
    
    if err != nil {
        s.db.Exec(`
            UPDATE video_compose_jobs 
            SET status='failed', error_message=$1, update_time=now()
            WHERE id=$2
        `, err.Error(), jobID)
        return
    }
    
    // 更新为完成状态
    s.db.Exec(`
        UPDATE video_compose_jobs 
        SET status='completed', final_video_url=$1, update_time=now()
        WHERE id=$2
    `, finalURL, jobID)
    
    s.db.Exec(`
        UPDATE video_projects 
        SET final_video_url=$1, compose_status='completed'
        WHERE id=$2
    `, finalURL, projectID)
}
```

---

## 七、实施步骤

### Step 1: 安装 FFmpeg（如果未安装）
```bash
# Ubuntu/Debian
apt-get install ffmpeg

# macOS
brew install ffmpeg

# 验证安装
ffmpeg -version
```

### Step 2: 实现基础拼接功能（2-3小时）
- [ ] VideoComposer 基础类
- [ ] 简单拼接（无转场）
- [ ] API: POST /api/video/projects/:id/compose

### Step 3: 前端合成界面（2小时）
- [ ] 项目详情页添加"合成"区域
- [ ] 进度轮询
- [ ] 预览和下载

### Step 4: 进阶功能（可选，各1-2小时）
- [ ] 转场效果
- [ ] 背景音乐
- [ ] 字幕烧录

---

## 八、成本和性能

### 性能指标
| 操作 | 视频数量 | 总时长 | 处理时间 |
|------|---------|--------|---------|
| 简单拼接 | 3 | 45秒 | < 5秒 |
| 转场拼接 | 3 | 45秒 | 15-30秒 |
| 添加音乐 | 3 | 45秒 | 10-20秒 |
| 烧录字幕 | 3 | 45秒 | 20-40秒 |

### 成本
- ✅ **FFmpeg 方案**：完全免费，仅占用服务器 CPU
- ❌ **云端 API**：按次收费，约 ¥0.1-0.5/次

---

## 九、总结

**推荐实施方案**：

1. **第一阶段（必须）**：基础拼接
   - 纯 FFmpeg concat
   - 速度快，成本零
   - 满足 80% 需求

2. **第二阶段（可选）**：转场效果
   - FFmpeg xfade 滤镜
   - 提升视觉体验

3. **第三阶段（可选）**：音乐和字幕
   - 根据用户反馈决定是否开发

**核心优势**：
- ✅ 服务器端处理，速度快
- ✅ 完全自动化，无需人工干预
- ✅ 成本为零（使用开源工具）
- ✅ 可扩展性强（支持各种高级功能）

---

**你觉得这个方案如何？我建议先实现基础拼接功能，然后根据实际需求逐步添加转场和音乐功能。**
