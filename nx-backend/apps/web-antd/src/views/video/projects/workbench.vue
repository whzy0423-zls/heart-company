<template>
  <div class="workbench-container">
    <!-- 顶部工具栏 -->
    <div class="workbench-header">
      <a-breadcrumb>
        <a-breadcrumb-item>
          <router-link to="/video/projects">视频项目</router-link>
        </a-breadcrumb-item>
        <a-breadcrumb-item>{{ project?.name || '加载中...' }}</a-breadcrumb-item>
      </a-breadcrumb>

      <div class="header-actions">
        <a-button @click="showProjectSettings">项目设置</a-button>
        <a-space>
          <a-button
            type="primary"
            @click="composeVideo"
            :loading="composing"
            :disabled="shotMutationBusy || hasOpenShotInlineForm"
          >
            合成视频
          </a-button>
          <span v-if="composing && composeProgress > 0" class="progress-text">
            {{ composeProgress }}%
          </span>
        </a-space>
      </div>
    </div>

    <!-- 项目制五步生产流：参考旧项目 episode.vue 的轻量 step-bar，不再挤压下方工作台 -->
    <section class="production-flow-panel compact-production-flow">
      <div class="flow-title">
        <div class="workbench-mode-switch" aria-label="制片工作台模式">
          <button
            v-for="option in workbenchModeOptions"
            :key="option.value"
            type="button"
            :class="['mode-switch-item', workbenchMode === option.value ? 'is-active' : '']"
            @click="workbenchMode = option.value"
          >
            {{ option.label }}
          </button>
        </div>
        <a-tag color="blue">{{ workbenchMode === 'project' ? '项目制生产流' : '短片制工作台占位' }}</a-tag>
        <div class="flow-title-copy">
          <h2>制片工作台流程</h2>
          <p>
            {{
              workbenchMode === 'project'
                ? activeProductionStepMeta?.description
                : '短片制能力正在接入：后续会在这里承载单条短片快速生成、素材选择和版本管理。'
            }}
          </p>
        </div>
      </div>

      <div v-if="workbenchMode === 'project'" class="step-bar" aria-label="项目制生产流程">
        <button
          v-for="(step, index) in productionSteps"
          :key="step.key"
          type="button"
          :class="[
            'step-item',
            step.key === activeProductionStep ? 'is-active' : '',
            index < currentProductionStepIndex ? 'is-done' : '',
          ]"
          @click="setProductionStep(index)"
        >
          <span class="step-index">{{ index + 1 }}</span>
          <span class="step-label">{{ step.title }}</span>
        </button>
      </div>

      <div v-else class="short-film-mode-placeholder">
        <strong>短片制能力正在接入</strong>
        <span>当前先保留入口，占位后续短片制的一镜到底/轻量分镜/快速成片流程。</span>
      </div>

      <div v-if="workbenchMode === 'project'" class="flow-actions">
        <span class="flow-stat">
          角色 {{ characters.length }} · 场景 {{ scenes.length }} · 分镜 {{ shots.length }} · 已完成
          {{ completedShotCount }}
        </span>
        <span class="bucket-pill">资产统一上传到阿里云 OSS 文件桶</span>
        <a-button size="small" :disabled="productionSecondaryDisabled" @click="handleProductionSecondaryAction">
          {{ productionSecondaryActionLabel }}
        </a-button>
        <a-button
          size="small"
          type="primary"
          :loading="productionPrimaryLoading"
          :disabled="productionPrimaryDisabled"
          @click="handleProductionPrimaryAction"
        >
          {{ productionPrimaryActionLabel }}
        </a-button>
      </div>
    </section>

    <!-- 主工作区 -->
    <a-layout
      v-if="workbenchMode === 'project'"
      class="workbench-layout"
      @wheel.passive="handleWorkbenchWheel"
    >
      <!-- 左侧：角色和场景库 -->
      <a-layout-sider width="300" theme="light" class="workbench-sider">
        <a-tabs v-model:activeKey="leftTab">
          <a-tab-pane key="characters" tab="角色">
            <div class="resource-panel">
              <a-button type="dashed" block @click="showAddCharacter">
                <PlusOutlined /> 添加角色
              </a-button>

              <div class="resource-list">
                <div
                  v-for="char in characters"
                  :key="char.id"
                  class="resource-item"
                  draggable="true"
                  @dragstart="onDragStart($event, 'character', char)"
                >
                  <div class="resource-preview">
                    <img
                      v-if="previewReferenceAsset(char.referenceImageUrl)"
                      :src="previewReferenceAsset(char.referenceImageUrl)"
                      alt="角色参考图"
                    />
                    <UserOutlined v-else />
                  </div>
                  <div class="resource-info">
                    <div class="resource-name">
                      {{ char.name }}
                      <a-tag v-if="char.isMain" color="blue" size="small">主角</a-tag>
                    </div>
                    <div class="resource-desc">{{ char.description }}</div>
                  </div>
                  <div class="resource-actions">
                    <EditOutlined @click="editCharacter(char)" />
                    <DeleteOutlined @click="deleteCharacter(char.id)" />
                  </div>
                </div>
              </div>
            </div>
          </a-tab-pane>

          <a-tab-pane key="scenes" tab="场景">
            <div class="resource-panel">
              <a-button type="dashed" block @click="showAddScene">
                <PlusOutlined /> 添加场景
              </a-button>

              <div class="resource-list">
                <div
                  v-for="scene in scenes"
                  :key="scene.id"
                  class="resource-item"
                  draggable="true"
                  @dragstart="onDragStart($event, 'scene', scene)"
                >
                  <div class="resource-preview">
                    <img
                      v-if="previewReferenceAsset(scene.referenceImageUrl)"
                      :src="previewReferenceAsset(scene.referenceImageUrl)"
                      alt="场景参考图"
                    />
                    <EnvironmentOutlined v-else />
                  </div>
                  <div class="resource-info">
                    <div class="resource-name">{{ scene.name }}</div>
                    <div class="resource-desc">{{ scene.description }}</div>
                  </div>
                  <div class="resource-actions">
                    <EditOutlined @click="editScene(scene)" />
                    <DeleteOutlined @click="deleteScene(scene.id)" />
                  </div>
                </div>
              </div>
            </div>
          </a-tab-pane>
        </a-tabs>
      </a-layout-sider>

      <!-- 中间：分镜列表 -->
      <a-layout-content ref="workbenchContentRef" class="workbench-content">
        <div class="shots-header">
          <h3>分镜列表</h3>
          <a-space>
            <a-button :disabled="shotMutationBusy" @click="showAddShot">
              <PlusOutlined /> 添加分镜
            </a-button>
            <a-space>
              <a-button
                type="primary"
                @click="generateAllShots"
                :loading="generating"
                :disabled="shotMutationBusy || hasOpenShotInlineForm || getGeneratableShots().length === 0"
              >
                批量生成
              </a-button>
              <span v-if="generating && batchProgress.total > 0" class="progress-text">
                {{ batchProgress.completed }}/{{ batchProgress.total }}
                <span v-if="batchProgress.failed > 0" style="color: #ff4d4f">
                  ({{ batchProgress.failed }} 失败)
                </span>
              </span>
            </a-space>
          </a-space>
        </div>

        <div class="shots-list">
          <div class="board-list">
            <div
              v-for="(shot, index) in shots"
              :key="shot.id"
              class="shot-card board-card"
              :class="{ active: selectedShot?.id === shot.id, editing: editingShotId === shot.id }"
              @click="editingShotId === shot.id ? undefined : selectShot(shot)"
              @dragover.prevent
              @drop.stop="onDropToShot($event, shot)"
            >
              <div class="board-head">
                <div class="board-title">
                  <span class="shot-order">{{ index + 1 }}</span>
                  <div class="board-title-copy">
                    <div class="shot-name">{{ shot.name || `分镜 ${index + 1}` }}</div>
                    <div class="board-subtitle">
                      {{ shot.scriptOriginalContent || shot.actionDescription || '未填写分镜剧本' }}
                    </div>
                  </div>
                </div>
                <div class="board-status">
                  <span class="status-dot" :class="shot.videoUrl ? 'status-green' : ''" />
                  <span>{{ shot.videoUrl ? '视频已生成' : getStatusText(shot.status) }}</span>
                </div>
                <div class="shot-actions">
                  <a-dropdown>
                    <a-button type="text" size="small">
                      <MoreOutlined />
                    </a-button>
                    <template #overlay>
                      <a-menu>
                        <a-menu-item @click="previewShot(shot)">
                          <EyeOutlined /> 预览提示词
                        </a-menu-item>
                        <a-menu-item
                          :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotActionBusy(shot.id)"
                          @click="generateShot(shot)"
                        >
                          <PlayCircleOutlined /> 生成视频
                        </a-menu-item>
                        <a-menu-item
                          :disabled="shotMutationBusy || hasOpenShotInlineForm"
                          @click="editShot(shot)"
                        >
                          <EditOutlined /> 编辑
                        </a-menu-item>
                        <a-menu-divider />
                        <a-menu-item
                          danger
                          :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotActionBusy(shot.id)"
                          @click="deleteShot(shot.id)"
                        >
                          <DeleteOutlined /> 删除
                        </a-menu-item>
                      </a-menu>
                    </template>
                  </a-dropdown>
                </div>
              </div>

              <template v-if="editingShotId === shot.id">
                <div class="board-body board-body-edit">
                  <div class="board-col col-left col-left-wide">
                    <div class="shot-inline-form">
                      <div class="inline-form-head">
                        <strong>编辑分镜 {{ index + 1 }}</strong>
                        <span>参考 liuguang 的分镜框方式，在当前卡片内维护剧本、动作、角色和场景。</span>
                      </div>
                      <a-input v-model:value="shotForm.name" placeholder="分镜名称（可选）" />
                      <a-textarea
                        v-model:value="shotForm.scriptOriginalContent"
                        placeholder="分镜剧本：填写这一镜的剧本内容"
                        :rows="3"
                      />
                      <a-textarea
                        v-model:value="shotForm.actionDescription"
                        placeholder="动作描述：描述角色动作、场景变化、镜头内容"
                        :rows="3"
                      />
                      <div class="shot-inline-grid">
                        <a-select v-model:value="shotForm.duration" placeholder="时长">
                          <a-select-option :value="5">5 秒</a-select-option>
                          <a-select-option :value="10">10 秒</a-select-option>
                          <a-select-option :value="15">15 秒</a-select-option>
                        </a-select>
                        <a-select v-model:value="shotForm.aspectRatio" placeholder="画面比例">
                          <a-select-option value="16:9">16:9</a-select-option>
                          <a-select-option value="9:16">9:16</a-select-option>
                          <a-select-option value="1:1">1:1</a-select-option>
                        </a-select>
                        <a-select
                          v-model:value="shotForm.characterIds"
                          mode="multiple"
                          placeholder="选择角色"
                        >
                          <a-select-option v-for="char in characters" :key="char.id" :value="char.id">
                            {{ char.name }}
                          </a-select-option>
                        </a-select>
                        <a-select v-model:value="shotForm.sceneId" placeholder="选择场景">
                          <a-select-option v-for="scene in scenes" :key="scene.id" :value="scene.id">
                            {{ scene.name }}
                          </a-select-option>
                        </a-select>
                        <a-select v-model:value="shotForm.cameraMovement" placeholder="镜头运动">
                          <a-select-option value="">无镜头运动</a-select-option>
                          <a-select-option value="push">推镜</a-select-option>
                          <a-select-option value="pull">拉镜</a-select-option>
                          <a-select-option value="pan">横摇</a-select-option>
                          <a-select-option value="tilt">竖摇</a-select-option>
                        </a-select>
                      </div>
                      <div class="inline-form-actions">
                        <a-button :disabled="shotLoading" @click="cancelShotInlineEdit">取消</a-button>
                        <a-button type="primary" :loading="shotLoading" @click="handleAddShot">
                          保存修改
                        </a-button>
                      </div>
                    </div>
                  </div>
                </div>
              </template>

              <template v-else>
                <div class="board-body">
                  <div class="board-col col-left">
                    <div class="section-block">
                      <div class="col-title">分镜剧本</div>
                      <a-textarea
                        :value="shot.scriptOriginalContent"
                        placeholder="剧本内容"
                        :rows="3"
                        @blur="(event) => handleScriptOriginalContentChange(shot, getInputValue(event))"
                      />
                    </div>

                    <div class="section-block">
                      <div class="col-title">动作描述</div>
                      <div class="shot-action">{{ shot.actionDescription || '暂无动作描述' }}</div>
                      <div class="shot-meta">
                        <a-tag v-if="shot.duration" size="small">{{ shot.duration }}s</a-tag>
                        <a-tag v-if="shot.aspectRatio" size="small">{{ shot.aspectRatio }}</a-tag>
                        <a-tag v-if="shot.cameraMovement" size="small">{{ shot.cameraMovement }}</a-tag>
                      </div>
                    </div>

                    <div class="section-block">
                      <div class="section-header">
                        <span class="col-title">参考素材（{{ getShotReferenceAssetCount(shot) }}）</span>
                        <div class="section-actions">
                          <a-button
                            size="small"
                            :disabled="shotMutationBusy || hasOpenShotInlineForm"
                            @click.stop="openShotAssetPicker(shot, 'image')"
                          >
                            资产库图片
                          </a-button>
                          <a-button
                            size="small"
                            :disabled="shotMutationBusy || hasOpenShotInlineForm"
                            @click.stop="openShotAssetPicker(shot, 'video')"
                          >
                            资产库视频
                          </a-button>
                          <a-button
                            size="small"
                            :disabled="shotMutationBusy || hasOpenShotInlineForm"
                            @click.stop="openShotAssetPicker(shot, 'audio')"
                          >
                            资产库音频
                          </a-button>
                          <a-upload
                            accept="image/*"
                            :before-upload="(file) => uploadShotReferenceImage(shot, file)"
                            :show-upload-list="false"
                          >
                            <a-button size="small" :loading="isShotAssetUploading(shot.id, 'image')">
                              上传图片
                            </a-button>
                          </a-upload>
                          <a-upload
                            accept="video/*"
                            :before-upload="(file) => uploadShotReferenceVideo(shot, file)"
                            :show-upload-list="false"
                          >
                            <a-button size="small" :loading="isShotAssetUploading(shot.id, 'video')">
                              上传视频
                            </a-button>
                          </a-upload>
                          <a-upload
                            accept="audio/*"
                            :before-upload="(file) => uploadShotReferenceAudio(shot, file)"
                            :show-upload-list="false"
                          >
                            <a-button size="small" :loading="isShotAssetUploading(shot.id, 'audio')">
                              上传音频
                            </a-button>
                          </a-upload>
                        </div>
                      </div>

                      <div class="shot-reference-grid">
                        <div
                          v-for="asset in getShotAssetsByType(shot, 'image')"
                          :key="asset.id"
                          class="shot-reference-item"
                        >
                          <img :src="previewReferenceAsset(asset.objectUrl)" :alt="asset.name || '分镜参考图片'" />
                          <button class="reference-delete" type="button" @click.stop="deleteShotAsset(asset)">
                            <DeleteOutlined />
                          </button>
                          <div class="reference-label">图片：{{ asset.name || '参考图' }}</div>
                        </div>
                        <div
                          v-for="asset in getShotAssetsByType(shot, 'video')"
                          :key="asset.id"
                          class="shot-reference-item shot-reference-video"
                        >
                          <video :src="previewReferenceAsset(asset.objectUrl)" controls muted playsinline />
                          <button class="reference-delete" type="button" @click.stop="deleteShotAsset(asset)">
                            <DeleteOutlined />
                          </button>
                          <div class="reference-label">视频：{{ asset.name || '参考视频' }}</div>
                        </div>
                        <div
                          v-for="asset in getShotAssetsByType(shot, 'audio')"
                          :key="asset.id"
                          class="shot-reference-item shot-reference-audio"
                        >
                          <audio :src="previewReferenceAsset(asset.objectUrl)" controls />
                          <button class="reference-delete" type="button" @click.stop="deleteShotAsset(asset)">
                            <DeleteOutlined />
                          </button>
                          <div class="reference-label">音频：{{ asset.name || '参考音频' }}</div>
                        </div>
                        <a-empty
                          v-if="getShotReferenceAssetCount(shot) === 0"
                          description="暂无分镜参考素材，请上传图片、视频或音频"
                          :image="simpleImage"
                        />
                      </div>
                    </div>
                  </div>

                  <div class="board-col col-generate">
                    <div class="col-title">视频生成</div>
                    <div class="video-generate-panel">
                      <div class="shot-preview video-preview-large">
                        <video v-if="shot.videoUrl" :src="shot.videoUrl" controls playsinline />
                        <div v-else class="shot-placeholder">
                          <VideoCameraOutlined />
                          <div class="shot-status">{{ getStatusText(shot.status) }}</div>
                        </div>
                      </div>
                      <a-alert
                        v-if="shot.errorMessage"
                        type="error"
                        :message="shot.errorMessage"
                        show-icon
                      />
                      <a-textarea
                        :value="shot.dynamicDescription || shot.generatedPrompt"
                        placeholder="动态描述/生成提示词会显示在这里"
                        :rows="5"
                        @blur="(event) => handleVideoGenerationParamsChange(shot, { dynamicDescription: getInputValue(event) })"
                      />
                      <div class="generation-param-grid">
                        <a-select
                          :value="shot.videoModel || selectedVideoModel"
                          placeholder="选择生视频模型"
                          @change="(value) => handleVideoGenerationParamsChange(shot, { videoModel: String(value || '') })"
                        >
                          <a-select-option
                            v-for="option in videoModelOptions"
                            :key="option.value"
                            :value="option.value"
                          >
                            {{ option.label }}
                          </a-select-option>
                        </a-select>
                        <a-select
                          :value="shot.videoResolution || '720p'"
                          placeholder="选择分辨率"
                          @change="(value) => handleVideoGenerationParamsChange(shot, { videoResolution: String(value || '') })"
                        >
                          <a-select-option value="720p">720p</a-select-option>
                          <a-select-option value="1080p">1080p</a-select-option>
                        </a-select>
                        <a-select
                          :value="shot.soundAndPictureTogether || soundAndPictureTogether"
                          placeholder="启用/禁用音画同出"
                          @change="(value) => handleVideoGenerationParamsChange(shot, { soundAndPictureTogether: String(value || '') })"
                        >
                          <a-select-option
                            v-for="option in soundAndPictureTogetherOptions"
                            :key="option.value"
                            :value="option.value"
                          >
                            {{ option.label }}
                          </a-select-option>
                        </a-select>
                      </div>
                      <div class="estimate-pill">
                        积分预估：{{ videoEstimateMap[shot.id] ?? estimateShotVideoPoints(shot) }}
                      </div>
                      <a-button
                        type="primary"
                        block
                        :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotActionBusy(shot.id)"
                        :loading="isShotActionBusy(shot.id)"
                        @click="generateShot(shot)"
                      >
                        生成视频
                      </a-button>
                    </div>
                  </div>

                  <div class="board-col col-version">
                    <div class="version-panel-head">
                      <div class="col-title">视频版本</div>
                      <a-button
                        size="small"
                        :loading="isShotVideoVersionRefreshing(shot.id)"
                        :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotVideoVersionRefreshing(shot.id)"
                        @click.stop="handleRefreshShotVideoVersions(shot)"
                      >
                        刷新版本
                      </a-button>
                      <a-tag v-if="isShotGenerationPolling(shot.id)" color="processing">自动刷新中</a-tag>
                    </div>
                    <div class="version-panel">
                      <a-spin :spinning="isShotVideoVersionLoading(shot.id)">
                        <div v-if="getShotVideoVersions(shot).length > 0" class="version-list">
                          <div
                            v-for="version in getShotVideoVersions(shot)"
                            :key="version.id"
                            class="version-item"
                            :class="{ 'is-active': version.isCurrent }"
                          >
                            <div class="version-preview-wrap">
                              <div
                                v-if="previewReferenceAsset(version.videoUrl)"
                                class="version-video-thumbnail"
                                @click.stop="handleOpenShotVideoPreview(shot, version)"
                              >
                                <img
                                  v-if="supportsShotVideoThumbnailSnapshot(version.videoUrl)"
                                  class="thumbnail-image"
                                  :src="getShotVideoThumbnailUrl(version.videoUrl)"
                                  alt="视频缩略图"
                                  @error.stop="handleShotVideoThumbnailError(version.videoUrl)"
                                />
                                <div v-else class="thumbnail-fallback">
                                  <VideoCameraOutlined />
                                  <span>点击预览视频</span>
                                </div>
                                <div class="play-overlay">
                                  <PlayCircleOutlined />
                                </div>
                              </div>
                              <div v-else class="version-empty-preview">
                                {{ getStatusText(version.status) }}
                              </div>
                              <button
                                v-if="isUnviewedShotVideoVersion(version)"
                                class="unviewed-badge"
                                type="button"
                                title="关闭未查看标记"
                                @click.stop="handleMarkShotVideoVersionViewed(shot, version)"
                              >
                                <span class="unviewed-dot" />
                                <span>未查看</span>
                                <span class="unviewed-close">×</span>
                              </button>
                              <div class="version-resolution">
                                <template v-if="isShotVideoVersionSubtitleRemoved(version)">无字幕 · </template>
                                {{ version.model || shot.videoModel || selectedVideoModel || '视频' }}
                                <template v-if="version.seconds || shot.duration">
                                  · {{ version.seconds || shot.duration }}s
                                </template>
                                <template v-if="version.aspectRatio || shot.aspectRatio">
                                  · {{ version.aspectRatio || shot.aspectRatio }}
                                </template>
                              </div>
                              <div class="version-status-row">
                                <button
                                  v-if="version.isCurrent"
                                  class="status-tag status-tag--current"
                                  type="button"
                                  title="当前版本"
                                >
                                  当前
                                </button>
                                <button
                                  class="status-tag status-tag--backup"
                                  :class="{ 'is-active': isShotVideoVersionBackup(version) }"
                                  type="button"
                                  :title="isShotVideoVersionBackup(version) ? '取消备选' : '设为备选'"
                                  :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotVideoVersionBusy(shot.id, version.id)"
                                  @click.stop="handleSetShotVideoVersionBackup(shot, version)"
                                >
                                  {{ isShotVideoVersionBackup(version) ? '备选' : '备' }}
                                </button>
                                <a-dropdown :disabled="!canUpscaleShotVideoVersion(shot, version)" :trigger="['click']">
                                  <button
                                    class="status-tag status-tag--upscale"
                                    :class="{ 'is-active': isShotVideoVersionUpscaled(version) }"
                                    type="button"
                                    title="超分辨率"
                                    :disabled="!canUpscaleShotVideoVersion(shot, version)"
                                    @click.stop
                                  >
                                    {{ isShotVideoVersionUpscaled(version) ? '超分' : '超' }}
                                  </button>
                                  <template #overlay>
                                    <a-menu>
                                      <a-menu-item
                                        v-for="option in upscaleResolutionOptions"
                                        :key="option.value"
                                        @click.stop="handleUpscaleShotVideoVersion(shot, version, option.value)"
                                      >
                                        {{ option.label }}
                                      </a-menu-item>
                                    </a-menu>
                                  </template>
                                </a-dropdown>
                                <button
                                  class="status-tag status-tag--extract"
                                  type="button"
                                  title="抽帧"
                                  aria-label="视频抽帧"
                                  :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotVideoFrameExtracting(shot.id, version.id) || !previewReferenceAsset(version.videoUrl)"
                                  @click.stop="handleExtractShotVideoFrame(shot, version)"
                                >
                                  抽
                                </button>
                                <button
                                  class="status-tag status-tag--subtitle"
                                  :class="{ 'is-active': isShotVideoVersionSubtitleRemoved(version) }"
                                  type="button"
                                  title="擦除字幕"
                                  :disabled="!canRemoveShotVideoVersionSubtitle(shot, version)"
                                  @click.stop="handleRemoveShotVideoVersionSubtitle(shot, version)"
                                >
                                  {{ isShotVideoVersionSubtitleRemoved(version) ? '无字幕' : '擦' }}
                                </button>
                              </div>
                              <button
                                class="detail-view-btn"
                                type="button"
                                title="查看视频生成详情"
                                aria-label="查看详情"
                                @click.stop="handleOpenVideoGenerationDetail(shot, version)"
                              >
                                <EyeOutlined />
                              </button>
                            </div>
                            <div class="version-meta">
                              <div class="version-tags">
                                <a-tag v-if="version.isCurrent" color="blue">当前版本</a-tag>
                                <a-tag
                                  v-if="isShotVideoVersionBackup(version)"
                                  class="backup-video-tag"
                                  color="gold"
                                >
                                  备选
                                </a-tag>
                                <a-tag v-if="isShotVideoVersionSubtitleRemoved(version)" color="purple">
                                  已擦字幕
                                </a-tag>
                                <a-tag v-if="isShotVideoVersionUpscaled(version)" color="green">
                                  已超分{{ version.upscaledResolution ? ` · ${version.upscaledResolution}` : '' }}
                                </a-tag>
                                <a-tag :color="getStatusColor(version.status)">
                                  {{ getStatusText(version.status) }}
                                </a-tag>
                              </div>
                              <div class="version-model">
                                {{ version.model || shot.videoModel || selectedVideoModel }}
                                · {{ version.seconds || shot.duration }}s
                                · {{ version.aspectRatio || shot.aspectRatio }}
                              </div>
                              <div class="version-time">{{ version.createTime || version.updateTime }}</div>
                            </div>
                            <div class="version-actions is-secondary">
                              <a-button
                                class="action-tag"
                                size="small"
                                :disabled="!previewReferenceAsset(version.videoUrl)"
                                @click.stop="handleOpenShotVideoPreview(shot, version)"
                              >
                                预览视频
                              </a-button>
                              <a-button
                                v-if="!version.isCurrent"
                                class="action-tag"
                                size="small"
                                title="设为当前"
                                :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotVideoVersionBusy(shot.id, version.id)"
                                :loading="isShotVideoVersionBusy(shot.id, version.id)"
                                @click.stop="handleUseShotVideoVersion(shot, version)"
                              >
                                使用此视频
                              </a-button>
                              <a-button
                                class="action-tag"
                                size="small"
                                :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotVideoVersionBusy(shot.id, version.id)"
                                :loading="isShotVideoVersionBusy(shot.id, version.id)"
                                @click.stop="handleRegenerateShotVideoVersion(shot, version)"
                              >
                                重新生成
                              </a-button>
                              <a-button
                                class="action-tag"
                                size="small"
                                :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotVideoVersionBusy(shot.id, version.id)"
                                :loading="isShotVideoVersionBusy(shot.id, version.id)"
                                @click.stop="handleReeditShotVideoVersion(shot, version)"
                              >
                                重编辑
                              </a-button>
                              <a-button
                                class="action-tag"
                                size="small"
                                :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotVideoVersionBusy(shot.id, version.id)"
                                @click.stop="handleOpenCopyShotVideoVersion(shot, version)"
                              >
                                复制到分镜
                              </a-button>
                              <a-button
                                class="action-tag"
                                size="small"
                                danger
                                :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotVideoVersionBusy(shot.id, version.id)"
                                :loading="isShotVideoVersionBusy(shot.id, version.id)"
                                @click.stop="handleDeleteShotVideoVersion(shot, version)"
                              >
                                删除版本
                              </a-button>
                            </div>
                          </div>
                        </div>
                        <a-empty
                          v-else
                          description="暂无视频版本，生成后会在这里回显预览"
                          :image="simpleImage"
                        />
                      </a-spin>
                    </div>
                  </div>
                </div>
              </template>
            </div>

            <div
              v-if="inlineShotFormVisible && !editingShotId"
              ref="shotEditCardRef"
              class="shot-card shot-edit-card"
              @click.stop
            >
              <div class="board-card inline-board-card">
                <div class="board-head">
                  <div class="board-title">
                    <span class="shot-order">{{ getNextShotOrderNum() }}</span>
                    <div class="board-title-copy">
                      <div class="shot-name">新增分镜</div>
                      <div class="board-subtitle">直接在分镜框里填写信息，保存后继续绑定资产和生成视频。</div>
                    </div>
                  </div>
                </div>
                <div class="board-body board-body-edit">
                  <div class="board-col col-left col-left-wide">
                    <div class="shot-inline-form">
                      <div class="inline-form-head">
                        <strong>新增分镜</strong>
                        <span>直接在分镜框里填写信息，保存后进入列表继续绑定资产和生成视频。</span>
                      </div>
                      <a-input v-model:value="shotForm.name" placeholder="分镜名称（可选）" />
                      <a-textarea
                        v-model:value="shotForm.scriptOriginalContent"
                        placeholder="分镜剧本：填写这一镜的剧本内容"
                        :rows="3"
                      />
                      <a-textarea
                        v-model:value="shotForm.actionDescription"
                        placeholder="动作描述：描述角色动作、场景变化、镜头内容"
                        :rows="3"
                      />
                      <div class="shot-inline-grid">
                        <a-select v-model:value="shotForm.duration" placeholder="时长">
                          <a-select-option :value="5">5 秒</a-select-option>
                          <a-select-option :value="10">10 秒</a-select-option>
                          <a-select-option :value="15">15 秒</a-select-option>
                        </a-select>
                        <a-select v-model:value="shotForm.aspectRatio" placeholder="画面比例">
                          <a-select-option value="16:9">16:9</a-select-option>
                          <a-select-option value="9:16">9:16</a-select-option>
                          <a-select-option value="1:1">1:1</a-select-option>
                        </a-select>
                        <a-select
                          v-model:value="shotForm.characterIds"
                          mode="multiple"
                          placeholder="选择角色"
                        >
                          <a-select-option v-for="char in characters" :key="char.id" :value="char.id">
                            {{ char.name }}
                          </a-select-option>
                        </a-select>
                        <a-select v-model:value="shotForm.sceneId" placeholder="选择场景">
                          <a-select-option v-for="scene in scenes" :key="scene.id" :value="scene.id">
                            {{ scene.name }}
                          </a-select-option>
                        </a-select>
                        <a-select v-model:value="shotForm.cameraMovement" placeholder="镜头运动">
                          <a-select-option value="">无镜头运动</a-select-option>
                          <a-select-option value="push">推镜</a-select-option>
                          <a-select-option value="pull">拉镜</a-select-option>
                          <a-select-option value="pan">横摇</a-select-option>
                          <a-select-option value="tilt">竖摇</a-select-option>
                        </a-select>
                      </div>
                      <div class="inline-form-actions">
                        <a-button :disabled="shotLoading" @click="cancelShotInlineEdit">取消</a-button>
                        <a-button type="primary" :loading="shotLoading" @click="handleAddShot">
                          保存分镜
                        </a-button>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <a-empty
              v-if="shots.length === 0 && !inlineShotFormVisible"
              description="暂无分镜，点击添加分镜开始创作"
            />
          </div>
        </div>
      </a-layout-content>

      <!-- 右侧：分镜详情和参考素材 -->
      <a-layout-sider width="350" theme="light" class="workbench-sider-right">
        <div v-if="selectedShot" class="shot-detail-panel">
          <a-tabs v-model:activeKey="rightTab">
            <a-tab-pane key="detail" tab="分镜详情">
              <div class="detail-section">
                <h4>动作描述</h4>
                <p>{{ selectedShot.actionDescription }}</p>

                <h4>生成参数</h4>
                <a-descriptions :column="1" size="small" bordered>
                  <a-descriptions-item label="时长">
                    {{ selectedShot.duration }}s
                  </a-descriptions-item>
                  <a-descriptions-item label="比例">
                    {{ selectedShot.aspectRatio }}
                  </a-descriptions-item>
                  <a-descriptions-item label="镜头运动">
                    {{ selectedShot.cameraMovement || '无' }}
                  </a-descriptions-item>
                  <a-descriptions-item label="状态">
                    <a-tag :color="getStatusColor(selectedShot.status)">
                      {{ getStatusText(selectedShot.status) }}
                    </a-tag>
                  </a-descriptions-item>
                </a-descriptions>

                <h4>参考角色</h4>
                <div class="reference-list">
                  <div
                    v-for="charId in selectedShot.characterIds"
                    :key="charId"
                    class="reference-item"
                  >
                    <img v-if="getCharacterImage(charId)" :src="getCharacterImage(charId)" alt="角色参考图" />
                    <UserOutlined v-else />
                    <span>{{ getCharacterName(charId) }}</span>
                  </div>
                  <a-empty v-if="!selectedShot.characterIds?.length" :image="simpleImage" />
                </div>

                <h4>参考素材</h4>
                <div class="reference-list">
                  <div
                    v-for="reference in getShotReferenceDisplayAssets(selectedShot)"
                    :key="`${reference.type}-${reference.url}`"
                    class="reference-item"
                    :class="{
                      'reference-video-item': reference.type === 'video',
                      'reference-audio-item': reference.type === 'audio',
                    }"
                  >
                    <img
                      v-if="reference.type === 'image'"
                      :src="previewReferenceAsset(reference.url)"
                      :alt="reference.label"
                    />
                    <video
                      v-else-if="reference.type === 'video'"
                      :src="previewReferenceAsset(reference.url)"
                      controls
                      muted
                      playsinline
                    />
                    <audio
                      v-else-if="reference.type === 'audio'"
                      :src="previewReferenceAsset(reference.url)"
                      controls
                    />
                    <span>{{ reference.label }}</span>
                  </div>
                  <a-empty
                    v-if="getShotReferenceDisplayAssets(selectedShot).length === 0"
                    :image="simpleImage"
                  />
                </div>
              </div>
            </a-tab-pane>

            <a-tab-pane key="prompt" tab="提示词">
              <div class="prompt-section">
                <a-alert
                  message="智能提示词"
                  description="系统自动生成的结构化提示词，遵循即梦最佳实践，降低抽卡率"
                  type="info"
                  show-icon
                  style="margin-bottom: 16px"
                />

                <a-textarea
                  :value="selectedShot.generatedPrompt"
                  :rows="15"
                  readonly
                  style="font-family: monospace"
                />

                <a-button block style="margin-top: 16px" @click="copyPrompt">
                  <CopyOutlined /> 复制提示词
                </a-button>
              </div>
            </a-tab-pane>
          </a-tabs>
        </div>

        <a-empty
          v-else
          description="选择一个分镜查看详情"
          :image="simpleImage"
        />
      </a-layout-sider>
    </a-layout>

    <section v-else class="short-film-placeholder-card">
      <div class="short-film-placeholder-icon">短片制</div>
      <h3>短片制工作台占位</h3>
      <p>
        短片制能力正在接入。当前项目制工作台已先完成分镜、素材、生成参数和版本管理迁移；
        短片制后续会复用 OSS 素材上传、视频生成和版本回显能力。
      </p>
      <div class="short-film-placeholder-grid">
        <span>快速脚本</span>
        <span>单片素材</span>
        <span>一键生成</span>
        <span>版本回显</span>
      </div>
    </section>

    <!-- 项目设置对话框 -->
    <a-modal
      v-model:open="projectModalVisible"
      title="项目设置"
      @ok="handleUpdateProject"
      :confirmLoading="projectLoading"
      width="680px"
    >
      <a-form :model="projectForm" layout="vertical">
        <a-form-item label="项目名称" required>
          <a-input v-model:value="projectForm.name" placeholder="请输入项目名称" />
        </a-form-item>
        <a-form-item label="主题提示词">
          <a-textarea
            v-model:value="projectForm.theme"
            placeholder="描述项目主题"
            :rows="2"
          />
        </a-form-item>
        <a-form-item label="项目描述">
          <a-textarea
            v-model:value="projectForm.description"
            placeholder="请输入项目描述"
            :rows="3"
          />
        </a-form-item>
        <a-form-item label="风格提示词">
          <a-textarea
            v-model:value="projectForm.styleGuide"
            placeholder="例如：Studio Ghibli animation style, soft natural lighting"
            :rows="3"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 添加角色对话框 -->
    <a-modal
      v-model:open="characterModalVisible"
      :title="editingCharacterId ? '编辑角色' : '添加角色'"
      @ok="handleAddCharacter"
      :confirmLoading="characterLoading"
    >
      <a-form :model="characterForm" layout="vertical">
        <a-form-item label="角色名称" required>
          <a-input v-model:value="characterForm.name" placeholder="输入角色名称" />
        </a-form-item>
        <a-form-item label="角色描述" required>
          <a-textarea
            v-model:value="characterForm.description"
            placeholder="描述角色外貌特征"
            :rows="3"
          />
        </a-form-item>
        <a-form-item label="参考图片">
          <div class="reference-upload-field">
            <a-upload
              accept="image/*"
              :before-upload="uploadCharacterReferenceImage"
              :show-upload-list="false"
            >
              <a-button :loading="characterImageUploading">
                上传角色参考图
              </a-button>
            </a-upload>
            <div v-if="characterImagePreviewUrl" class="reference-upload-preview">
              <img :src="characterImagePreviewUrl" alt="角色参考图预览" />
              <div class="reference-upload-actions">
                <span class="reference-upload-url">{{ characterForm.referenceImageUrl }}</span>
                <a-button
                  danger
                  size="small"
                  type="text"
                  @click="clearCharacterReferenceImage"
                >
                  清除
                </a-button>
              </div>
            </div>
            <div v-else class="reference-upload-empty">
              请选择图片，上传后会保存到阿里云 OSS 文件桶并在这里回显。
            </div>
          </div>
        </a-form-item>
        <a-form-item>
          <a-checkbox v-model:checked="characterForm.isMain">设为主角</a-checkbox>
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 添加场景对话框 -->
    <a-modal
      v-model:open="sceneModalVisible"
      :title="editingSceneId ? '编辑场景' : '添加场景'"
      @ok="handleAddScene"
      :confirmLoading="sceneLoading"
    >
      <a-form :model="sceneForm" layout="vertical">
        <a-form-item label="场景名称" required>
          <a-input v-model:value="sceneForm.name" placeholder="输入场景名称" />
        </a-form-item>
        <a-form-item label="场景描述" required>
          <a-textarea
            v-model:value="sceneForm.description"
            placeholder="描述场景环境"
            :rows="3"
          />
        </a-form-item>
        <a-form-item label="参考图片">
          <div class="reference-upload-field">
            <a-upload
              accept="image/*"
              :before-upload="uploadSceneReferenceImage"
              :show-upload-list="false"
            >
              <a-button :loading="sceneImageUploading">
                上传场景参考图
              </a-button>
            </a-upload>
            <div v-if="sceneImagePreviewUrl" class="reference-upload-preview">
              <img :src="sceneImagePreviewUrl" alt="场景参考图预览" />
              <div class="reference-upload-actions">
                <span class="reference-upload-url">{{ sceneForm.referenceImageUrl }}</span>
                <a-button
                  danger
                  size="small"
                  type="text"
                  @click="clearSceneReferenceImage"
                >
                  清除
                </a-button>
              </div>
            </div>
            <div v-else class="reference-upload-empty">
              请选择图片，上传后会保存到阿里云 OSS 文件桶并在这里回显。
            </div>
          </div>
        </a-form-item>
        <a-form-item label="参考视频">
          <div class="reference-upload-field">
            <a-upload
              accept="video/*"
              :before-upload="uploadSceneReferenceVideo"
              :show-upload-list="false"
            >
              <a-button :loading="sceneVideoUploading">
                上传场景参考视频
              </a-button>
            </a-upload>
            <div v-if="sceneVideoPreviewUrl" class="reference-upload-preview reference-upload-video">
              <video
                :src="sceneVideoPreviewUrl"
                controls
                playsinline
              />
              <div class="reference-upload-actions">
                <span class="reference-upload-url">{{ sceneForm.referenceVideoUrl }}</span>
                <a-button
                  danger
                  size="small"
                  type="text"
                  @click="clearSceneReferenceVideo"
                >
                  清除
                </a-button>
              </div>
            </div>
            <div v-else class="reference-upload-empty">
              请选择视频，上传后会保存到阿里云 OSS 文件桶并在这里回显。
            </div>
          </div>
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 视频生成详情 -->
    <a-modal
      v-model:open="videoGenerationDetailModalVisible"
      title="视频生成详情"
      :footer="null"
      width="1080px"
      wrap-class-name="video-generation-detail-modal"
      @cancel="handleCloseVideoGenerationDetail"
    >
      <a-spin :spinning="videoGenerationDetailLoading">
        <div
          v-if="videoGenerationDetailShot && videoGenerationDetailVersion"
          class="video-generation-detail-dialog"
        >
        <div class="detail-meta">
          <div class="detail-meta-item">
            <span>模型</span>
            <strong>{{ videoGenerationDetailVersion.model || videoGenerationDetailShot.videoModel || selectedVideoModel }}</strong>
          </div>
          <div class="detail-meta-item">
            <span>状态</span>
            <strong>
              {{ getStatusText(videoGenerationDetailVersion.status) }}
              {{ isShotVideoVersionBackup(videoGenerationDetailVersion) ? ' · 备选' : '' }}
              {{ isShotVideoVersionSubtitleRemoved(videoGenerationDetailVersion) ? ' · 无字幕' : '' }}
              {{ isShotVideoVersionUpscaled(videoGenerationDetailVersion) ? ` · 已超分${videoGenerationDetailVersion.upscaledResolution ? ` ${videoGenerationDetailVersion.upscaledResolution}` : ''}` : '' }}
            </strong>
          </div>
          <div class="detail-meta-item">
            <span>时长 / 画幅</span>
            <strong>
              {{ videoGenerationDetailVersion.seconds || videoGenerationDetailShot.duration }}s ·
              {{ videoGenerationDetailVersion.aspectRatio || videoGenerationDetailShot.aspectRatio }}
            </strong>
          </div>
          <div class="detail-meta-item">
            <span>生成时间</span>
            <strong>{{ videoGenerationDetailVersion.createTime || videoGenerationDetailVersion.updateTime || '暂无' }}</strong>
          </div>
        </div>

        <div class="video-generation-detail-grid">
          <section class="detail-panel detail-panel-script">
            <h4>分镜脚本</h4>
            <div class="detail-dark detail-script">
              {{
                videoGenerationDetailShot.scriptOriginalContent ||
                videoGenerationDetailShot.actionDescription ||
                '暂无'
              }}
            </div>

            <h4>生成参考内容</h4>
            <div class="detail-reference-grid">
              <div
                v-for="reference in getVideoGenerationDetailReferences(videoGenerationDetailShot)"
                :key="`${reference.type}-${reference.url}`"
                class="detail-reference-card"
              >
                <img
                  v-if="reference.type === 'image'"
                  :src="previewReferenceAsset(reference.url)"
                  :alt="reference.label"
                />
                <video
                  v-else-if="reference.type === 'video'"
                  :src="previewReferenceAsset(reference.url)"
                  controls
                  playsinline
                />
                <audio
                  v-else
                  :src="previewReferenceAsset(reference.url)"
                  controls
                />
                <span>{{ reference.label }}</span>
              </div>
              <div
                v-if="getVideoGenerationDetailReferences(videoGenerationDetailShot).length === 0"
                class="detail-empty"
              >
                暂无参考内容
              </div>
            </div>
          </section>

          <section class="detail-panel detail-panel-prompt">
            <h4>提示词</h4>
            <div class="detail-dark detail-prompt">
              {{
                videoGenerationDetailVersion.prompt ||
                getShotVideoPrompt(videoGenerationDetailShot) ||
                '暂无'
              }}
            </div>
          </section>

          <section class="detail-panel detail-panel-result">
            <h4>视频结果</h4>
            <video
              v-if="previewReferenceAsset(videoGenerationDetailVersion.videoUrl)"
              :src="previewReferenceAsset(videoGenerationDetailVersion.videoUrl)"
              controls
              playsinline
              class="detail-video-result"
            />
            <div v-else class="detail-empty">暂无视频结果</div>
          </section>
        </div>

        <div class="detail-footer">
          <a-button
            class="detail-action detail-video-preview"
            :disabled="!previewReferenceAsset(videoGenerationDetailVersion.videoUrl)"
            @click="handleOpenShotVideoPreview(videoGenerationDetailShot, videoGenerationDetailVersion)"
          >
            预览视频
          </a-button>
          <a-button
            v-if="!videoGenerationDetailVersion.isCurrent"
            class="detail-action"
            type="primary"
            :disabled="shotMutationBusy || hasOpenShotInlineForm"
            @click="handleUseVideoGenerationDetailVersion"
          >
            使用此视频
          </a-button>
          <a-button
            class="detail-action"
            :disabled="shotMutationBusy || hasOpenShotInlineForm"
            @click="handleRegenerateVideoGenerationDetailVersion"
          >
            重新生成
          </a-button>
          <a-button
            class="detail-action"
            :disabled="shotMutationBusy || hasOpenShotInlineForm"
            @click="handleReeditVideoGenerationDetailVersion"
          >
            重编辑
          </a-button>
          <a-button
            class="detail-action"
            :disabled="shotMutationBusy || hasOpenShotInlineForm"
            @click="handleCopyVideoGenerationDetailVersion"
          >
            复制到分镜
          </a-button>
          <a-button
            class="detail-action"
            :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotVideoVersionBusy(videoGenerationDetailShot.id, videoGenerationDetailVersion.id)"
            :loading="isShotVideoVersionBusy(videoGenerationDetailShot.id, videoGenerationDetailVersion.id)"
            @click="handleSetShotVideoVersionBackup(videoGenerationDetailShot, videoGenerationDetailVersion)"
          >
            {{ isShotVideoVersionBackup(videoGenerationDetailVersion) ? '取消备选' : '设为备选' }}
          </a-button>
          <a-button
            class="detail-action"
            :disabled="shotMutationBusy || hasOpenShotInlineForm || isShotVideoFrameExtracting(videoGenerationDetailShot.id, videoGenerationDetailVersion.id) || !previewReferenceAsset(videoGenerationDetailVersion.videoUrl)"
            :loading="isShotVideoFrameExtracting(videoGenerationDetailShot.id, videoGenerationDetailVersion.id)"
            @click="handleExtractShotVideoFrame(videoGenerationDetailShot, videoGenerationDetailVersion)"
          >
            抽帧
          </a-button>
          <a-button
            class="detail-action"
            :disabled="!canRemoveShotVideoVersionSubtitle(videoGenerationDetailShot, videoGenerationDetailVersion)"
            :loading="isShotVideoVersionBusy(videoGenerationDetailShot.id, videoGenerationDetailVersion.id)"
            @click="handleRemoveSubtitleVideoGenerationDetailVersion"
          >
            {{ isShotVideoVersionSubtitleRemoved(videoGenerationDetailVersion) ? '已擦字幕' : '擦除字幕' }}
          </a-button>
          <a-dropdown
            :disabled="!canUpscaleShotVideoVersion(videoGenerationDetailShot, videoGenerationDetailVersion)"
            :trigger="['click']"
          >
            <a-button
              class="detail-action"
              :disabled="!canUpscaleShotVideoVersion(videoGenerationDetailShot, videoGenerationDetailVersion)"
              :loading="isShotVideoVersionBusy(videoGenerationDetailShot.id, videoGenerationDetailVersion.id)"
            >
              {{ isShotVideoVersionUpscaled(videoGenerationDetailVersion) ? '已超分' : '超分辨率' }}
            </a-button>
            <template #overlay>
              <a-menu>
                <a-menu-item
                  v-for="option in upscaleResolutionOptions"
                  :key="option.value"
                  @click="handleUpscaleVideoGenerationDetailVersion(option.value)"
                >
                  {{ option.label }}
                </a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>
          <a-button class="detail-action detail-action-close" @click="handleCloseVideoGenerationDetail">
            关闭
          </a-button>
        </div>
        </div>
      </a-spin>
    </a-modal>

    <!-- 视频版本大窗预览 -->
    <a-modal
      v-model:open="shotVideoPreviewModalVisible"
      :title="shotVideoPreviewTitle || '视频预览'"
      :footer="null"
      width="960px"
      @cancel="handleCloseShotVideoPreview"
    >
      <div class="shot-video-preview-dialog">
        <video
          v-if="shotVideoPreviewUrl"
          class="preview-dialog-video"
          :src="shotVideoPreviewUrl"
          controls
          autoplay
          playsinline
        />
        <a-empty v-else description="暂无可预览视频" :image="simpleImage" />
      </div>
    </a-modal>

    <!-- 复制视频版本到分镜 -->
    <a-modal
      v-model:open="copyShotVideoVersionModalVisible"
      title="复制到分镜"
      ok-text="确认复制"
      :confirmLoading="copyShotVideoVersionLoading"
      @ok="handleCopyShotVideoVersion"
    >
      <a-form layout="vertical">
        <a-form-item label="复制方式" required>
          <a-segmented
            v-model:value="copyShotVideoVersionMode"
            :options="[
              { label: '指定分镜', value: 'assign' },
              { label: '新增分镜', value: 'append' },
            ]"
          />
        </a-form-item>
        <a-form-item
          v-if="copyShotVideoVersionMode === 'assign'"
          label="选择目标分镜"
          required
        >
          <a-select
            v-model:value="copyShotVideoVersionTargetShotId"
            placeholder="选择目标分镜"
          >
            <a-select-option
              v-for="option in copyShotVideoVersionTargetOptions"
              :key="option.value"
              :value="option.value"
            >
              {{ option.label }}
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-alert
          type="info"
          show-icon
          :message="
            copyShotVideoVersionMode === 'assign'
              ? '确认复制后，目标分镜会新增一个相同视频版本并设为当前版本。'
              : '确认复制后，会新增一个分镜并把当前视频版本复制过去。'
          "
        />
      </a-form>
    </a-modal>

    <AssetPicker
      v-model:open="shotAssetPickerOpen"
      :allow-types="shotAssetPickerAllowTypes"
      @pick="handlePickShotAsset"
    />

    <!-- 预览提示词对话框 -->
    <a-modal
      v-model:open="previewModalVisible"
      title="预览提示词"
      :footer="null"
      width="700px"
    >
      <a-spin :spinning="previewLoading">
        <div v-if="shotPreview">
          <a-alert
            v-if="!shotPreview.validation.isValid"
            type="error"
            message="验证失败"
            :description="shotPreview.validation.errors.join(', ')"
            show-icon
            style="margin-bottom: 16px"
          />

          <a-alert
            v-if="shotPreview.validation.warnings.length > 0"
            type="warning"
            :message="`建议：${shotPreview.validation.warnings.join(', ')}`"
            show-icon
            style="margin-bottom: 16px"
          />

          <a-statistic
            title="预估成功率"
            :value="shotPreview.estimatedSuccessRate"
            suffix="%"
            :value-style="{ color: shotPreview.estimatedSuccessRate > 70 ? '#3f8600' : '#cf1322' }"
            style="margin-bottom: 16px"
          />

          <h4>生成的提示词</h4>
          <a-textarea
            :value="shotPreview.prompt"
            :rows="10"
            readonly
            style="font-family: monospace; margin-bottom: 16px"
          />

          <h4>参考素材</h4>
          <div class="preview-images">
            <img
              v-for="(img, idx) in shotPreview.images"
              :key="idx"
              :src="previewReferenceAsset(img)"
              alt="提示词参考图片"
              style="width: 100px; height: 100px; object-fit: cover; margin: 4px"
            />
            <video
              v-for="(video, idx) in shotPreview.videos"
              :key="`video-${idx}`"
              :src="previewReferenceAsset(video)"
              controls
              playsinline
              style="width: 160px; height: 100px; object-fit: cover; margin: 4px"
            />
          </div>
        </div>
      </a-spin>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import type { VideoAsset, VideoAssetType } from '#/api/core/asset'
import type { Ref } from 'vue'

import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useAccessStore } from '@vben/stores'

import {
  Alert as AAlert,
  Breadcrumb as ABreadcrumb,
  BreadcrumbItem as ABreadcrumbItem,
  Button as AButton,
  Checkbox as ACheckbox,
  Descriptions as ADescriptions,
  DescriptionsItem as ADescriptionsItem,
  Dropdown as ADropdown,
  Empty,
  Empty as AEmpty,
  Form as AForm,
  FormItem as AFormItem,
  Input as AInput,
  Layout as ALayout,
  LayoutContent as ALayoutContent,
  LayoutSider as ALayoutSider,
  Menu as AMenu,
  MenuDivider as AMenuDivider,
  MenuItem as AMenuItem,
  Modal,
  Modal as AModal,
  Select as ASelect,
  SelectOption as ASelectOption,
  Segmented as ASegmented,
  Space as ASpace,
  Spin as ASpin,
  Statistic as AStatistic,
  TabPane as ATabPane,
  Tabs as ATabs,
  Tag as ATag,
  Textarea as ATextarea,
  Upload as AUpload,
  message,
} from 'ant-design-vue'
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  UserOutlined,
  EnvironmentOutlined,
  VideoCameraOutlined,
  MoreOutlined,
  EyeOutlined,
  PlayCircleOutlined,
  CopyOutlined,
} from '@ant-design/icons-vue'
import {
  getProjectApi,
  updateProjectApi,
  listCharactersApi,
  listScenesApi,
  listShotsApi,
  createCharacterApi,
  createSceneApi,
  createShotApi,
  updateCharacterApi,
  updateSceneApi,
  updateShotApi,
  deleteShotApi,
  deleteCharacterApi,
  deleteSceneApi,
  createShotAssetApi,
  deleteShotAssetApi,
  listShotVideoVersionsApi,
  getShotVideoVersionDetailApi,
  setShotVideoVersionApi,
  setShotVideoVersionBackupApi,
  removeShotVideoVersionSubtitleApi,
  upscaleShotVideoVersionApi,
  refreshShotVideoVersionsApi,
  copyShotVideoVersionApi,
  extractShotVideoFrameApi,
  deleteShotVideoVersionApi,
  markShotVideoVersionViewedApi,
  previewShotPromptApi,
  generateShotApi,
  batchGenerateShotsApi,
  composeProjectVideoApi,
  getComposeJobApi,
  type Project,
  type Character,
  type Scene,
  type Shot,
  type ShotAsset,
  type ShotVideoVersion,
  type ShotVideoVersionDetail,
  type ShotPreview,
} from '#/api/core/videoproject'
import { uploadFileApi, type UploadedFile } from '#/api/core/upload'
import {
  useUploadAssetPreviewResolver,
  useUploadAssetPreviewUrl,
} from '#/utils/upload-asset-preview'
import { getAssetPreviewSource } from '../asset-preview'
import AssetPicker from '../components/AssetPicker.vue'

const route = useRoute()
const router = useRouter()
const accessStore = useAccessStore()
const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE

// 状态
const project = ref<Project | null>(null)
const characters = ref<Character[]>([])
const scenes = ref<Scene[]>([])
const shots = ref<Shot[]>([])
const selectedShot = ref<Shot | null>(null)
const shotPreview = ref<ShotPreview | null>(null)
const workbenchContentRef = ref<HTMLElement | { $el?: HTMLElement } | null>(null)

const leftTab = ref('characters')
const rightTab = ref('detail')

const workbenchModeOptions: Array<{ label: string; value: 'project' | 'short-film' }> = [
  { label: '项目制', value: 'project' },
  { label: '短片制', value: 'short-film' },
]
const workbenchMode = ref<'project' | 'short-film'>('project')
const activeProductionStep = ref('script')

const productionSteps = [
  { key: 'script', title: '剧本录入', description: '录入项目剧本，准备拆解资产和分镜' },
  { key: 'analysis', title: '资产分析', description: '分析人物、场景、物品和镜头需求' },
  { key: 'assets', title: '创建资产', description: '人物/场景/物品/服装/音频/视频统一上传到 OSS 后可在分镜参考素材中选择使用' },
  { key: 'storyboard', title: '分镜设计', description: '设计分镜并绑定参考资产' },
  { key: 'compose', title: '剪辑合成', description: '批量生成镜头并合成为成片' },
]

const videoModelOptions = [
  { label: 'video-ds-2.0-fast（快速）', value: 'video-ds-2.0-fast' },
  { label: 'video-ds-2.0（标准）', value: 'video-ds-2.0' },
]

const soundAndPictureTogetherOptions = [
  { label: '跟随模型默认', value: '' },
  { label: '启用音画同出', value: 'enabled' },
  { label: '禁用音画同出', value: 'disabled' },
]

const upscaleResolutionOptions = [
  { label: '超分到 1080P', value: '1080p' },
  { label: '超分到 1440P', value: '1440p' },
  { label: '超分到 2160P', value: '2160p' },
]

const selectedVideoModel = ref('video-ds-2.0-fast')
const soundAndPictureTogether = ref('')
const videoEstimateMap = ref<Record<string, number>>({})
const shotVideoVersions = ref<Record<string, ShotVideoVersion[]>>({})
const shotVideoVersionLoadingIds = ref<Set<string>>(new Set())
const shotVideoVersionRefreshIds = ref<Set<string>>(new Set())
const shotVideoVersionBusyIds = ref<Set<string>>(new Set())
const shotVideoFrameExtractingIds = ref<Set<string>>(new Set())
const shotGenerationPollingIds = ref<Set<string>>(new Set())
const shotGenerationPollingTimers = ref<Map<string, number>>(new Map())
const shotVideoThumbnailFailedUrls = ref<Set<string>>(new Set())
const videoGenerationDetailModalVisible = ref(false)
const videoGenerationDetailLoading = ref(false)
const videoGenerationDetailShot = ref<Shot | null>(null)
const videoGenerationDetailVersion = ref<ShotVideoVersion | null>(null)
const videoGenerationDetailReferencesLoaded = ref(false)
const videoGenerationDetailReferences = ref<
  Array<{
    label: string
    type: 'audio' | 'image' | 'video'
    url: string
  }>
>([])
const shotVideoPreviewModalVisible = ref(false)
const shotVideoPreviewUrl = ref('')
const shotVideoPreviewTitle = ref('')
const copyShotVideoVersionModalVisible = ref(false)
const copyShotVideoVersionLoading = ref(false)
const copyShotVideoVersionSourceShot = ref<Shot | null>(null)
const copyShotVideoVersionSourceVersion = ref<ShotVideoVersion | null>(null)
const copyShotVideoVersionMode = ref<'append' | 'assign'>('assign')
const copyShotVideoVersionTargetShotId = ref('')

const currentProductionStepIndex = computed(() =>
  productionSteps.findIndex((step) => step.key === activeProductionStep.value),
)

const activeProductionStepMeta = computed(() =>
  productionSteps.find((step) => step.key === activeProductionStep.value),
)

const copyShotVideoVersionTargetOptions = computed(() => {
  const sourceShotId = copyShotVideoVersionSourceShot.value?.id || ''
  return shots.value
    .filter((shot) => shot.id !== sourceShotId)
    .map((shot, index) => ({
      label: `${shot.orderNum || index + 1}. ${shot.name || shot.actionDescription || `分镜 ${index + 1}`}`,
      value: shot.id,
    }))
})

const completedShotCount = computed(
  () => shots.value.filter((shot) => shot.status === 'completed' && shot.videoUrl).length,
)

const productionPrimaryActionLabel = computed(() => {
  const actionMap: Record<string, string> = {
    script: '编辑项目信息',
    analysis: '查看资产',
    assets: '打开资产库',
    storyboard: '添加分镜',
    compose: '剪辑合成',
  }
  return actionMap[activeProductionStep.value] || '继续制作'
})

const productionSecondaryActionLabel = computed(() => {
  const actionMap: Record<string, string> = {
    script: '进入分镜',
    analysis: '查看场景',
    assets: leftTab.value === 'characters' ? '切到场景' : '切到角色',
    storyboard: '批量生成',
    compose: '返回分镜',
  }
  return actionMap[activeProductionStep.value] || '查看工作区'
})

function setProductionStep(index: number) {
  if (shotLoading.value) return
  activeProductionStep.value = productionSteps[index]?.key || 'script'
}

function handleProductionPrimaryAction() {
  switch (activeProductionStep.value) {
    case 'script': {
      showProjectSettings()
      break
    }
    case 'analysis': {
      leftTab.value = 'characters'
      break
    }
    case 'assets': {
      void openVideoAssetLibrary()
      break
    }
    case 'storyboard': {
      showAddShot()
      break
    }
    case 'compose': {
      void composeVideo()
      break
    }
  }
}

async function openVideoAssetLibrary() {
  await router.push('/video/assets')
}

function handleProductionSecondaryAction() {
  switch (activeProductionStep.value) {
    case 'script': {
      activeProductionStep.value = 'storyboard'
      break
    }
    case 'analysis': {
      leftTab.value = 'scenes'
      break
    }
    case 'assets': {
      leftTab.value = leftTab.value === 'characters' ? 'scenes' : 'characters'
      break
    }
    case 'storyboard': {
      void generateAllShots()
      break
    }
    case 'compose': {
      activeProductionStep.value = 'storyboard'
      break
    }
  }
}

// 加载状态
const composing = ref(false)
const generating = ref(false)
const previewLoading = ref(false)
const projectLoading = ref(false)

// 批量生成和合成状态
const batchProgress = ref({ completed: 0, total: 0, failed: 0 })
const composeProgress = ref(0)

// 对话框状态
const characterModalVisible = ref(false)
const sceneModalVisible = ref(false)
const previewModalVisible = ref(false)
const projectModalVisible = ref(false)
const inlineShotFormVisible = ref(false)

const editingCharacterId = ref('')
const editingSceneId = ref('')
const editingShotId = ref('')

const characterLoading = ref(false)
const sceneLoading = ref(false)
const characterImageUploading = ref(false)
const sceneImageUploading = ref(false)
const sceneVideoUploading = ref(false)
const shotLoading = ref(false)
const shotEditCardRef = ref<HTMLElement | null>(null)
const shotActionBusyIds = ref(new Set<string>())
const generationRequestKeys = ref(new Map<string, string>())
const composeJobId = ref('')

function createGenerationRequestKey() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (char) => {
    const value = Math.floor(Math.random() * 16)
    return (char === 'x' ? value : (value & 0x3) | 0x8).toString(16)
  })
}

function getGenerationRequestKey(shotId: string, forceNew = false) {
  if (forceNew || !generationRequestKeys.value.has(shotId)) {
    generationRequestKeys.value.set(shotId, createGenerationRequestKey())
    generationRequestKeys.value = new Map(generationRequestKeys.value)
  }
  return generationRequestKeys.value.get(shotId)!
}

function clearGenerationRequestKey(shotId: string) {
  generationRequestKeys.value.delete(shotId)
  generationRequestKeys.value = new Map(generationRequestKeys.value)
}
const bindingShotIds = ref(new Set<string>())
const shotAssetUploadingKeys = ref(new Set<string>())
const shotImageAssetLibraryTypes: VideoAssetType[] = ['scene', 'character', 'prop', 'outfit', 'style']
const shotVideoAssetLibraryTypes: VideoAssetType[] = ['video']
const shotAudioAssetLibraryTypes: VideoAssetType[] = ['audio']
const shotAssetPickerOpen = ref(false)
const shotAssetPickerTargetShot = ref<Shot | null>(null)
const shotAssetPickerAssetType = ref<'audio' | 'image' | 'video'>('image')
const shotAssetPickerAllowTypes = ref<VideoAssetType[]>(shotImageAssetLibraryTypes)

const hasOpenShotInlineForm = computed(() => inlineShotFormVisible.value || !!editingShotId.value)
const hasShotActionBusy = computed(
  () => shotActionBusyIds.value.size > 0 || bindingShotIds.value.size > 0,
)
const shotMutationBusy = computed(
  () => shotLoading.value || generating.value || composing.value || hasShotActionBusy.value,
)

function isShotActionBusy(shotId: string) {
  return shotActionBusyIds.value.has(shotId) || bindingShotIds.value.has(shotId)
}

const productionPrimaryLoading = computed(() => {
  if (activeProductionStep.value === 'compose') return composing.value
  if (activeProductionStep.value === 'storyboard') return shotLoading.value
  return generating.value
})

const productionPrimaryDisabled = computed(() => {
  if (shotLoading.value) return true
  if (activeProductionStep.value === 'compose') {
    return generating.value || composing.value || hasOpenShotInlineForm.value || hasShotActionBusy.value
  }
  if (activeProductionStep.value === 'storyboard') {
    return shotMutationBusy.value || hasOpenShotInlineForm.value
  }
  return composing.value
})

const productionSecondaryDisabled = computed(() => {
  if (activeProductionStep.value === 'storyboard') {
    return shotMutationBusy.value || hasOpenShotInlineForm.value
  }
  return shotLoading.value
})

// 表单数据
const projectForm = ref({
  name: '',
  theme: '',
  description: '',
  styleGuide: '',
})

const characterForm = ref({
  name: '',
  description: '',
  referenceImageUrl: '',
  isMain: false,
})

const sceneForm = ref({
  name: '',
  description: '',
  referenceImageUrl: '',
  referenceVideoUrl: '',
})

const characterImagePreviewUrl = useUploadAssetPreviewUrl(
  () => characterForm.value.referenceImageUrl,
  () => accessStore.accessToken,
)
const sceneImagePreviewUrl = useUploadAssetPreviewUrl(
  () => sceneForm.value.referenceImageUrl,
  () => accessStore.accessToken,
)
const sceneVideoPreviewUrl = useUploadAssetPreviewUrl(
  () => sceneForm.value.referenceVideoUrl,
  () => accessStore.accessToken,
)
const assetRefPreview = useUploadAssetPreviewResolver(
  () => accessStore.accessToken,
)

function getUploadErrorMessage(error: any, fallback: string) {
  return (
    error?.response?.data?.error ||
    error?.response?.data?.message ||
    error?.message ||
    fallback
  )
}

function isPublicHttpUrl(url?: string) {
  const value = String(url || '').trim()
  return value.startsWith('http://') || value.startsWith('https://')
}

function requirePublicReferenceUrl(url: string | undefined, label: string) {
  const value = String(url || '').trim()
  if (isPublicHttpUrl(value)) {
    return value
  }
  throw new Error(`${label}需要阿里云 OSS 文件桶公网 objectUrl，请先上传到文件桶并确保 objectUrl 为 http(s) 地址`)
}

function requireUploadedPublicObjectUrl(result: UploadedFile, label: string) {
  return requirePublicReferenceUrl(result.objectUrl, label)
}

async function uploadWorkbenchReference(
  file: File,
  dir: string,
  label: string,
  assign: (url: string) => void,
  uploading: Ref<boolean>,
) {
  uploading.value = true
  try {
    const result = await uploadFileApi(file, dir)
    assign(requireUploadedPublicObjectUrl(result, label))
    message.success(`${label}已上传到阿里云 OSS 文件桶`)
  } catch (error: any) {
    message.error(getUploadErrorMessage(error, `${label}上传失败`))
  } finally {
    uploading.value = false
  }
  return false
}

function uploadCharacterReferenceImage(file: File) {
  return uploadWorkbenchReference(
    file,
    'video/character',
    '角色参考图',
    (url) => {
      characterForm.value.referenceImageUrl = url
    },
    characterImageUploading,
  )
}

function uploadSceneReferenceImage(file: File) {
  return uploadWorkbenchReference(
    file,
    'video/scene',
    '场景参考图',
    (url) => {
      sceneForm.value.referenceImageUrl = url
    },
    sceneImageUploading,
  )
}

function uploadSceneReferenceVideo(file: File) {
  return uploadWorkbenchReference(
    file,
    'video/scene-video',
    '场景参考视频',
    (url) => {
      sceneForm.value.referenceVideoUrl = url
    },
    sceneVideoUploading,
  )
}

function clearCharacterReferenceImage() {
  characterForm.value.referenceImageUrl = ''
}

function clearSceneReferenceImage() {
  sceneForm.value.referenceImageUrl = ''
}

function clearSceneReferenceVideo() {
  sceneForm.value.referenceVideoUrl = ''
}

function previewReferenceAsset(source?: string) {
  return assetRefPreview.resolve(source)
}

function getShotVideoThumbnailUrl(source?: string) {
  const videoUrl = previewReferenceAsset(source)
  if (!videoUrl) return ''
  const separator = videoUrl.includes('?') ? '&' : '?'
  return `${videoUrl}${separator}x-oss-process=video/snapshot,t_1000,f_jpg,w_0,h_0,m_fast`
}

function supportsShotVideoThumbnailSnapshot(source?: string) {
  const videoUrl = previewReferenceAsset(source)
  return Boolean(videoUrl && !shotVideoThumbnailFailedUrls.value.has(videoUrl))
}

function handleShotVideoThumbnailError(source?: string) {
  const videoUrl = previewReferenceAsset(source)
  if (!videoUrl) return
  shotVideoThumbnailFailedUrls.value.add(videoUrl)
  shotVideoThumbnailFailedUrls.value = new Set(shotVideoThumbnailFailedUrls.value)
}

function getInputValue(event: Event) {
  return (event.target as HTMLInputElement | HTMLTextAreaElement | null)?.value || ''
}

function getWorkbenchContentElement() {
  const target = workbenchContentRef.value
  if (target instanceof HTMLElement) return target
  const element = target?.$el
  return element instanceof HTMLElement ? element : null
}

function canScrollElementInDirection(element: HTMLElement, deltaY: number) {
  const maxScrollTop = element.scrollHeight - element.clientHeight
  if (maxScrollTop <= 1) return false
  return deltaY < 0 ? element.scrollTop > 0 : element.scrollTop < maxScrollTop
}

function findScrollableWorkbenchWheelTarget(target: EventTarget | null, root: HTMLElement) {
  let element = target instanceof HTMLElement ? target : null
  while (element && element !== root) {
    const style = window.getComputedStyle(element)
    if (
      (style.overflowY === 'auto' || style.overflowY === 'scroll') &&
      element.scrollHeight > element.clientHeight + 1
    ) {
      return element
    }
    element = element.parentElement
  }
  return null
}

function handleWorkbenchWheel(event: WheelEvent) {
  if (Math.abs(event.deltaY) <= Math.abs(event.deltaX)) return
  const workbenchContent = getWorkbenchContentElement()
  if (!workbenchContent) return
  const root = event.currentTarget instanceof HTMLElement ? event.currentTarget : null
  const eventTargetNode = event.target instanceof Node ? event.target : null
  if (!root || (eventTargetNode && workbenchContent.contains(eventTargetNode))) return

  const scrollableTarget = findScrollableWorkbenchWheelTarget(event.target, root)
  if (scrollableTarget && canScrollElementInDirection(scrollableTarget, event.deltaY)) {
    return
  }

  workbenchContent.scrollTop += event.deltaY
}

function getShotAssetsByType(shot: Shot, assetType: 'audio' | 'image' | 'video') {
  return (shot.shotAssets || []).filter((asset) => asset.assetType === assetType)
}

function getShotReferenceAssetCount(shot: Shot) {
  return (shot.shotAssets || []).length
}

type VideoGenerationDetailReference = {
  label: string
  type: 'audio' | 'image' | 'video'
  url: string
}

function collectShotReferenceAssets(shot: Shot | null): VideoGenerationDetailReference[] {
  if (!shot) return []
  const references: VideoGenerationDetailReference[] = []
  const seen = new Set<string>()
  const pushReference = (type: 'audio' | 'image' | 'video', url?: string, label?: string) => {
    const normalizedUrl = String(url || '').trim()
    if (!normalizedUrl || seen.has(`${type}:${normalizedUrl}`)) return
    seen.add(`${type}:${normalizedUrl}`)
    references.push({
      label: label || (type === 'image' ? '参考图片' : type === 'video' ? '参考视频' : '参考音频'),
      type,
      url: normalizedUrl,
    })
  }

  for (const asset of shot.shotAssets || []) {
    if (asset.assetType === 'image' || asset.assetType === 'video' || asset.assetType === 'audio') {
      pushReference(asset.assetType, asset.objectUrl, asset.name)
    }
  }
  for (const image of shot.usedImages || []) {
    pushReference('image', image, '生成使用图片')
  }
  for (const video of shot.usedVideos || []) {
    pushReference('video', video, '生成使用视频')
  }
  for (const audio of shot.usedAudios || []) {
    pushReference('audio', audio, '生成使用音频')
  }

  return references
}

function getShotReferenceDisplayAssets(shot: Shot | null): VideoGenerationDetailReference[] {
  return collectShotReferenceAssets(shot)
}

function getVideoGenerationDetailReferences(shot: Shot | null): VideoGenerationDetailReference[] {
  if (!shot) return []
  if (videoGenerationDetailShot.value?.id === shot.id && videoGenerationDetailReferencesLoaded.value) {
    return videoGenerationDetailReferences.value
  }
  return collectShotReferenceAssets(shot)
}

function normalizeVideoGenerationDetailReferences(
  references: ShotVideoVersionDetail['references'] = [],
): VideoGenerationDetailReference[] {
  return (references || [])
    .filter((reference) =>
      reference.type === 'image' || reference.type === 'video' || reference.type === 'audio',
    )
    .map((reference) => ({
      label: reference.label,
      type: reference.type as 'audio' | 'image' | 'video',
      url: reference.url,
    }))
}

function applyVideoGenerationDetail(
  detail: ShotVideoVersionDetail,
  fallbackShot: Shot,
  fallbackVersion: ShotVideoVersion,
) {
  videoGenerationDetailShot.value = detail.shot || fallbackShot
  videoGenerationDetailVersion.value = detail.version || fallbackVersion
  videoGenerationDetailReferences.value = normalizeVideoGenerationDetailReferences(detail.references)
  videoGenerationDetailReferencesLoaded.value = true
}

async function reloadOpenVideoGenerationDetail(fallbackShot?: Shot) {
  const shot = fallbackShot || videoGenerationDetailShot.value
  const version = videoGenerationDetailVersion.value
  if (!shot || !version || !videoGenerationDetailModalVisible.value) return

  videoGenerationDetailLoading.value = true
  try {
    const detail = await getShotVideoVersionDetailApi(shot.id, version.id)
    applyVideoGenerationDetail(detail, shot, version)
  } catch (error) {
    message.error('视频生成详情加载失败')
  } finally {
    videoGenerationDetailLoading.value = false
  }
}

function getShotVideoVersions(shot: Shot) {
  return shotVideoVersions.value[shot.id] || []
}

function isShotVideoVersionLoading(shotId: string) {
  return shotVideoVersionLoadingIds.value.has(shotId)
}

function isShotVideoVersionRefreshing(shotId: string) {
  return shotVideoVersionRefreshIds.value.has(shotId)
}

function isShotGenerationPolling(shotId: string) {
  return shotGenerationPollingIds.value.has(shotId)
}

function getShotVideoVersionBusyKey(shotId: string, versionId: string) {
  return `${shotId}:${versionId}`
}

function isShotVideoVersionBusy(shotId: string, versionId: string) {
  return shotVideoVersionBusyIds.value.has(getShotVideoVersionBusyKey(shotId, versionId))
}

function isShotVideoVersionBackup(version: ShotVideoVersion) {
  return Boolean(version.backupFlag)
}

function isShotVideoVersionSubtitleRemoved(version: ShotVideoVersion) {
  return String(version.subtitleRemove || '').trim().toUpperCase() === 'REMOVED'
}

function isShotVideoVersionUpscaled(version: ShotVideoVersion) {
  return Boolean(version.upscaledFlag || String(version.upscaledResolution || '').trim())
}

function canRemoveShotVideoVersionSubtitle(shot: Shot, version: ShotVideoVersion) {
  return Boolean(
    shot &&
    version &&
    !shotMutationBusy.value &&
    !hasOpenShotInlineForm.value &&
    !isShotVideoVersionBusy(shot.id, version.id) &&
    previewReferenceAsset(version.videoUrl) &&
    !isShotVideoVersionSubtitleRemoved(version),
  )
}

function canUpscaleShotVideoVersion(shot: Shot, version: ShotVideoVersion) {
  return Boolean(
    shot &&
    version &&
    !shotMutationBusy.value &&
    !hasOpenShotInlineForm.value &&
    !isShotVideoVersionBusy(shot.id, version.id) &&
    previewReferenceAsset(version.videoUrl),
  )
}

function getShotVideoFrameExtractingKey(shotId: string, versionId: string) {
  return `${shotId}:${versionId}`
}

function isShotVideoFrameExtracting(shotId: string, versionId: string) {
  return shotVideoFrameExtractingIds.value.has(getShotVideoFrameExtractingKey(shotId, versionId))
}

function isUnviewedShotVideoVersion(version: ShotVideoVersion) {
  return Boolean(previewReferenceAsset(version.videoUrl) && !version.viewedFlag)
}

function setLocalShotVideoVersionViewed(shotId: string, versionId: string) {
  const versions = shotVideoVersions.value[shotId] || []
  shotVideoVersions.value = {
    ...shotVideoVersions.value,
    [shotId]: versions.map((item) =>
      item.id === versionId ? { ...item, viewedFlag: true } : item,
    ),
  }
  if (videoGenerationDetailVersion.value?.id === versionId) {
    videoGenerationDetailVersion.value = {
      ...videoGenerationDetailVersion.value,
      viewedFlag: true,
    }
  }
}

async function handleMarkShotVideoVersionViewed(shot: Shot, version: ShotVideoVersion) {
  if (!isUnviewedShotVideoVersion(version)) return
  setLocalShotVideoVersionViewed(shot.id, version.id)
  try {
    await markShotVideoVersionViewedApi(version.id)
  } catch (error) {
    // 已读标记是弱交互：本地先关闭未查看标记，不打断用户观看。
  }
}

function setShotVideoVersionBusy(shotId: string, versionId: string, busy: boolean) {
  const key = getShotVideoVersionBusyKey(shotId, versionId)
  if (busy) {
    shotVideoVersionBusyIds.value.add(key)
  } else {
    shotVideoVersionBusyIds.value.delete(key)
  }
  shotVideoVersionBusyIds.value = new Set(shotVideoVersionBusyIds.value)
}

function setShotVideoFrameExtracting(shotId: string, versionId: string, extracting: boolean) {
  const key = getShotVideoFrameExtractingKey(shotId, versionId)
  if (extracting) {
    shotVideoFrameExtractingIds.value.add(key)
  } else {
    shotVideoFrameExtractingIds.value.delete(key)
  }
  shotVideoFrameExtractingIds.value = new Set(shotVideoFrameExtractingIds.value)
}

function upsertLocalShotVideoVersion(shotId: string, version: ShotVideoVersion) {
  const versions = shotVideoVersions.value[shotId] || []
  const exists = versions.some((item) => item.id === version.id)
  shotVideoVersions.value = {
    ...shotVideoVersions.value,
    [shotId]: exists
      ? versions.map((item) => (item.id === version.id ? { ...item, ...version } : item))
      : [version, ...versions],
  }
}

function setShotVideoVersionLoading(shotId: string, loading: boolean) {
  if (loading) {
    shotVideoVersionLoadingIds.value.add(shotId)
  } else {
    shotVideoVersionLoadingIds.value.delete(shotId)
  }
  shotVideoVersionLoadingIds.value = new Set(shotVideoVersionLoadingIds.value)
}

function setShotVideoVersionRefreshing(shotId: string, refreshing: boolean) {
  if (refreshing) {
    shotVideoVersionRefreshIds.value.add(shotId)
  } else {
    shotVideoVersionRefreshIds.value.delete(shotId)
  }
  shotVideoVersionRefreshIds.value = new Set(shotVideoVersionRefreshIds.value)
}

function stopShotGenerationPolling(shotId: string) {
  const timer = shotGenerationPollingTimers.value.get(shotId)
  if (timer !== undefined) {
    window.clearTimeout(timer)
  }
  shotGenerationPollingTimers.value.delete(shotId)
  shotGenerationPollingTimers.value = new Map(shotGenerationPollingTimers.value)
  shotGenerationPollingIds.value.delete(shotId)
  shotGenerationPollingIds.value = new Set(shotGenerationPollingIds.value)
}

function stopAllShotGenerationPolling() {
  for (const shotId of shotGenerationPollingIds.value) {
    const timer = shotGenerationPollingTimers.value.get(shotId)
    if (timer !== undefined) {
      window.clearTimeout(timer)
    }
  }
  shotGenerationPollingTimers.value = new Map()
  shotGenerationPollingIds.value = new Set()
}

function scheduleShotGenerationPollingTick(shotId: string, tick: () => void, delay: number) {
  const timer = window.setTimeout(tick, delay)
  shotGenerationPollingTimers.value.set(shotId, timer)
  shotGenerationPollingTimers.value = new Map(shotGenerationPollingTimers.value)
}

function startShotGenerationPolling(shotId: string, attempts = 12, delay = 5000) {
  if (!shotId) return
  stopShotGenerationPolling(shotId)
  shotGenerationPollingIds.value.add(shotId)
  shotGenerationPollingIds.value = new Set(shotGenerationPollingIds.value)

  let count = 0
  const tick = async () => {
    shotGenerationPollingTimers.value.delete(shotId)
    shotGenerationPollingTimers.value = new Map(shotGenerationPollingTimers.value)
    count += 1

    const shot = shots.value.find((item) => item.id === shotId)
    if (!shot) {
      stopShotGenerationPolling(shotId)
      return
    }

    await handleRefreshShotVideoVersions(shot, { force: true, silent: true })
    const latest = shots.value.find((item) => item.id === shotId)
    if (
      latest?.videoUrl ||
      latest?.status === 'completed' ||
      latest?.status === 'failed' ||
      count >= attempts
    ) {
	  if (latest?.status === 'completed' || latest?.status === 'failed') {
	    clearGenerationRequestKey(shotId)
	  }
      stopShotGenerationPolling(shotId)
      return
    }

    if (shotGenerationPollingIds.value.has(shotId)) {
      scheduleShotGenerationPollingTick(shotId, tick, delay)
    }
  }

  scheduleShotGenerationPollingTick(shotId, tick, delay)
}

async function loadShotVideoVersions(
  shotsToLoad = shots.value,
  options: { pruneStale?: boolean } = {},
) {
  const shotIds = shotsToLoad.map((shot) => shot.id).filter(Boolean)
  if (shotIds.length === 0) {
    if (options.pruneStale) {
      shotVideoVersions.value = {}
    }
    return
  }

  const nextVersions = { ...shotVideoVersions.value }
  let hasError = false
  await Promise.all(
    shotIds.map(async (shotId) => {
      setShotVideoVersionLoading(shotId, true)
      try {
        nextVersions[shotId] = await listShotVideoVersionsApi(shotId)
      } catch (error) {
        hasError = true
        nextVersions[shotId] = nextVersions[shotId] || []
      } finally {
        setShotVideoVersionLoading(shotId, false)
      }
    }),
  )

  if (options.pruneStale) {
    for (const existingShotId of Object.keys(nextVersions)) {
      if (!shotIds.includes(existingShotId)) {
        delete nextVersions[existingShotId]
      }
    }
  }
  shotVideoVersions.value = nextVersions

  if (hasError) {
    message.error('加载分镜视频版本失败')
  }
}

async function handleRefreshShotVideoVersions(
  shot: Shot,
  options: { force?: boolean; silent?: boolean } = {},
) {
  if (isShotVideoVersionRefreshing(shot.id)) {
    return
  }
  if (!options.force && (shotMutationBusy.value || hasOpenShotInlineForm.value)) {
    if (hasOpenShotInlineForm.value && !options.silent) {
      message.warning('请先保存或取消当前分镜编辑')
    }
    return
  }
  setShotVideoVersionRefreshing(shot.id, true)
  try {
    shotVideoVersions.value = {
      ...shotVideoVersions.value,
      [shot.id]: await refreshShotVideoVersionsApi(shot.id),
    }
    if (!options.silent) {
      message.success('刷新生成状态成功')
    }
    await loadShots()
    const latestShot = shots.value.find((item) => item.id === shot.id) || shot
    if (selectedShot.value?.id === shot.id) {
      selectedShot.value = latestShot
    }
    if (videoGenerationDetailShot.value?.id === shot.id) {
      videoGenerationDetailShot.value = latestShot
    }
    const latestDetailVersion = (shotVideoVersions.value[shot.id] || []).find(
      (item) => item.id === videoGenerationDetailVersion.value?.id,
    )
    if (latestDetailVersion) {
      videoGenerationDetailVersion.value = latestDetailVersion
    }
  } catch (error) {
    if (!options.silent) {
      message.error('刷新视频版本失败')
    }
  } finally {
    setShotVideoVersionRefreshing(shot.id, false)
  }
}

async function handleOpenVideoGenerationDetail(shot: Shot, version: ShotVideoVersion) {
  videoGenerationDetailShot.value = shot
  videoGenerationDetailVersion.value = version
  videoGenerationDetailReferences.value = []
  videoGenerationDetailReferencesLoaded.value = false
  videoGenerationDetailModalVisible.value = true
  void handleMarkShotVideoVersionViewed(shot, version)
  videoGenerationDetailLoading.value = true
  try {
    const detail = await getShotVideoVersionDetailApi(shot.id, version.id)
    applyVideoGenerationDetail(detail, shot, version)
  } catch (error) {
    message.error('视频生成详情加载失败')
  } finally {
    videoGenerationDetailLoading.value = false
  }
}

function handleCloseVideoGenerationDetail() {
  videoGenerationDetailModalVisible.value = false
  videoGenerationDetailShot.value = null
  videoGenerationDetailVersion.value = null
  videoGenerationDetailReferences.value = []
  videoGenerationDetailReferencesLoaded.value = false
}

function handleOpenShotVideoPreview(shot: Shot, version: ShotVideoVersion) {
  const videoUrl = previewReferenceAsset(version.videoUrl)
  if (!videoUrl) {
    message.warning('当前视频版本暂无可预览视频')
    return
  }
  shotVideoPreviewUrl.value = videoUrl
  shotVideoPreviewTitle.value = `${shot.name || shot.actionDescription || '分镜视频'} · 视频预览`
  shotVideoPreviewModalVisible.value = true
  void handleMarkShotVideoVersionViewed(shot, version)
}

function handleCloseShotVideoPreview() {
  shotVideoPreviewModalVisible.value = false
  shotVideoPreviewUrl.value = ''
  shotVideoPreviewTitle.value = ''
}

async function handleUseVideoGenerationDetailVersion() {
  const shot = videoGenerationDetailShot.value
  const version = videoGenerationDetailVersion.value
  if (!shot || !version) return
  await handleUseShotVideoVersion(shot, version)
  handleCloseVideoGenerationDetail()
}

async function handleRegenerateVideoGenerationDetailVersion() {
  const shot = videoGenerationDetailShot.value
  const version = videoGenerationDetailVersion.value
  if (!shot || !version) return
  await handleRegenerateShotVideoVersion(shot, version)
}

async function handleReeditVideoGenerationDetailVersion() {
  const shot = videoGenerationDetailShot.value
  const version = videoGenerationDetailVersion.value
  if (!shot || !version) return
  await handleReeditShotVideoVersion(shot, version)
  handleCloseVideoGenerationDetail()
}

function handleCopyVideoGenerationDetailVersion() {
  const shot = videoGenerationDetailShot.value
  const version = videoGenerationDetailVersion.value
  if (!shot || !version) return
  handleOpenCopyShotVideoVersion(shot, version)
  if (copyShotVideoVersionModalVisible.value) {
    handleCloseVideoGenerationDetail()
  }
}

async function handleRemoveSubtitleVideoGenerationDetailVersion() {
  const shot = videoGenerationDetailShot.value
  const version = videoGenerationDetailVersion.value
  if (!shot || !version) return
  await handleRemoveShotVideoVersionSubtitle(shot, version)
}

async function handleUpscaleVideoGenerationDetailVersion(resolution: string) {
  const shot = videoGenerationDetailShot.value
  const version = videoGenerationDetailVersion.value
  if (!shot || !version) return
  await handleUpscaleShotVideoVersion(shot, version, resolution)
}

async function handleExtractShotVideoFrame(shot: Shot, version: ShotVideoVersion) {
  if (hasOpenShotInlineForm.value) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  if (shotMutationBusy.value || isShotVideoFrameExtracting(shot.id, version.id)) return
  if (!previewReferenceAsset(version.videoUrl)) {
    message.warning('当前视频版本还没有可抽帧的视频')
    return
  }

  setShotVideoFrameExtracting(shot.id, version.id, true)
  try {
    // 后端会把抽帧图片上传到 video/frames，并通过 createShotAssetApi 等价逻辑写回分镜参考素材。
    await extractShotVideoFrameApi(shot.id, version.id)
    message.success('视频抽帧已保存到分镜参考素材')
    await loadShots()
    await syncShotReferenceAssetChange(shot.id)
  } catch (error) {
    message.error('视频抽帧失败')
  } finally {
    setShotVideoFrameExtracting(shot.id, version.id, false)
  }
}

function handleOpenCopyShotVideoVersion(shot: Shot, version: ShotVideoVersion) {
  if (shotMutationBusy.value || hasOpenShotInlineForm.value) {
    if (hasOpenShotInlineForm.value) {
      message.warning('请先保存或取消当前分镜编辑')
    }
    return
  }
  const targets = shots.value.filter((item) => item.id !== shot.id)
  copyShotVideoVersionSourceShot.value = shot
  copyShotVideoVersionSourceVersion.value = version
  copyShotVideoVersionMode.value = targets.length > 0 ? 'assign' : 'append'
  copyShotVideoVersionTargetShotId.value = targets[0]?.id || ''
  copyShotVideoVersionModalVisible.value = true
}

async function handleCopyShotVideoVersion() {
  const sourceShot = copyShotVideoVersionSourceShot.value
  const sourceVersion = copyShotVideoVersionSourceVersion.value
  const targetShotId = copyShotVideoVersionTargetShotId.value
  if (copyShotVideoVersionMode.value === 'append') {
    await handleCopyShotVideoVersionToNewShot()
    return
  }
  if (!sourceShot || !sourceVersion || !targetShotId) {
    message.warning('请选择目标分镜')
    return
  }
  if (sourceShot.id === targetShotId) {
    message.warning('不能复制到当前分镜')
    return
  }

  copyShotVideoVersionLoading.value = true
  setShotVideoVersionBusy(sourceShot.id, sourceVersion.id, true)
  try {
    await copyShotVideoVersionApi(sourceShot.id, sourceVersion.id, targetShotId)
    message.success('已复制到目标分镜')
    copyShotVideoVersionModalVisible.value = false
    copyShotVideoVersionSourceShot.value = null
    copyShotVideoVersionSourceVersion.value = null
    copyShotVideoVersionTargetShotId.value = ''
    copyShotVideoVersionMode.value = 'assign'
    await loadShots()
    const latestTargetShot = shots.value.find((item) => item.id === targetShotId)
    if (latestTargetShot) {
      await loadShotVideoVersions([latestTargetShot])
      if (selectedShot.value?.id === targetShotId) {
        selectedShot.value = latestTargetShot
      }
    }
  } catch (error) {
    message.error('复制到分镜失败')
  } finally {
    copyShotVideoVersionLoading.value = false
    setShotVideoVersionBusy(sourceShot.id, sourceVersion.id, false)
  }
}

async function handleCopyShotVideoVersionToNewShot() {
  const sourceShot = copyShotVideoVersionSourceShot.value
  const sourceVersion = copyShotVideoVersionSourceVersion.value
  if (!sourceShot || !sourceVersion) {
    message.warning('请选择要复制的视频版本')
    return
  }

  copyShotVideoVersionLoading.value = true
  setShotVideoVersionBusy(sourceShot.id, sourceVersion.id, true)
  try {
    const createdShot = await createShotApi(projectId.value, {
      ...shotToPayload(sourceShot),
      name: `${sourceShot.name || '复制分镜'} 副本`,
      orderNum: getNextShotOrderNum(),
    })
    await copyShotVideoVersionApi(sourceShot.id, sourceVersion.id, createdShot.id)
    message.success('已新增分镜并复制视频版本')
    copyShotVideoVersionModalVisible.value = false
    copyShotVideoVersionSourceShot.value = null
    copyShotVideoVersionSourceVersion.value = null
    copyShotVideoVersionTargetShotId.value = ''
    copyShotVideoVersionMode.value = 'assign'
    await loadShots()
    const latestCreatedShot = shots.value.find((item) => item.id === createdShot.id)
    await loadShotVideoVersions([latestCreatedShot || createdShot])
    selectedShot.value = latestCreatedShot || createdShot
  } catch (error) {
    message.error('新增分镜并复制视频版本失败')
  } finally {
    copyShotVideoVersionLoading.value = false
    setShotVideoVersionBusy(sourceShot.id, sourceVersion.id, false)
  }
}

async function handleUseShotVideoVersion(shot: Shot, version: ShotVideoVersion) {
  await handleSetShotVideoVersion(shot, version)
}

async function handleRegenerateShotVideoVersion(shot: Shot, version: ShotVideoVersion) {
  if (hasOpenShotInlineForm.value) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  if (shotMutationBusy.value || isShotVideoVersionBusy(shot.id, version.id)) return
  if (!validateShotVideoGenerationParams(shot)) return
  setShotVideoVersionBusy(shot.id, version.id, true)
  try {
    const requestKey = getGenerationRequestKey(shot.id, true)
    await generateShotApi(shot.id, { requestKey })
    message.success('已提交重新生成任务，请稍后查看结果')
    await loadShots()
    const latestShot = shots.value.find((item) => item.id === shot.id) || shot
    await loadShotVideoVersions([latestShot || shot])
    if (selectedShot.value?.id === shot.id) {
      selectedShot.value = latestShot
    }
    if (videoGenerationDetailShot.value?.id === shot.id) {
      videoGenerationDetailShot.value = latestShot
    }
    startShotGenerationPolling(shot.id)
  } catch (error) {
    message.error('重新生成失败')
  } finally {
    setShotVideoVersionBusy(shot.id, version.id, false)
  }
}

async function handleReeditShotVideoVersion(shot: Shot, version: ShotVideoVersion) {
  if (hasOpenShotInlineForm.value) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  if (shotMutationBusy.value || isShotVideoVersionBusy(shot.id, version.id)) return
  const nextDynamicDescription = (version.prompt || shot.dynamicDescription || shot.generatedPrompt || '').trim()
  if (!nextDynamicDescription) {
    message.warning('该版本没有可重编辑的提示词')
    return
  }

  setShotVideoVersionBusy(shot.id, version.id, true)
  try {
    const updated = await updateShotApi(shot.id, {
      ...shotToPayload(shot),
      dynamicDescription: nextDynamicDescription,
    })
    const target = shots.value.find((item) => item.id === shot.id)
    if (target) {
      Object.assign(target, updated)
      videoEstimateMap.value = {
        ...videoEstimateMap.value,
        [shot.id]: estimateShotVideoPoints(target),
      }
    }
    selectedShot.value = shots.value.find((item) => item.id === shot.id) || updated
    activeProductionStep.value = 'storyboard'
    message.success('已载入该版本提示词，可在动态描述中重编辑')
  } catch (error) {
    message.error('重编辑失败')
  } finally {
    setShotVideoVersionBusy(shot.id, version.id, false)
  }
}

async function handleSetShotVideoVersion(shot: Shot, version: ShotVideoVersion) {
  if (version.isCurrent || shotMutationBusy.value || hasOpenShotInlineForm.value) return
  setShotVideoVersionBusy(shot.id, version.id, true)
  try {
    await setShotVideoVersionApi(shot.id, version.id)
    message.success('已设为当前版本')
    await loadShots()
    const latestShot = shots.value.find((item) => item.id === shot.id) || shot
    await loadShotVideoVersions([latestShot || shot])
    if (selectedShot.value?.id === shot.id) {
      selectedShot.value = latestShot
    }
    if (videoGenerationDetailShot.value?.id === shot.id) {
      videoGenerationDetailShot.value = latestShot
    }
    const latestVersion = (shotVideoVersions.value[shot.id] || []).find((item) => item.id === version.id)
    if (videoGenerationDetailVersion.value?.id === version.id && latestVersion) {
      videoGenerationDetailVersion.value = latestVersion
    }
  } catch (error) {
    message.error('设置当前版本失败')
  } finally {
    setShotVideoVersionBusy(shot.id, version.id, false)
  }
}

async function handleSetShotVideoVersionBackup(shot: Shot, version: ShotVideoVersion) {
  if (shotMutationBusy.value || hasOpenShotInlineForm.value || isShotVideoVersionBusy(shot.id, version.id)) {
    if (hasOpenShotInlineForm.value) {
      message.warning('请先保存或取消当前分镜编辑')
    }
    return
  }
  const nextBackupFlag = !isShotVideoVersionBackup(version)
  setShotVideoVersionBusy(shot.id, version.id, true)
  try {
    const updated = await setShotVideoVersionBackupApi(shot.id, version.id, nextBackupFlag)
    const versions = shotVideoVersions.value[shot.id] || []
    shotVideoVersions.value = {
      ...shotVideoVersions.value,
      [shot.id]: versions.map((item) => (item.id === version.id ? { ...item, ...updated } : item)),
    }
    if (videoGenerationDetailVersion.value?.id === version.id) {
      videoGenerationDetailVersion.value = { ...videoGenerationDetailVersion.value, ...updated }
    }
    message.success(nextBackupFlag ? '已设为备选视频' : '已取消备选视频')
  } catch (error) {
    message.error(nextBackupFlag ? '设为备选失败' : '取消备选失败')
  } finally {
    setShotVideoVersionBusy(shot.id, version.id, false)
  }
}

async function handleRemoveShotVideoVersionSubtitle(shot: Shot, version: ShotVideoVersion) {
  if (!canRemoveShotVideoVersionSubtitle(shot, version)) {
    if (hasOpenShotInlineForm.value) {
      message.warning('请先保存或取消当前分镜编辑')
    } else if (isShotVideoVersionSubtitleRemoved(version)) {
      message.warning('当前视频版本已经是无字幕版本')
    } else if (!previewReferenceAsset(version.videoUrl)) {
      message.warning('当前视频版本还没有可擦字幕的视频')
    }
    return
  }

  setShotVideoVersionBusy(shot.id, version.id, true)
  try {
    const updated = await removeShotVideoVersionSubtitleApi(shot.id, version.id)
    upsertLocalShotVideoVersion(shot.id, updated)
    if (videoGenerationDetailVersion.value?.id === version.id) {
      videoGenerationDetailVersion.value = updated
    }
    message.success('已生成无字幕视频版本')
    await loadShotVideoVersions([shot])
  } catch (error) {
    message.error('擦除字幕失败')
  } finally {
    setShotVideoVersionBusy(shot.id, version.id, false)
  }
}

async function handleUpscaleShotVideoVersion(shot: Shot, version: ShotVideoVersion, resolution: string) {
  if (!canUpscaleShotVideoVersion(shot, version)) {
    if (hasOpenShotInlineForm.value) {
      message.warning('请先保存或取消当前分镜编辑')
    } else if (!previewReferenceAsset(version.videoUrl)) {
      message.warning('当前视频版本还没有可超分的视频')
    }
    return
  }

  setShotVideoVersionBusy(shot.id, version.id, true)
  try {
    const updated = await upscaleShotVideoVersionApi(shot.id, version.id, { resolution })
    upsertLocalShotVideoVersion(shot.id, updated)
    if (videoGenerationDetailVersion.value?.id === version.id) {
      videoGenerationDetailVersion.value = updated
    }
    message.success('已生成超分视频版本')
    await loadShotVideoVersions([shot])
  } catch (error) {
    message.error('超分辨率处理失败')
  } finally {
    setShotVideoVersionBusy(shot.id, version.id, false)
  }
}

function handleDeleteShotVideoVersion(shot: Shot, version: ShotVideoVersion) {
  if (shotMutationBusy.value || hasOpenShotInlineForm.value) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  Modal.confirm({
    title: '删除视频版本',
    content: version.isCurrent
      ? '当前版本删除后会切换到最近的可用版本；如果没有其他版本，该分镜会回到草稿状态。'
      : '确定要删除这个视频版本吗？',
    okText: '删除版本',
    okButtonProps: { danger: true },
    onOk: async () => {
      setShotVideoVersionBusy(shot.id, version.id, true)
      try {
        await deleteShotVideoVersionApi(shot.id, version.id)
        message.success('视频版本已删除')
        await loadShots()
        const latestShot = shots.value.find((item) => item.id === shot.id) || shot
        await loadShotVideoVersions([latestShot || shot])
        if (selectedShot.value?.id === shot.id) {
          selectedShot.value = latestShot
        }
        if (videoGenerationDetailShot.value?.id === shot.id) {
          videoGenerationDetailShot.value = latestShot
        }
        if (videoGenerationDetailVersion.value?.id === version.id) {
          handleCloseVideoGenerationDetail()
        }
      } catch (error) {
        message.error('删除视频版本失败')
      } finally {
        setShotVideoVersionBusy(shot.id, version.id, false)
      }
    },
  })
}

function estimateShotVideoPoints(shot: Shot) {
  const duration = Number(shot.duration || 15)
  const modelWeight = (shot.videoModel || selectedVideoModel.value).includes('fast') ? 1 : 1.4
  const resolutionWeight = shot.videoResolution === '1080p' ? 1.5 : 1
  const audioWeight = shot.soundAndPictureTogether === 'enabled' ? 1.2 : 1
  const referenceWeight = Math.min(getShotReferenceAssetCount(shot), 4) * 2
  return Math.ceil((duration / 5) * 10 * modelWeight * resolutionWeight * audioWeight + referenceWeight)
}

function getShotVideoPrompt(shot: Shot) {
  return String(shot.dynamicDescription || shot.generatedPrompt || '').trim()
}

function hasShotVideoGenerationParams(shot: Shot) {
  return Boolean(
    getShotVideoPrompt(shot) &&
      String(shot.videoModel || selectedVideoModel.value || '').trim() &&
      String(shot.videoResolution || '720p').trim() &&
      Number(shot.duration || 0) > 0,
  )
}

function isShotGenerationLocked(shot: Shot) {
  return shot.status === 'generating' || isShotActionBusy(shot.id) || isShotGenerationPolling(shot.id)
}

function validateShotVideoGenerationParams(shot: Shot) {
  if (!getShotVideoPrompt(shot)) {
    message.warning('请输入视频描述')
    return false
  }
  if (!String(shot.videoModel || selectedVideoModel.value || '').trim()) {
    message.warning('请选择生视频模型')
    return false
  }
  if (!String(shot.videoResolution || '720p').trim()) {
    message.warning('请选择分辨率')
    return false
  }
  if (Number(shot.duration || 0) <= 0) {
    message.warning('请选择视频时长')
    return false
  }
  if (isShotGenerationLocked(shot)) {
    message.warning('该分镜正在生成视频，请稍候')
    return false
  }
  return true
}

function getGeneratableShots() {
  return shots.value.filter((shot) => hasShotVideoGenerationParams(shot) && !isShotGenerationLocked(shot))
}

function getShotAssetUploadingKey(shotId: string, assetType: string) {
  return `${shotId}:${assetType}`
}

function isShotAssetUploading(shotId: string, assetType: string) {
  return shotAssetUploadingKeys.value.has(getShotAssetUploadingKey(shotId, assetType))
}

function getShotAssetPickerAllowTypes(assetType: 'audio' | 'image' | 'video'): VideoAssetType[] {
  if (assetType === 'video') return shotVideoAssetLibraryTypes
  if (assetType === 'audio') return shotAudioAssetLibraryTypes
  return shotImageAssetLibraryTypes
}

function openShotAssetPicker(shot: Shot, assetType: 'audio' | 'image' | 'video') {
  if (shotMutationBusy.value || hasOpenShotInlineForm.value) {
    if (hasOpenShotInlineForm.value) {
      message.warning('请先保存或取消当前分镜编辑')
    }
    return
  }
  shotAssetPickerTargetShot.value = shot
  shotAssetPickerAssetType.value = assetType
  shotAssetPickerAllowTypes.value = getShotAssetPickerAllowTypes(assetType)
  shotAssetPickerOpen.value = true
}

function getPickedShotAssetType(asset: VideoAsset): 'audio' | 'image' | 'video' {
  if (asset.type === 'video') return 'video'
  if (asset.type === 'audio') return 'audio'
  return 'image'
}

async function handlePickShotAsset(asset: VideoAsset) {
  const targetShot = shotAssetPickerTargetShot.value
  if (!targetShot) {
    message.warning('请选择要添加参考素材的分镜')
    return
  }
  let objectUrl = ''
  try {
    objectUrl = requirePublicReferenceUrl(getAssetPreviewSource(asset), '资产库素材')
  } catch (error: any) {
    message.warning(error?.message || '该资产没有可用的 OSS 公网预览地址')
    return
  }
  const assetType = getPickedShotAssetType(asset)
  const key = getShotAssetUploadingKey(targetShot.id, assetType)
  if (shotAssetUploadingKeys.value.has(key)) return
  shotAssetUploadingKeys.value.add(key)
  shotAssetUploadingKeys.value = new Set(shotAssetUploadingKeys.value)
  try {
    await createShotAssetApi(targetShot.id, {
      assetType,
      mimeType: '',
      name: asset.name || (assetType === 'image' ? '资产库图片' : assetType === 'video' ? '资产库视频' : '资产库音频'),
      objectUrl,
      sizeBytes: 0,
    })
    message.success('已从资产库添加到分镜参考素材')
    shotAssetPickerOpen.value = false
    await loadShots()
    await syncShotReferenceAssetChange(targetShot.id)
  } catch (error) {
    message.error('添加资产库素材失败')
  } finally {
    shotAssetUploadingKeys.value.delete(key)
    shotAssetUploadingKeys.value = new Set(shotAssetUploadingKeys.value)
  }
}

async function uploadShotReferenceAsset(
  shot: Shot,
  file: File,
  assetType: 'audio' | 'image' | 'video',
  dir: string,
  label: string,
) {
  const key = getShotAssetUploadingKey(shot.id, assetType)
  if (shotAssetUploadingKeys.value.has(key)) return false
  shotAssetUploadingKeys.value.add(key)
  shotAssetUploadingKeys.value = new Set(shotAssetUploadingKeys.value)
  try {
    const result = await uploadFileApi(file, dir)
    await createShotAssetApi(shot.id, {
      assetType,
      mimeType: file.type || '',
      name: file.name || label,
      objectUrl: requireUploadedPublicObjectUrl(result, label),
      sizeBytes: file.size || 0,
    })
    message.success(`${label}已上传到阿里云 OSS 文件桶`)
    await loadShots()
    await syncShotReferenceAssetChange(shot.id)
  } catch (error: any) {
    message.error(getUploadErrorMessage(error, `${label}上传失败`))
  } finally {
    shotAssetUploadingKeys.value.delete(key)
    shotAssetUploadingKeys.value = new Set(shotAssetUploadingKeys.value)
  }
  return false
}

async function syncShotReferenceAssetChange(shotId: string) {
  const latestShot = shots.value.find((item) => item.id === shotId)
  if (!latestShot) return
  if (selectedShot.value?.id === shotId) {
    selectedShot.value = latestShot
  }
  if (videoGenerationDetailShot.value?.id === shotId) {
    videoGenerationDetailShot.value = latestShot
    await reloadOpenVideoGenerationDetail(latestShot)
  }
}

function uploadShotReferenceImage(shot: Shot, file: File) {
  return uploadShotReferenceAsset(shot, file, 'image', 'video/shot-image', '分镜参考图片')
}

function uploadShotReferenceVideo(shot: Shot, file: File) {
  return uploadShotReferenceAsset(shot, file, 'video', 'video/shot-video', '分镜参考视频')
}

function uploadShotReferenceAudio(shot: Shot, file: File) {
  return uploadShotReferenceAsset(shot, file, 'audio', 'video/shot-audio', '分镜参考音频')
}

function deleteShotAsset(asset: ShotAsset) {
  Modal.confirm({
    title: '确认删除',
    content: '确定要删除这个分镜参考素材吗？',
    onOk: async () => {
      try {
        await deleteShotAssetApi(asset.id)
        message.success('删除成功')
        await loadShots()
        await syncShotReferenceAssetChange(asset.shotId)
      } catch (error) {
        message.error('删除失败')
      }
    },
  })
}

async function handleVideoGenerationParamsChange(shot: Shot, patch: Partial<Shot>) {
  const payload = {
    ...shotToPayload(shot),
    ...patch,
  }
  try {
    const updated = await updateShotApi(shot.id, payload)
    const target = shots.value.find((item) => item.id === shot.id)
    if (target) {
      Object.assign(target, updated)
      videoEstimateMap.value = {
        ...videoEstimateMap.value,
        [shot.id]: estimateShotVideoPoints(target),
      }
    }
    if (selectedShot.value?.id === shot.id) {
      selectedShot.value = shots.value.find((item) => item.id === shot.id) || updated
    }
    message.success('视频生成参数已保存')
  } catch (error) {
    message.error('视频生成参数保存失败')
  }
}

function createEmptyShotForm() {
  return {
    name: '',
    scriptOriginalContent: '',
    actionDescription: '',
    dynamicDescription: '',
    videoModel: selectedVideoModel.value,
    videoResolution: '720p',
    soundAndPictureTogether: soundAndPictureTogether.value,
    duration: 15,
    aspectRatio: '16:9',
    characterIds: [] as string[],
    sceneId: '',
    cameraMovement: '',
  }
}

const shotForm = ref(createEmptyShotForm())

function getNextShotOrderNum() {
  return Math.max(0, ...shots.value.map((shot) => shot.orderNum || 0)) + 1
}

async function scrollShotInlineFormIntoView() {
  await nextTick()
  const target =
    shotEditCardRef.value || document.querySelector<HTMLElement>('.shot-card.editing')

  target?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  target
    ?.querySelector<HTMLInputElement | HTMLTextAreaElement>('textarea, input')
    ?.focus()
}

// 获取项目ID
const projectId = computed(() => route.params.id as string)

// 加载数据
async function loadProject() {
  try {
    project.value = await getProjectApi(projectId.value)
  } catch (error) {
    message.error('加载项目失败')
  }
}

async function loadCharacters() {
  try {
    characters.value = await listCharactersApi(projectId.value)
  } catch (error) {
    message.error('加载角色失败')
  }
}

async function loadScenes() {
  try {
    scenes.value = await listScenesApi(projectId.value)
  } catch (error) {
    message.error('加载场景失败')
  }
}

async function loadShots() {
  try {
    const previousSelectedShotId = selectedShot.value?.id
    shots.value = await listShotsApi(projectId.value)
    if (previousSelectedShotId) {
      selectedShot.value =
        shots.value.find((shot) => shot.id === previousSelectedShotId) || shots.value[0] || null
    } else {
      selectedShot.value = shots.value[0] || null
    }
    await loadShotVideoVersions(shots.value, { pruneStale: true })
  } catch (error) {
    message.error('加载分镜失败')
  }
}

// 角色管理
function showAddCharacter() {
  editingCharacterId.value = ''
  characterForm.value = {
    name: '',
    description: '',
    referenceImageUrl: '',
    isMain: false,
  }
  characterModalVisible.value = true
}

async function handleAddCharacter() {
  if (!characterForm.value.name || !characterForm.value.description) {
    message.warning('请填写必填项')
    return
  }

  try {
    characterLoading.value = true
    if (editingCharacterId.value) {
      await updateCharacterApi(editingCharacterId.value, characterForm.value)
      message.success('更新成功')
    } else {
      await createCharacterApi(projectId.value, characterForm.value)
      message.success('添加成功')
    }
    editingCharacterId.value = ''
    characterModalVisible.value = false
    await loadCharacters()
  } catch (error) {
    message.error('添加失败')
  } finally {
    characterLoading.value = false
  }
}

function editCharacter(char: Character) {
  editingCharacterId.value = char.id
  characterForm.value = {
    name: char.name,
    description: char.description,
    referenceImageUrl: char.referenceImageUrl,
    isMain: char.isMain,
  }
  characterModalVisible.value = true
}

async function deleteCharacter(charId: string) {
  Modal.confirm({
    title: '确认删除',
    content: '确定要删除这个角色吗？',
    onOk: async () => {
      try {
        await deleteCharacterApi(charId)
        message.success('删除成功')
        await loadCharacters()
      } catch (error) {
        message.error('删除失败')
      }
    },
  })
}

// 场景管理
function showAddScene() {
  editingSceneId.value = ''
  sceneForm.value = {
    name: '',
    description: '',
    referenceImageUrl: '',
    referenceVideoUrl: '',
  }
  sceneModalVisible.value = true
}

async function handleAddScene() {
  if (!sceneForm.value.name || !sceneForm.value.description) {
    message.warning('请填写必填项')
    return
  }

  try {
    sceneLoading.value = true
    if (editingSceneId.value) {
      await updateSceneApi(editingSceneId.value, sceneForm.value)
      message.success('更新成功')
    } else {
      await createSceneApi(projectId.value, sceneForm.value)
      message.success('添加成功')
    }
    editingSceneId.value = ''
    sceneModalVisible.value = false
    await loadScenes()
  } catch (error) {
    message.error('添加失败')
  } finally {
    sceneLoading.value = false
  }
}

function editScene(scene: Scene) {
  editingSceneId.value = scene.id
  sceneForm.value = {
    name: scene.name,
    description: scene.description,
    referenceImageUrl: scene.referenceImageUrl,
    referenceVideoUrl: scene.referenceVideoUrl,
  }
  sceneModalVisible.value = true
}

async function deleteScene(sceneId: string) {
  Modal.confirm({
    title: '确认删除',
    content: '确定要删除这个场景吗？',
    onOk: async () => {
      try {
        await deleteSceneApi(sceneId)
        message.success('删除成功')
        await loadScenes()
      } catch (error) {
        message.error('删除失败')
      }
    },
  })
}

// 分镜管理
function showAddShot() {
  if (shotMutationBusy.value) return
  if (hasOpenShotInlineForm.value) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  editingShotId.value = ''
  shotForm.value = createEmptyShotForm()
  inlineShotFormVisible.value = true
  activeProductionStep.value = 'storyboard'
  void scrollShotInlineFormIntoView()
}

async function handleAddShot() {
  if (shotLoading.value) return
  const actionDescription = shotForm.value.actionDescription.trim()
  if (!actionDescription) {
    message.warning('请填写动作描述')
    return
  }

  try {
    shotLoading.value = true
    const payload = {
      ...shotForm.value,
      actionDescription,
    }
    let savedShot: null | Shot = null
    if (editingShotId.value) {
      const originalShot = shots.value.find((shot) => shot.id === editingShotId.value)
      savedShot = await updateShotApi(editingShotId.value, {
        ...(originalShot ? shotToPayload(originalShot) : {}),
        ...payload,
      })
      message.success('更新成功')
    } else {
      savedShot = await createShotApi(projectId.value, {
        ...payload,
        orderNum: getNextShotOrderNum(),
      })
      message.success('添加成功')
    }
    editingShotId.value = ''
    inlineShotFormVisible.value = false
    await loadShots()
    if (savedShot?.id) {
      selectedShot.value = shots.value.find((shot) => shot.id === savedShot?.id) || savedShot
    }
  } catch (error) {
    message.error('保存分镜失败')
  } finally {
    shotLoading.value = false
  }
}

function editShot(shot: Shot) {
  if (shotMutationBusy.value) return
  if (hasOpenShotInlineForm.value && editingShotId.value !== shot.id) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  editingShotId.value = shot.id
  shotForm.value = {
    name: shot.name,
    scriptOriginalContent: shot.scriptOriginalContent || '',
    actionDescription: shot.actionDescription,
    dynamicDescription: shot.dynamicDescription || '',
    videoModel: shot.videoModel || selectedVideoModel.value,
    videoResolution: shot.videoResolution || '720p',
    soundAndPictureTogether: shot.soundAndPictureTogether || soundAndPictureTogether.value,
    duration: shot.duration || 15,
    aspectRatio: shot.aspectRatio || '16:9',
    characterIds: [...(shot.characterIds || [])],
    sceneId: shot.sceneId || '',
    cameraMovement: shot.cameraMovement || '',
  }
  inlineShotFormVisible.value = true
  activeProductionStep.value = 'storyboard'
  void scrollShotInlineFormIntoView()
}

function cancelShotInlineEdit() {
  if (shotLoading.value) return
  editingShotId.value = ''
  inlineShotFormVisible.value = false
  shotForm.value = createEmptyShotForm()
}

async function deleteShot(shotId: string) {
  if (shotMutationBusy.value) return
  if (hasOpenShotInlineForm.value) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  Modal.confirm({
    title: '确认删除',
    content: '确定要删除这个分镜吗？',
    onOk: async () => {
      if (shotMutationBusy.value || hasOpenShotInlineForm.value) {
        message.warning('请先保存或取消当前分镜编辑')
        return
      }
      shotActionBusyIds.value.add(shotId)
      shotActionBusyIds.value = new Set(shotActionBusyIds.value)
      try {
        await deleteShotApi(shotId)
        if (editingShotId.value === shotId) {
          cancelShotInlineEdit()
        }
        if (selectedShot.value?.id === shotId) {
          selectedShot.value = null
        }
        message.success('删除成功')
        await loadShots()
      } catch (error) {
        message.error('删除失败')
      } finally {
        shotActionBusyIds.value.delete(shotId)
        shotActionBusyIds.value = new Set(shotActionBusyIds.value)
      }
    },
  })
}

function selectShot(shot: Shot) {
  selectedShot.value = shot
}

async function handleScriptOriginalContentChange(shot: Shot, scriptOriginalContent: string) {
  const nextValue = scriptOriginalContent.trim()
  if ((shot.scriptOriginalContent || '') === nextValue) return
  try {
    const updated = await updateShotApi(shot.id, {
      ...shotToPayload(shot),
      scriptOriginalContent: nextValue,
    })
    const target = shots.value.find((item) => item.id === shot.id)
    if (target) {
      target.scriptOriginalContent = updated.scriptOriginalContent
    }
    if (selectedShot.value?.id === shot.id) {
      selectedShot.value = {
        ...selectedShot.value,
        scriptOriginalContent: updated.scriptOriginalContent,
      }
    }
    message.success('分镜剧本已保存')
  } catch (error) {
    message.error('分镜剧本保存失败')
  }
}

// 预览和生成
async function previewShot(shot: Shot) {
  try {
    previewLoading.value = true
    previewModalVisible.value = true
    const result = await previewShotPromptApi(shot.id)
    shotPreview.value = result
  } catch (error) {
    message.error('预览失败')
    previewModalVisible.value = false
  } finally {
    previewLoading.value = false
  }
}

async function generateShot(shot: Shot) {
  if (hasOpenShotInlineForm.value) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  if (shotMutationBusy.value) return
  if (!validateShotVideoGenerationParams(shot)) return
  shotActionBusyIds.value.add(shot.id)
  shotActionBusyIds.value = new Set(shotActionBusyIds.value)
  try {
    const requestKey = getGenerationRequestKey(shot.id)
    await generateShotApi(shot.id, { requestKey })
    message.success('开始生成视频')
    await loadShots()
    const latestShot = shots.value.find((item) => item.id === shot.id) || shot
    await loadShotVideoVersions([latestShot || shot])
    if (selectedShot.value?.id === shot.id) {
      selectedShot.value = latestShot
    }
    startShotGenerationPolling(shot.id)
  } catch (error) {
    message.error('生成失败')
  } finally {
    shotActionBusyIds.value.delete(shot.id)
    shotActionBusyIds.value = new Set(shotActionBusyIds.value)
  }
}

async function generateAllShots() {
  if (hasOpenShotInlineForm.value) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  if (shotMutationBusy.value) return
  const validShots = getGeneratableShots()
  if (validShots.length === 0) {
    if (shots.value.some((shot) => hasShotVideoGenerationParams(shot) && isShotGenerationLocked(shot))) {
      message.warning('符合条件的分镜正在生成视频，请稍候')
    } else {
      message.warning('没有符合条件的分镜（需要有视频描述、模型、分辨率和时长）')
    }
    return
  }
  generating.value = true
  batchProgress.value = { completed: 0, total: validShots.length, failed: 0 }
  try {
    const result = await batchGenerateShotsApi(projectId.value, {
      items: validShots.map((shot) => ({
        shotId: shot.id,
        requestKey: getGenerationRequestKey(shot.id),
      })),
    })
    const skipped = result.shotResults.filter((item) => item.status === 'skipped').length
    batchProgress.value = {
      completed: result.successCount + skipped,
      total: result.totalShots,
      failed: result.failedCount,
    }

    if (result.failedCount > 0) {
      message.warning(
        `批量生成完成：成功 ${result.successCount} 个，跳过 ${skipped} 个，失败 ${result.failedCount} 个`,
      )
    } else {
      message.success(`批量生成完成：成功 ${result.successCount} 个，跳过 ${skipped} 个`)
    }

    await loadShots()
    const submittedShotIds = result.shotResults
      .filter((item) => item.status === 'success')
      .map((item) => item.shotId)
    const submittedShots = shots.value.filter((shot) => submittedShotIds.includes(shot.id))
    if (submittedShots.length > 0) {
      await loadShotVideoVersions(submittedShots)
      const latestSelectedShot = submittedShots.find((shot) => shot.id === selectedShot.value?.id)
      if (latestSelectedShot) {
        selectedShot.value = latestSelectedShot
      }
    }
    const pollingShotIds = new Set([
      ...submittedShotIds,
      ...shots.value.filter((item) => item.status === 'generating').map((item) => item.id),
    ])
    pollingShotIds.forEach((shotId) => startShotGenerationPolling(shotId))
  } catch (error) {
    message.error('批量生成失败')
  } finally {
    generating.value = false
    batchProgress.value = { completed: 0, total: 0, failed: 0 }
  }
}

// 辅助函数
function getStatusText(status: string): string {
  const map: Record<string, string> = {
    draft: '草稿',
    generating: '生成中',
    completed: '已完成',
    failed: '失败',
  }
  return map[status] || status
}

function getStatusColor(status: string): string {
  const map: Record<string, string> = {
    draft: 'default',
    generating: 'processing',
    completed: 'success',
    failed: 'error',
  }
  return map[status] || 'default'
}

function getCharacterImage(charId: string): string {
  const char = characters.value.find((c) => c.id === charId)
  return previewReferenceAsset(char?.referenceImageUrl)
}

function getCharacterName(charId: string): string {
  const char = characters.value.find((c) => c.id === charId)
  return char?.name || ''
}

function copyPrompt() {
  if (selectedShot.value?.generatedPrompt) {
    navigator.clipboard.writeText(selectedShot.value.generatedPrompt)
    message.success('已复制到剪贴板')
  }
}

function showProjectSettings() {
  if (!project.value) return
  projectForm.value = {
    name: project.value.name,
    theme: project.value.theme || '',
    description: project.value.description || '',
    styleGuide: project.value.styleGuide || '',
  }
  projectModalVisible.value = true
}

async function handleUpdateProject() {
  if (!projectForm.value.name.trim()) {
    message.warning('请填写项目名称')
    return
  }

  projectLoading.value = true
  try {
    project.value = await updateProjectApi(projectId.value, projectForm.value)
    message.success('项目设置已更新')
    projectModalVisible.value = false
  } catch (error) {
    message.error('项目设置更新失败')
  } finally {
    projectLoading.value = false
  }
}

const composeOptions = ref({
  transition: 'none',
  musicUrl: '',
  enableSubtitles: false,
})

async function waitForComposeJob() {
  for (let attempt = 0; attempt < 120; attempt += 1) {
    const job = await getComposeJobApi(projectId.value, composeJobId.value)
    composeProgress.value = job.progress
    if (job.status === 'completed') return job
    if (job.status === 'failed') throw new Error(job.error || '视频合成失败')
    await new Promise((resolve) => window.setTimeout(resolve, 2000))
  }
  throw new Error('合成仍在处理中，请稍后刷新')
}

async function composeVideo() {
  if (hasOpenShotInlineForm.value) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  if (shotMutationBusy.value) return
  // 检查是否有已完成的分镜
  const completedShots = shots.value.filter((s) => s.status === 'completed' && s.videoUrl)
  if (completedShots.length === 0) {
    message.warning('没有已完成的分镜，无法合成视频')
    return
  }

  composing.value = true
  composeProgress.value = 50
  try {
	const started = await composeProjectVideoApi(projectId.value, composeOptions.value)
	composeJobId.value = started.jobId
	composeProgress.value = started.progress
	const result = await waitForComposeJob()
    message.success('视频合成完成')

    Modal.success({
      title: '视频合成成功',
      content: `视频已生成：${result.videoUrl}`,
      onOk: () => {
        if (result.videoUrl) {
          window.open(result.videoUrl, '_blank')
        }
      },
    })

    await loadProject()
  } catch (error) {
    message.error('视频合成失败')
  } finally {
    composing.value = false
    composeProgress.value = 0
  }
}

function onDragStart(event: DragEvent, type: string, data: any) {
  if (shotMutationBusy.value || hasOpenShotInlineForm.value) {
    event.preventDefault()
    return
  }
  event.dataTransfer?.setData('type', type)
  event.dataTransfer?.setData('data', JSON.stringify(data))
}

function shotToPayload(shot: Shot) {
  return {
    name: shot.name,
    scriptOriginalContent: shot.scriptOriginalContent || '',
    actionDescription: shot.actionDescription,
    dynamicDescription: shot.dynamicDescription || '',
    gridStoryboardPrompt: shot.gridStoryboardPrompt || '',
    storyboardUrl: shot.storyboardUrl || '',
    videoModel: shot.videoModel || selectedVideoModel.value,
    videoResolution: shot.videoResolution || '720p',
    soundAndPictureTogether: shot.soundAndPictureTogether || soundAndPictureTogether.value,
    duration: shot.duration,
    aspectRatio: shot.aspectRatio,
    characterIds: [...(shot.characterIds || [])],
    sceneId: shot.sceneId || '',
    cameraMovement: shot.cameraMovement || '',
    imageReferenceModes: [...(shot.imageReferenceModes || [])],
    videoReferenceMode: shot.videoReferenceMode || 'none',
    orderNum: shot.orderNum,
  }
}

async function onDropToShot(event: DragEvent, shot: Shot) {
  if (shotMutationBusy.value || hasOpenShotInlineForm.value) {
    message.warning('请先保存或取消当前分镜编辑')
    return
  }
  if (bindingShotIds.value.has(shot.id)) return
  const type = event.dataTransfer?.getData('type')
  const raw = event.dataTransfer?.getData('data')
  if (!type || !raw) return

  bindingShotIds.value.add(shot.id)
  bindingShotIds.value = new Set(bindingShotIds.value)
  try {
    const resource = JSON.parse(raw) as Character | Scene
    const latestShot = shots.value.find((item) => item.id === shot.id) || shot
    const payload = shotToPayload(latestShot)

    if (type === 'character') {
      const characterId = resource.id
      if (!payload.characterIds.includes(characterId)) {
        payload.characterIds.push(characterId)
      }
    } else if (type === 'scene') {
      payload.sceneId = resource.id
    } else {
      return
    }

    const updated = await updateShotApi(shot.id, payload)
    message.success(type === 'character' ? '角色已绑定到分镜' : '场景已绑定到分镜')
    await loadShots()
    selectedShot.value = shots.value.find((item) => item.id === updated.id) || updated
  } catch (error) {
    message.error('绑定素材失败')
  } finally {
    bindingShotIds.value.delete(shot.id)
    bindingShotIds.value = new Set(bindingShotIds.value)
  }
}

// 初始化
onMounted(() => {
  loadProject()
  loadCharacters()
  loadScenes()
  loadShots()
})

onBeforeUnmount(() => {
  stopAllShotGenerationPolling()
})
</script>

<style scoped lang="less">
.workbench-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #f0f2f5;
}

.workbench-header {
  padding: 16px 24px;
  background: #fff;
  color: #0f172a;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid #f0f0f0;

  :deep(.ant-breadcrumb),
  :deep(.ant-breadcrumb-link),
  :deep(.ant-breadcrumb-separator) {
    color: #475569 !important;
  }
}


.header-actions {
  display: flex;
  gap: 8px;
}


.production-flow-panel {
  flex: 0 0 auto;
  margin: 12px 24px 10px;
  padding: 10px 14px;
  color: #e9f2ff;
  display: grid;
  grid-template-columns: minmax(360px, 420px) minmax(480px, 1fr) minmax(280px, auto);
  align-items: center;
  gap: 14px;
  border: 1px solid rgba(77, 227, 255, 0.18);
  border-radius: 14px;
  background:
    radial-gradient(360px circle at 8% 0%, rgba(77, 227, 255, 0.18), transparent 62%),
    linear-gradient(135deg, rgba(10, 16, 26, 0.98), rgba(12, 18, 28, 0.94));
  box-shadow: 0 14px 32px rgba(6, 10, 16, 0.22);

  :deep(.ant-tag) {
    margin-inline-end: 0;
    color: #7cffc4;
    background: rgba(77, 227, 255, 0.08);
    border-color: rgba(124, 255, 196, 0.36);
  }

  :deep(.ant-btn-default) {
    color: rgba(233, 242, 255, 0.88);
    background: rgba(255, 255, 255, 0.06);
    border-color: rgba(233, 242, 255, 0.18);
  }

  :deep(.ant-btn-default:hover) {
    color: #7cffc4;
    border-color: rgba(124, 255, 196, 0.58);
  }
}

.flow-title {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 10px;
}

.workbench-mode-switch {
  padding: 3px;
  display: inline-flex;
  flex: 0 0 auto;
  gap: 3px;
  border: 1px solid rgba(233, 242, 255, 0.14);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.mode-switch-item {
  min-height: 26px;
  padding: 3px 10px;
  color: rgba(233, 242, 255, 0.72);
  cursor: pointer;
  border: 0;
  border-radius: 999px;
  background: transparent;
  transition: color 0.2s ease, background 0.2s ease;
}

.mode-switch-item.is-active {
  color: #071018;
  font-weight: 700;
  background: linear-gradient(135deg, rgba(77, 227, 255, 0.95), rgba(124, 255, 196, 0.95));
}

.flow-title-copy {
  min-width: 0;

  h2 {
    margin: 0 0 2px;
    color: #f8fbff;
    font-size: 15px;
    font-weight: 700;
    letter-spacing: 0.2px;
    line-height: 1.25;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  p {
    margin: 0;
    color: rgba(233, 242, 255, 0.62);
    font-size: 12px;
    line-height: 1.35;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.step-bar {
  display: grid;
  grid-template-columns: repeat(5, minmax(92px, 1fr));
  align-items: center;
  min-width: 0;
  gap: 8px;
}

.short-film-mode-placeholder {
  min-width: 0;
  padding: 8px 12px;
  display: flex;
  align-items: center;
  gap: 10px;
  color: rgba(233, 242, 255, 0.76);
  border: 1px dashed rgba(124, 255, 196, 0.34);
  border-radius: 12px;
  background: rgba(124, 255, 196, 0.06);

  strong {
    color: #7cffc4;
    white-space: nowrap;
  }

  span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.step-item {
  min-width: 0;
  min-height: 36px;
  padding: 6px 8px;
  color: rgba(233, 242, 255, 0.7);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  border: 1px solid rgba(77, 227, 255, 0.2);
  border-radius: 999px;
  background: rgba(12, 18, 28, 0.72);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.02);
  transition: color 0.2s ease, border-color 0.2s ease, background 0.2s ease, box-shadow 0.2s ease;
}

.step-item:hover {
  color: #ffffff;
  border-color: rgba(77, 227, 255, 0.5);
}

.step-item.is-done {
  color: rgba(124, 255, 196, 0.95);
  border-color: rgba(124, 255, 196, 0.55);
  background: rgba(12, 20, 28, 0.9);
}

.step-item.is-active {
  color: #071018;
  font-weight: 700;
  border-color: transparent;
  background: linear-gradient(135deg, rgba(77, 227, 255, 0.95), rgba(124, 255, 196, 0.95));
  box-shadow: 0 10px 18px rgba(77, 227, 255, 0.24);
}

.step-index {
  width: 18px;
  height: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 18px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  background: rgba(233, 242, 255, 0.1);
}

.step-item.is-active .step-index {
  color: #071018;
  background: rgba(255, 255, 255, 0.44);
}

.step-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
}

.flow-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}

.flow-stat,
.bucket-pill {
  min-height: 24px;
  padding: 3px 9px;
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  color: rgba(233, 242, 255, 0.74);
  font-size: 12px;
  line-height: 1;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(233, 242, 255, 0.1);
}

.bucket-pill {
  color: rgba(124, 255, 196, 0.95);
  border-color: rgba(124, 255, 196, 0.24);
  background: rgba(124, 255, 196, 0.08);
}

.workbench-layout {
  margin: 0 24px 24px;
  flex: 1 1 0;
  min-height: 0;
  overflow: hidden;
  border-radius: 12px;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.08);
}

.short-film-placeholder-card {
  margin: 0 24px 24px;
  flex: 1 1 0;
  min-height: 320px;
  padding: 32px;
  color: #0f172a;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  text-align: center;
  border: 1px dashed #bae6fd;
  border-radius: 16px;
  background:
    radial-gradient(360px circle at 50% 0%, rgba(14, 165, 233, 0.13), transparent 62%),
    #ffffff;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.08);

  h3 {
    margin: 0;
    font-size: 20px;
  }

  p {
    max-width: 720px;
    margin: 0;
    color: #64748b;
    line-height: 1.7;
  }
}

.short-film-placeholder-icon {
  width: 72px;
  height: 72px;
  color: #0369a1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 22px;
  background: #e0f2fe;
  font-weight: 700;
}

.short-film-placeholder-grid {
  margin-top: 8px;
  display: grid;
  grid-template-columns: repeat(4, minmax(88px, 1fr));
  gap: 10px;

  span {
    padding: 8px 10px;
    color: #0f766e;
    border: 1px solid #99f6e4;
    border-radius: 999px;
    background: #f0fdfa;
  }
}

.workbench-sider,
.workbench-sider-right {
  min-height: 0;
  background: #fff;
  color: #0f172a;
  border-right: 1px solid #f0f0f0;
  overflow-y: auto;

  :deep(.ant-tabs-tab-btn),
  :deep(.ant-empty-description),
  :deep(.ant-form-item-label > label) {
    color: #334155 !important;
  }

  :deep(.ant-tabs-tab-active .ant-tabs-tab-btn) {
    color: #1677ff !important;
  }

  :deep(.ant-btn-default) {
    background: #fff;
    color: #0f172a;
    border-color: #cbd5e1;
  }
}


.workbench-sider-right {
  border-right: none;
  border-left: 1px solid #f0f0f0;
}

.resource-panel {
  padding: 16px;
  color: #0f172a;
}

.resource-list {
  margin-top: 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.resource-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: #fafafa;
  color: #0f172a;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  cursor: move;
  transition: all 0.2s;

  &:hover {
    background: #f0f0f0;

    .resource-actions {
      opacity: 1;
    }
  }
}

.resource-preview {
  width: 48px;
  height: 48px;
  border-radius: 4px;
  background: #e6e6e6;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  color: #999;
  overflow: hidden;

  img,
  video {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.resource-info {
  flex: 1;
  min-width: 0;
}

.resource-name {
  font-weight: 500;
  margin-bottom: 4px;
  display: flex;
  align-items: center;
  gap: 4px;
}

.resource-desc {
  font-size: 12px;
  color: #999;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-actions {
  display: flex;
  gap: 8px;
  opacity: 0;
  transition: opacity 0.2s;

  svg {
    cursor: pointer;
    color: #999;

    &:hover {
      color: #1890ff;
    }
  }
}

.workbench-content {
  min-height: 0;
  padding: 16px;
  overflow-x: hidden;
  overflow-y: auto;
  background: #f8fafc;
  color: #0f172a;

  :deep(.ant-empty-description) {
    color: #64748b !important;
  }

  :deep(.ant-btn-default) {
    background: #fff;
    color: #0f172a;
    border-color: #cbd5e1;
  }
}


.shots-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;

  h3 {
    margin: 0;
  }
}

.shots-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.board-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.shot-card {
  display: flex;
  gap: 16px;
  padding: 16px;
  background: #fff;
  color: #0f172a;
  border-radius: 8px;
  border: 2px solid transparent;
  cursor: pointer;
  transition: all 0.2s;

  &:hover {
    border-color: #d9d9d9;
  }

  &.active {
    border-color: #1890ff;
    box-shadow: 0 2px 8px rgba(24, 144, 255, 0.2);
  }

  &.editing,
  &.shot-edit-card {
    align-items: flex-start;
    border-color: rgba(24, 144, 255, 0.35);
    background: linear-gradient(180deg, #ffffff 0%, #f8fbff 100%);
    cursor: default;
  }
}

.board-card {
  width: 100%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0;
  padding: 0;
  overflow: hidden;
  cursor: default;
}

.inline-board-card {
  width: 100%;
  border: 0;
  background: transparent;
}

.board-head {
  min-height: 58px;
  padding: 12px 14px;
  display: flex;
  align-items: center;
  gap: 12px;
  border-bottom: 1px solid #e2e8f0;
  background: linear-gradient(180deg, #ffffff 0%, #f8fbff 100%);
}

.board-title {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 10px;
}

.board-title-copy {
  min-width: 0;
}

.board-subtitle {
  max-width: 720px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.board-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #64748b;
  font-size: 12px;
  white-space: nowrap;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #cbd5e1;
}

.status-green {
  background: #22c55e;
}

.board-body {
  display: grid;
  grid-template-columns: minmax(220px, 1.05fr) minmax(220px, 0.9fr) minmax(180px, 0.72fr);
  gap: 14px;
  padding: 14px;
  background: #fff;
}

.board-body-edit {
  grid-template-columns: 1fr;
}

.board-col {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow-wrap: anywhere;
}

.col-left-wide {
  max-width: none;
}

.section-block,
.video-generate-panel,
.version-panel {
  min-width: 0;
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: #f8fafc;
}

.col-title {
  margin-bottom: 8px;
  color: #0f172a;
  font-size: 13px;
  font-weight: 700;
}

.version-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;

  .col-title {
    margin-bottom: 0;
  }
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 10px;
}

.section-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.shot-reference-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.shot-reference-item {
  position: relative;
  width: 108px;
  min-height: 108px;
  overflow: hidden;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: #fff;

  img,
  video {
    width: 100%;
    height: 82px;
    display: block;
    object-fit: cover;
    background: #0f172a;
  }

  audio {
    width: 100%;
    height: 42px;
    margin-top: 28px;
  }
}

.shot-reference-video {
  width: 132px;
}

.shot-reference-audio {
  width: 180px;
}

.reference-delete {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 24px;
  height: 24px;
  color: #fff;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.62);
}

.reference-label {
  padding: 4px 6px;
  color: #475569;
  font-size: 12px;
  line-height: 1.35;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.video-generate-panel,
.version-panel,
.version-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.generation-param-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(132px, 1fr));
  gap: 8px;
}

.estimate-pill {
  width: fit-content;
  padding: 4px 10px;
  color: #0f766e;
  font-size: 12px;
  border: 1px solid #99f6e4;
  border-radius: 999px;
  background: #f0fdfa;
}

.video-preview-large {
  width: 100%;
  height: 180px;
  border-radius: 10px;
}

.version-item {
  overflow: hidden;
  border: 1px solid #dbeafe;
  border-radius: 10px;
  background: #fff;
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.04);

  &.is-active {
    border-color: #1677ff;
    box-shadow: 0 0 0 2px rgba(22, 119, 255, 0.12);
  }

  video {
    width: 100%;
    height: 142px;
    display: block;
    object-fit: cover;
    background: #0f172a;
  }
}

.version-preview-wrap {
  position: relative;
  min-height: 116px;
  overflow: hidden;
  background: #0f172a;
}

.version-resolution {
  position: absolute;
  top: 8px;
  left: 8px;
  z-index: 6;
  max-width: calc(100% - 110px);
  padding: 3px 7px;
  color: rgba(255, 255, 255, 0.94);
  font-size: 11px;
  line-height: 1.2;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  border: 1px solid rgba(255, 255, 255, 0.14);
  border-radius: 6px;
  background: rgba(2, 6, 23, 0.72);
  box-shadow: 0 4px 12px rgba(2, 6, 23, 0.28);
  backdrop-filter: blur(6px);
  pointer-events: none;
}

.version-status-row {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 8;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.status-tag {
  min-width: 24px;
  height: 24px;
  padding: 0 7px;
  color: rgba(255, 255, 255, 0.92);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  line-height: 1;
  border: 1px solid rgba(255, 255, 255, 0.16);
  border-radius: 6px;
  background: rgba(2, 6, 23, 0.72);
  box-shadow: 0 4px 12px rgba(2, 6, 23, 0.22), inset 0 1px 0 rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(6px);
}

.status-tag:hover:not(:disabled) {
  border-color: rgba(96, 165, 250, 0.58);
  background: rgba(30, 64, 175, 0.82);
}

.status-tag:disabled {
  cursor: not-allowed;
  opacity: 0.72;
}

.status-tag--current {
  color: #dbeafe;
  border-color: rgba(96, 165, 250, 0.52);
  background: rgba(29, 78, 216, 0.84);
}

.status-tag--backup.is-active {
  color: #fff7ed;
  border-color: rgba(251, 191, 36, 0.64);
  background: linear-gradient(135deg, rgba(180, 83, 9, 0.92), rgba(217, 119, 6, 0.86));
}

.status-tag--upscale {
  color: #ecfdf5;
  background: rgba(6, 95, 70, 0.78);
}

.status-tag--upscale.is-active {
  color: #fff;
  border-color: rgba(52, 211, 153, 0.62);
  background: linear-gradient(135deg, rgba(5, 150, 105, 0.94), rgba(20, 184, 166, 0.86));
}

.status-tag--upscale:hover:not(:disabled) {
  border-color: rgba(52, 211, 153, 0.62);
  background: rgba(13, 148, 136, 0.88);
}

.status-tag--extract {
  color: #fff;
  background: rgba(76, 29, 149, 0.78);
}

.status-tag--extract:hover:not(:disabled) {
  border-color: rgba(196, 181, 253, 0.6);
  background: rgba(109, 40, 217, 0.86);
}

.status-tag--subtitle {
  color: #f5f3ff;
  background: rgba(88, 28, 135, 0.76);
}

.status-tag--subtitle.is-active {
  color: #fff;
  border-color: rgba(216, 180, 254, 0.66);
  background: linear-gradient(135deg, rgba(107, 33, 168, 0.94), rgba(147, 51, 234, 0.86));
}

.status-tag--subtitle:hover:not(:disabled) {
  border-color: rgba(216, 180, 254, 0.66);
  background: rgba(126, 34, 206, 0.88);
}

.detail-view-btn {
  position: absolute;
  left: 10px;
  bottom: 10px;
  z-index: 8;
  width: 28px;
  height: 24px;
  padding: 0;
  color: rgba(255, 255, 255, 0.92);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(255, 255, 255, 0.16);
  border-radius: 6px;
  background: rgba(2, 6, 23, 0.72);
  box-shadow: 0 4px 12px rgba(2, 6, 23, 0.22), inset 0 1px 0 rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(6px);
}

.detail-view-btn:hover {
  color: #fff;
  border-color: rgba(96, 165, 250, 0.58);
  background: rgba(30, 64, 175, 0.82);
}

.version-video-thumbnail {
  position: relative;
  min-height: 142px;
  cursor: pointer;
  overflow: hidden;
  background: #0f172a;
}

.thumbnail-image {
  width: 100%;
  height: 142px;
  display: block;
  object-fit: cover;
  background: #0f172a;
}

.thumbnail-fallback {
  height: 142px;
  color: rgba(226, 232, 240, 0.82);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;

  svg {
    font-size: 28px;
  }
}

.play-overlay {
  position: absolute;
  inset: 0;
  color: rgba(255, 255, 255, 0.92);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 42px;
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.08), rgba(15, 23, 42, 0.36));
  pointer-events: none;
  transition: transform 0.18s ease, background 0.18s ease;
}

.version-video-thumbnail:hover .play-overlay {
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.02), rgba(15, 23, 42, 0.5));
  transform: scale(1.06);
}

.unviewed-badge {
  position: absolute;
  left: 50%;
  top: 50%;
  z-index: 7;
  transform: translate(-50%, -50%);
  min-height: 28px;
  padding: 0 8px;
  color: #fff;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.78);
  box-shadow: 0 8px 18px rgba(15, 23, 42, 0.28);
  backdrop-filter: blur(8px);
}

.unviewed-dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: #ff4d6d;
  box-shadow: 0 0 0 4px rgba(255, 77, 109, 0.18);
}

.unviewed-close {
  width: 16px;
  height: 16px;
  color: rgba(255, 255, 255, 0.82);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.12);
  line-height: 1;
}

.version-empty-preview {
  height: 116px;
  color: #64748b;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #eff6ff 0%, #f8fafc 100%);
}

.version-meta {
  padding: 8px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.version-tags {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 6px;
}

.version-model,
.version-time {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.version-actions {
  padding: 0 8px 8px;
  display: flex;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
}

.version-actions.is-secondary {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
}

.version-actions.is-secondary :deep(.ant-btn.action-tag) {
  width: 100%;
  min-width: 0;
  padding: 0 6px;
  overflow: hidden;
}

.version-actions.is-secondary :deep(.ant-btn.action-tag > span) {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.video-generation-detail-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
  color: #0f172a;
}

:global(.video-generation-detail-modal .ant-modal) {
  top: 24px;
  max-width: calc(100vw - 24px);
  padding-bottom: 24px;
}

:global(.video-generation-detail-modal .ant-modal-content) {
  max-height: calc(100vh - 48px);
  display: flex;
  flex-direction: column;
}

:global(.video-generation-detail-modal .ant-modal-body) {
  max-height: calc(100vh - 112px);
  overflow-x: hidden;
  overflow-y: auto;
}

.detail-meta {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.detail-meta-item {
  min-width: 0;
  padding: 10px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: #f8fafc;

  span {
    display: block;
    color: #64748b;
    font-size: 12px;
    margin-bottom: 4px;
  }

  strong {
    display: block;
    overflow: hidden;
    color: #0f172a;
    font-size: 13px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.video-generation-detail-grid {
  display: grid;
  grid-template-columns: minmax(260px, 0.95fr) minmax(280px, 1fr) minmax(260px, 0.95fr);
  gap: 12px;
}

.detail-panel {
  min-width: 0;
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  background: #ffffff;

  h4 {
    margin: 0 0 8px;
    color: #0f172a;
    font-size: 14px;
  }

  h4:not(:first-child) {
    margin-top: 14px;
  }
}

.detail-dark {
  min-height: 96px;
  padding: 10px;
  color: #dbeafe;
  line-height: 1.7;
  white-space: pre-wrap;
  border-radius: 10px;
  background: #0f172a;
}

.detail-prompt {
  min-height: 320px;
  max-height: 420px;
  overflow-y: auto;
}

.detail-reference-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.detail-reference-card {
  width: 128px;
  min-height: 108px;
  overflow: hidden;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: #f8fafc;

  img,
  video {
    width: 100%;
    height: 82px;
    display: block;
    object-fit: cover;
    background: #0f172a;
  }

  audio {
    width: 100%;
    height: 42px;
    margin-top: 22px;
  }

  span {
    display: block;
    padding: 4px 6px;
    color: #475569;
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.detail-video-result {
  width: 100%;
  min-height: 260px;
  max-height: 420px;
  object-fit: contain;
  border-radius: 10px;
  background: #0f172a;
}

.detail-empty {
  min-height: 96px;
  padding: 16px;
  color: #64748b;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px dashed #cbd5e1;
  border-radius: 10px;
  background: #f8fafc;
}

.detail-footer {
  display: flex;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
}

.detail-action-close {
  margin-left: auto;
}

.shot-video-preview-dialog {
  min-height: 360px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #020617;
  border-radius: 12px;
  overflow: hidden;
}

.preview-dialog-video {
  width: 100%;
  max-height: 70vh;
  display: block;
  object-fit: contain;
  background: #020617;
}

.shot-order {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: #1890ff;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 500;
  flex-shrink: 0;
}

.shot-preview {
  width: 120px;
  height: 80px;
  border-radius: 4px;
  background: #000;
  overflow: hidden;
  flex-shrink: 0;

  img,
  video {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.shot-preview-edit {
  border: 1px dashed #cbd5e1;
  background: #f8fafc;
  color: #64748b;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;

  svg {
    font-size: 28px;
  }
}

.shot-inline-form {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;

  :deep(.ant-input),
  :deep(.ant-select-selector) {
    background: #fff;
    border-color: #dbe3ef !important;
  }
}

.inline-form-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  color: #0f172a;

  strong {
    flex-shrink: 0;
  }

  span {
    color: #64748b;
    font-size: 12px;
    line-height: 1.5;
  }
}

.shot-inline-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(132px, 1fr));
  gap: 8px;
}

.inline-form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.shot-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #666;
  font-size: 32px;
}

.shot-status {
  font-size: 12px;
  margin-top: 4px;
}

.shot-info {
  flex: 1;
  min-width: 0;
}

.shot-name {
  font-weight: 500;
  margin-bottom: 4px;
}

.shot-action {
  color: #666;
  margin-bottom: 8px;
  font-size: 14px;
}

.shot-meta {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}

.shot-actions {
  flex-shrink: 0;
}

.shot-detail-panel {
  padding: 16px;
  color: #0f172a;

  :deep(.ant-empty-description),
  :deep(.ant-descriptions-item-label),
  :deep(.ant-descriptions-item-content) {
    color: #334155 !important;
  }
}


.detail-section {
  h4 {
    margin: 16px 0 8px;
    font-weight: 500;
  }

  p {
    color: #666;
    margin-bottom: 16px;
  }
}

.reference-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.reference-item {
  width: 80px;
  height: 80px;
  border-radius: 4px;
  overflow: hidden;
  position: relative;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  span {
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    background: rgba(0, 0, 0, 0.6);
    color: #fff;
    font-size: 12px;
    padding: 2px 4px;
    text-align: center;
  }
}

.reference-video-item {
  width: 128px;

  video {
    width: 100%;
    height: 100%;
    object-fit: cover;
    background: #000;
  }
}

.reference-audio-item {
  width: 160px;
  display: flex;
  align-items: center;
  background: #f8fafc;
  border: 1px solid #e2e8f0;

  audio {
    width: 100%;
    padding: 0 6px;
  }
}

.reference-upload-field {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}

.reference-upload-preview {
  display: grid;
  grid-template-columns: 144px minmax(0, 1fr);
  gap: 12px;
  align-items: center;
  padding: 10px;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: #f8fafc;

  img,
  video {
    width: 144px;
    height: 96px;
    border: 1px solid #e2e8f0;
    border-radius: 8px;
    background: #fff;
    object-fit: cover;
  }

  video {
    background: #000;
  }
}

.reference-upload-video {
  grid-template-columns: minmax(180px, 220px) minmax(0, 1fr);

  video {
    width: 100%;
    height: 124px;
  }
}

.reference-upload-actions {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}

.reference-upload-url {
  flex: 1;
  min-width: 0;
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.reference-upload-empty {
  padding: 12px;
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
  border: 1px dashed #cbd5e1;
  border-radius: 10px;
  background: #f8fafc;
}

.prompt-section {
  padding: 16px 0;
}

.preview-images {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.progress-text {
  font-size: 14px;
  color: #1890ff;
  font-weight: 500;
  white-space: nowrap;
}

@media (max-width: 1200px) and (min-width: 901px) {
  .production-flow-panel {
    grid-template-columns: 1fr;
    align-items: stretch;
  }

  .flow-title-copy p {
    white-space: normal;
  }

  .step-bar {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }

  .flow-actions {
    justify-content: flex-start;
  }

  .workbench-layout {
    display: grid;
    grid-template-columns: minmax(220px, 280px) minmax(0, 1fr);
    grid-template-rows: minmax(0, 1fr) auto;
    overflow: hidden;
  }

  .workbench-sider {
    width: auto !important;
    min-width: 0 !important;
    max-width: none !important;
    grid-column: 1;
    grid-row: 1;
  }

  .workbench-content {
    width: 100% !important;
    max-width: 100%;
    min-width: 0;
    grid-column: 2;
    grid-row: 1;
  }

  .workbench-sider-right {
    width: auto !important;
    min-width: 0 !important;
    max-width: none !important;
    max-height: 220px;
    grid-column: 1 / -1;
    grid-row: 2;
    border-top: 1px solid #f0f0f0;
    border-left: none;
    overflow-y: auto;
  }

  .shot-card {
    gap: 10px;
    padding: 12px;
    min-width: 0;
  }

  .board-card {
    padding: 0;
  }

  .board-body {
    grid-template-columns: 1fr;
  }

  .shots-list,
  .shot-inline-form {
    min-width: 0;
  }

  .shot-preview {
    width: 96px;
    height: 64px;
  }

  .shot-inline-grid {
    grid-template-columns: repeat(auto-fit, minmax(min(120px, 100%), 1fr));
  }

  .shot-inline-grid :deep(.ant-select),
  .shot-inline-grid :deep(.ant-input),
  .shot-inline-grid :deep(.ant-select-selector) {
    min-width: 0;
    max-width: 100%;
  }

  .shot-name,
  .shot-action,
  .board-subtitle,
  .detail-section p {
    overflow-wrap: anywhere;
    word-break: break-word;
    white-space: normal;
  }
}

@media (max-width: 900px) {
  .workbench-container {
    height: auto;
    min-height: 100%;
  }

  .workbench-header,
  .shots-header {
    flex-direction: column;
    align-items: stretch;
  }

  .header-actions {
    flex-wrap: wrap;
  }

  .production-flow-panel {
    margin: 12px;
    grid-template-columns: 1fr;
    align-items: stretch;
  }

  .flow-title {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .flow-title-copy p {
    white-space: normal;
  }

  .step-bar {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .flow-actions {
    justify-content: flex-start;
  }

  .workbench-layout {
    margin: 0 12px 16px;
    flex-direction: column;
    overflow-x: hidden;
    overflow-y: auto;
  }

  .short-film-placeholder-card {
    margin: 0 12px 16px;
    padding: 24px 16px;
  }

  .short-film-placeholder-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .short-film-mode-placeholder {
    align-items: flex-start;
    flex-direction: column;

    span {
      white-space: normal;
    }
  }

  .workbench-sider,
  .workbench-sider-right {
    width: 100% !important;
    min-width: 0 !important;
    max-width: 100% !important;
    flex: none !important;
    border-right: none;
    border-left: none;
  }

  .workbench-content {
    width: 100% !important;
    max-width: 100% !important;
    min-width: 0;
    flex: none !important;
    overflow: visible;
  }

  .board-head,
  .section-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .board-body {
    grid-template-columns: 1fr;
  }

  .detail-meta {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .video-generation-detail-grid {
    grid-template-columns: 1fr;
  }

  .detail-prompt {
    min-height: 180px;
    max-height: 260px;
  }

  .detail-video-result {
    min-height: 180px;
    max-height: 260px;
  }
}

</style>
