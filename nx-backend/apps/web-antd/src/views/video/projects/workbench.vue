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
        <a-tag color="blue">项目制生产流</a-tag>
        <div class="flow-title-copy">
          <h2>制片工作台流程</h2>
          <p>{{ activeProductionStepMeta?.description }}</p>
        </div>
      </div>

      <div class="step-bar" aria-label="项目制生产流程">
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

      <div class="flow-actions">
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
    <a-layout class="workbench-layout">
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
                    <img v-if="char.referenceImageUrl" :src="char.referenceImageUrl" />
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
                    <img v-if="scene.referenceImageUrl" :src="scene.referenceImageUrl" />
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
      <a-layout-content class="workbench-content">
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
                :disabled="shotMutationBusy || hasOpenShotInlineForm"
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
          <div
            v-for="(shot, index) in shots"
            :key="shot.id"
            class="shot-card"
            :class="{ active: selectedShot?.id === shot.id, editing: editingShotId === shot.id }"
            @click="editingShotId === shot.id ? undefined : selectShot(shot)"
            @dragover.prevent
            @drop.stop="onDropToShot($event, shot)"
          >
            <template v-if="editingShotId === shot.id">
              <div class="shot-order">{{ index + 1 }}</div>

              <div class="shot-preview shot-preview-edit">
                <video v-if="shot.videoUrl" :src="shot.videoUrl" muted playsinline />
                <template v-else>
                  <VideoCameraOutlined />
                  <div class="shot-status">编辑中</div>
                </template>
              </div>

              <div class="shot-inline-form">
                <div class="inline-form-head">
                  <strong>编辑分镜 {{ index + 1 }}</strong>
                  <span>参考旧项目方式，直接在当前分镜框内维护标题、动作、角色和场景。</span>
                </div>
                <a-input v-model:value="shotForm.name" placeholder="分镜名称（可选）" />
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
            </template>

            <template v-else>
              <div class="shot-order">{{ index + 1 }}</div>

              <div class="shot-preview">
                <video v-if="shot.videoUrl" :src="shot.videoUrl" muted playsinline />
                <div v-else class="shot-placeholder">
                  <VideoCameraOutlined />
                  <div class="shot-status">{{ getStatusText(shot.status) }}</div>
                </div>
              </div>

              <div class="shot-info">
                <div class="shot-name">{{ shot.name || `分镜 ${index + 1}` }}</div>
                <div class="shot-action">{{ shot.actionDescription }}</div>

                <div class="shot-meta">
                  <a-tag v-if="shot.duration" size="small">{{ shot.duration }}s</a-tag>
                  <a-tag v-if="shot.aspectRatio" size="small">{{ shot.aspectRatio }}</a-tag>
                </div>

                <div class="shot-progress" v-if="shot.status === 'generating'">
                  <a-progress :percent="50" size="small" status="active" />
                </div>
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
            </template>
          </div>

          <div
            v-if="inlineShotFormVisible && !editingShotId"
            ref="shotEditCardRef"
            class="shot-card shot-edit-card"
            @click.stop
          >
            <div class="shot-order">{{ getNextShotOrderNum() }}</div>

            <div class="shot-preview shot-preview-edit">
              <VideoCameraOutlined />
              <div class="shot-status">待创建</div>
            </div>

            <div class="shot-inline-form">
              <div class="inline-form-head">
                <strong>新增分镜</strong>
                <span>直接在分镜框里填写信息，保存后进入列表继续绑定资产和生成视频。</span>
              </div>
              <a-input v-model:value="shotForm.name" placeholder="分镜名称（可选）" />
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

          <a-empty
            v-if="shots.length === 0 && !inlineShotFormVisible"
            description="暂无分镜，点击添加分镜开始创作"
          />
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
                    <img :src="getCharacterImage(charId)" />
                    <span>{{ getCharacterName(charId) }}</span>
                  </div>
                  <a-empty v-if="!selectedShot.characterIds?.length" :image="simpleImage" />
                </div>

                <h4>参考素材</h4>
                <div class="reference-list">
                  <div
                    v-for="img in selectedShot.usedImages"
                    :key="img"
                    class="reference-item"
                  >
                    <img :src="img" />
                  </div>
                  <a-empty v-if="!selectedShot.usedImages?.length" :image="simpleImage" />
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
          <a-input v-model:value="characterForm.referenceImageUrl" placeholder="图片 URL" />
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
          <a-input v-model:value="sceneForm.referenceImageUrl" placeholder="图片 URL" />
        </a-form-item>
        <a-form-item label="参考视频">
          <a-input v-model:value="sceneForm.referenceVideoUrl" placeholder="视频 URL" />
        </a-form-item>
      </a-form>
    </a-modal>

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
              :src="img"
              style="width: 100px; height: 100px; object-fit: cover; margin: 4px"
            />
          </div>
        </div>
      </a-spin>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
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
  Progress as AProgress,
  Select as ASelect,
  SelectOption as ASelectOption,
  Space as ASpace,
  Spin as ASpin,
  Statistic as AStatistic,
  TabPane as ATabPane,
  Tabs as ATabs,
  Tag as ATag,
  Textarea as ATextarea,
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
  previewShotPromptApi,
  generateShotApi,
  batchGenerateShotsApi,
  composeProjectVideoApi,
  type Project,
  type Character,
  type Scene,
  type Shot,
  type ShotPreview,
} from '#/api/core/videoproject'

const route = useRoute()
const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE

// 状态
const project = ref<Project | null>(null)
const characters = ref<Character[]>([])
const scenes = ref<Scene[]>([])
const shots = ref<Shot[]>([])
const selectedShot = ref<Shot | null>(null)
const shotPreview = ref<ShotPreview | null>(null)

const leftTab = ref('characters')
const rightTab = ref('detail')


const activeProductionStep = ref('script')

const productionSteps = [
  { key: 'script', title: '剧本录入', description: '录入项目剧本，准备拆解资产和分镜' },
  { key: 'analysis', title: '资产分析', description: '分析人物、场景、物品和镜头需求' },
  { key: 'assets', title: '创建资产', description: '生成或上传人物、场景、物品、音频、视频资产' },
  { key: 'storyboard', title: '分镜设计', description: '设计分镜并绑定参考资产' },
  { key: 'compose', title: '剪辑合成', description: '批量生成镜头并合成为成片' },
]

const currentProductionStepIndex = computed(() =>
  productionSteps.findIndex((step) => step.key === activeProductionStep.value),
)

const activeProductionStepMeta = computed(() =>
  productionSteps.find((step) => step.key === activeProductionStep.value),
)

const completedShotCount = computed(
  () => shots.value.filter((shot) => shot.status === 'completed' && shot.videoUrl).length,
)

const productionPrimaryActionLabel = computed(() => {
  const actionMap: Record<string, string> = {
    script: '编辑项目信息',
    analysis: '查看资产',
    assets: '添加人物',
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
      leftTab.value = 'characters'
      showAddCharacter()
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
const shotLoading = ref(false)
const shotEditCardRef = ref<HTMLElement | null>(null)
const shotActionBusyIds = ref(new Set<string>())
const bindingShotIds = ref(new Set<string>())

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

function createEmptyShotForm() {
  return {
    name: '',
    actionDescription: '',
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
    shots.value = await listShotsApi(projectId.value)
    if (selectedShot.value) {
      selectedShot.value = shots.value.find((shot) => shot.id === selectedShot.value?.id) || null
    }
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
    actionDescription: shot.actionDescription,
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
  shotActionBusyIds.value.add(shot.id)
  shotActionBusyIds.value = new Set(shotActionBusyIds.value)
  try {
    await generateShotApi(shot.id)
    message.success('开始生成视频')
    await loadShots()
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
  generating.value = true
  batchProgress.value = { completed: 0, total: shots.value.length, failed: 0 }
  try {
    const result = await batchGenerateShotsApi(projectId.value)
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
  return char?.referenceImageUrl || ''
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
    const result = await composeProjectVideoApi(projectId.value, composeOptions.value)
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
    actionDescription: shot.actionDescription,
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
  grid-template-columns: minmax(220px, 280px) minmax(560px, 1fr) minmax(320px, auto);
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

.flow-title-copy {
  min-width: 0;

  h2 {
    margin: 0 0 2px;
    color: #f8fbff;
    font-size: 15px;
    font-weight: 700;
    letter-spacing: 0.2px;
    line-height: 1.25;
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

.step-item {
  min-width: 0;
  min-height: 36px;
  padding: 6px 10px;
  color: rgba(233, 242, 255, 0.7);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
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
  .detail-section p {
    overflow-wrap: anywhere;
    word-break: break-word;
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
    overflow: visible;
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
}

</style>
