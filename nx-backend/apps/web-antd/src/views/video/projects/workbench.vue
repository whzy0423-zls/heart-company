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
          <a-button type="primary" @click="composeVideo" :loading="composing" :disabled="generating">
            合成视频
          </a-button>
          <span v-if="composing && composeProgress > 0" class="progress-text">
            {{ composeProgress }}%
          </span>
        </a-space>
      </div>
    </div>

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
            <a-button @click="showAddShot">
              <PlusOutlined /> 添加分镜
            </a-button>
            <a-space>
              <a-button type="primary" @click="generateAllShots" :loading="generating" :disabled="composing">
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
            :class="{ active: selectedShot?.id === shot.id }"
            @click="selectShot(shot)"
            @dragover.prevent
            @drop.stop="onDropToShot($event, shot)"
          >
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
                    <a-menu-item @click="generateShot(shot)">
                      <PlayCircleOutlined /> 生成视频
                    </a-menu-item>
                    <a-menu-item @click="editShot(shot)">
                      <EditOutlined /> 编辑
                    </a-menu-item>
                    <a-menu-divider />
                    <a-menu-item danger @click="deleteShot(shot.id)">
                      <DeleteOutlined /> 删除
                    </a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
            </div>
          </div>

          <a-empty v-if="shots.length === 0" description="暂无分镜，点击添加分镜开始创作" />
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

    <!-- 添加分镜对话框 -->
    <a-modal
      v-model:open="shotModalVisible"
      :title="editingShotId ? '编辑分镜' : '添加分镜'"
      @ok="handleAddShot"
      :confirmLoading="shotLoading"
      width="600px"
    >
      <a-form :model="shotForm" layout="vertical">
        <a-form-item label="分镜名称">
          <a-input v-model:value="shotForm.name" placeholder="可选" />
        </a-form-item>
        <a-form-item label="动作描述" required>
          <a-textarea
            v-model:value="shotForm.actionDescription"
            placeholder="描述角色动作和场景变化"
            :rows="3"
          />
        </a-form-item>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="时长（秒）">
              <a-select v-model:value="shotForm.duration" style="width: 100%">
                <a-select-option :value="5">5 秒</a-select-option>
                <a-select-option :value="10">10 秒</a-select-option>
                <a-select-option :value="15">15 秒</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="画面比例">
              <a-select v-model:value="shotForm.aspectRatio">
                <a-select-option value="16:9">16:9</a-select-option>
                <a-select-option value="9:16">9:16</a-select-option>
                <a-select-option value="1:1">1:1</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="选择角色">
          <a-select
            v-model:value="shotForm.characterIds"
            mode="multiple"
            placeholder="选择出现的角色"
          >
            <a-select-option v-for="char in characters" :key="char.id" :value="char.id">
              {{ char.name }}
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="选择场景">
          <a-select v-model:value="shotForm.sceneId" placeholder="选择场景">
            <a-select-option v-for="scene in scenes" :key="scene.id" :value="scene.id">
              {{ scene.name }}
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="镜头运动">
          <a-select v-model:value="shotForm.cameraMovement">
            <a-select-option value="">无</a-select-option>
            <a-select-option value="push">推镜</a-select-option>
            <a-select-option value="pull">拉镜</a-select-option>
            <a-select-option value="pan">横摇</a-select-option>
            <a-select-option value="tilt">竖摇</a-select-option>
          </a-select>
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
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Empty, Modal, message } from 'ant-design-vue'
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
const shotModalVisible = ref(false)
const previewModalVisible = ref(false)
const projectModalVisible = ref(false)

const editingCharacterId = ref('')
const editingSceneId = ref('')
const editingShotId = ref('')

const characterLoading = ref(false)
const sceneLoading = ref(false)
const shotLoading = ref(false)

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

const shotForm = ref({
  name: '',
  actionDescription: '',
  duration: 15,
  aspectRatio: '16:9',
  characterIds: [] as string[],
  sceneId: '',
  cameraMovement: '',
})

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
  editingShotId.value = ''
  shotForm.value = {
    name: '',
    actionDescription: '',
    duration: 15,
    aspectRatio: '16:9',
    characterIds: [],
    sceneId: '',
    cameraMovement: '',
  }
  shotModalVisible.value = true
}

async function handleAddShot() {
  if (!shotForm.value.actionDescription) {
    message.warning('请填写动作描述')
    return
  }

  try {
    shotLoading.value = true
    if (editingShotId.value) {
      await updateShotApi(editingShotId.value, shotForm.value)
      message.success('更新成功')
    } else {
      await createShotApi(projectId.value, {
        ...shotForm.value,
        orderNum: shots.value.length + 1,
      })
      message.success('添加成功')
    }
    editingShotId.value = ''
    shotModalVisible.value = false
    await loadShots()
  } catch (error) {
    message.error('添加失败')
  } finally {
    shotLoading.value = false
  }
}

function editShot(shot: Shot) {
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
  shotModalVisible.value = true
}

async function deleteShot(shotId: string) {
  Modal.confirm({
    title: '确认删除',
    content: '确定要删除这个分镜吗？',
    onOk: async () => {
      try {
        await deleteShotApi(shotId)
        message.success('删除成功')
        await loadShots()
      } catch (error) {
        message.error('删除失败')
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
  try {
    await generateShotApi(shot.id)
    message.success('开始生成视频')
    await loadShots()
  } catch (error) {
    message.error('生成失败')
  }
}

async function generateAllShots() {
  generating.value = true
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
  const type = event.dataTransfer?.getData('type')
  const raw = event.dataTransfer?.getData('data')
  if (!type || !raw) return

  try {
    const resource = JSON.parse(raw) as Character | Scene
    const payload = shotToPayload(shot)

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
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid #f0f0f0;
}

.header-actions {
  display: flex;
  gap: 8px;
}

.workbench-layout {
  flex: 1;
  overflow: hidden;
}

.workbench-sider,
.workbench-sider-right {
  background: #fff;
  border-right: 1px solid #f0f0f0;
  overflow-y: auto;
}

.workbench-sider-right {
  border-right: none;
  border-left: 1px solid #f0f0f0;
}

.resource-panel {
  padding: 16px;
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
  padding: 16px;
  overflow-y: auto;
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

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
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
</style>
