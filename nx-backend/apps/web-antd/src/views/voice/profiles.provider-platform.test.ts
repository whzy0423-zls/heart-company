import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, onBeforeUnmount, onMounted, ref } from 'vue';

import {
  cloneVoiceProfileApi,
  copyVoiceProfileToBailianApi,
  createVoiceProfileApi,
  getVoiceProfilesApi,
  uploadFileApi,
} from '#/api';
import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

import VoiceProfiles from './profiles.vue';
import {
  getBailianCopyFeedback,
  getBailianCloneFeedback,
  updateCopyingProfileIds,
} from './profiles-copy-feedback';

function passthrough(name: string, tag = 'div') {
  return defineComponent({
    inheritAttrs: false,
    name,
    setup(_, { attrs, slots }) {
      return () => h(tag, attrs, slots.default?.());
    },
  });
}

const { confirm, credentialEmitters, uploadEmitters } = vi.hoisted(() => ({
  confirm: vi.fn(),
  credentialEmitters: new Set<(status: Record<string, unknown>) => void>(),
  uploadEmitters: new Set<(payload: Record<string, unknown>) => void>(),
}));

vi.mock('@vben/common-ui', () => ({ Page: passthrough('Page') }));
vi.mock('@vben/icons', () => ({ IconifyIcon: passthrough('IconifyIcon') }));
vi.mock('@vben/stores', () => ({
  useAccessStore: () => ({ accessToken: 'test-token' }),
}));

vi.mock('ant-design-vue', () => {
  const Alert = defineComponent({
    inheritAttrs: false,
    name: 'Alert',
    props: {
      description: { default: '', type: String },
      message: { default: '', type: String },
    },
    setup(props, { attrs }) {
      return () => h('div', attrs, [props.message, props.description]);
    },
  });
  const Button = defineComponent({
    inheritAttrs: false,
    name: 'Button',
    props: {
      disabled: { default: false, type: Boolean },
      loading: { default: false, type: Boolean },
    },
    setup(props, { attrs, slots }) {
      return () =>
        h(
          'button',
          {
            ...attrs,
            'data-loading': String(props.loading),
            disabled: props.disabled || props.loading,
          },
          slots.default?.(),
        );
    },
  });
  const Form = passthrough('Form') as any;
  Form.Item = passthrough('FormItem');
  const Input = defineComponent({
    emits: ['update:value'],
    inheritAttrs: false,
    name: 'Input',
    props: { value: { default: '', type: String } },
    setup(props, { attrs, emit }) {
      return () =>
        h('input', {
          ...attrs,
          onInput: (event: Event) =>
            emit('update:value', (event.target as HTMLInputElement).value),
          value: props.value,
        });
    },
  }) as any;
  Input.TextArea = Input;
  const Table = defineComponent({
    inheritAttrs: false,
    name: 'Table',
    props: { dataSource: { default: () => [], type: Array } },
    setup(props, { slots }) {
      return () =>
        h(
          'div',
          { 'data-testid': 'voice-table' },
          props.dataSource.map((record) =>
            slots.bodyCell?.({ column: { key: 'action' }, record }),
          ),
        );
    },
  });
  const Upload = defineComponent({
    emits: ['change'],
    inheritAttrs: false,
    name: 'Upload',
    props: { disabled: { default: false, type: Boolean } },
    setup(props, { emit, slots }) {
      const trigger = (payload: Record<string, unknown>) =>
        emit('change', payload);
      uploadEmitters.add(trigger);
      onBeforeUnmount(() => uploadEmitters.delete(trigger));
      return () =>
        h(
          'div',
          {
            'data-disabled': String(props.disabled),
            'data-testid': 'voice-upload',
          },
          slots.default?.(),
        );
    },
  });

  return {
    Alert,
    Button,
    Card: passthrough('Card'),
    Col: passthrough('Col'),
    Form,
    Input,
    message: {
      error: vi.fn(),
      info: vi.fn(),
      success: vi.fn(),
      warning: vi.fn(),
    },
    Modal: { confirm },
    Row: passthrough('Row'),
    Select: passthrough('Select'),
    Space: passthrough('Space'),
    Table,
    Tag: passthrough('Tag'),
    Upload,
  };
});

vi.mock('#/api', () => ({
  cloneVoiceProfileApi: vi.fn(),
  copyVoiceProfileToBailianApi: vi.fn(),
  createVoiceProfileApi: vi.fn(),
  deleteVoiceProfileApi: vi.fn(),
  getVoiceProfilesApi: vi.fn(),
  uploadFileApi: vi.fn(),
}));

vi.mock('#/components/ellipsis-tooltip/table', () => ({
  ellipsisColumn: (dataIndex: string, title: string) => ({ dataIndex, title }),
}));

vi.mock('#/utils/upload-asset-preview', () => ({
  useUploadAssetPreviewResolver: () => ({
    resolve: (source?: string) => source,
  }),
  useUploadAssetPreviewUrl: () => ref(''),
}));

vi.mock('./bailian-credentials-card.vue', () => ({
  default: defineComponent({
    emits: ['status-change'],
    name: 'BailianCredentialsCard',
    setup(_, { emit }) {
      const publish = (status: Record<string, unknown>) =>
        emit('status-change', status);
      credentialEmitters.add(publish);
      onMounted(() =>
        publish({
          apiKeySet: false,
          error: null,
          loading: true,
          saving: false,
          source: 'none',
          version: 0,
        }),
      );
      onBeforeUnmount(() => credentialEmitters.delete(publish));
      return () => h('div', { 'data-testid': 'credential-card' });
    },
  }),
}));

const source = readFileSync(resolve(__dirname, 'profiles.vue'), 'utf8');
const apiSource = readFileSync(
  resolve(__dirname, '../../api/core/voice.ts'),
  'utf8',
);
const testPageSource = readFileSync(resolve(__dirname, 'test.vue'), 'utf8');
const contentPageSource = readFileSync(
  resolve(__dirname, 'content.vue'),
  'utf8',
);
const bailianCopyApiSource = apiSource.slice(
  apiSource.indexOf('export function copyVoiceProfileToBailianApi'),
  apiSource.indexOf('export function deleteVoiceProfileApi'),
);

const failedMiniMaxProfile = {
  id: 'minimax-1',
  name: '旧 MiniMax 人声',
  provider: 'minimax',
  sampleAssetId: 'asset-1',
  sampleUrl: '/sample.mp3',
  status: 'failed',
};

function credentialStatus(
  overrides: Partial<{
    apiKeySet: boolean;
    error: null | string;
    loading: boolean;
    saving: boolean;
    source: string;
    version: number;
  }> = {},
) {
  return {
    apiKeySet: true,
    error: null,
    loading: false,
    saving: false,
    source: 'shared',
    version: 1,
    ...overrides,
  };
}

async function emitCredentialStatus(status: Record<string, unknown>) {
  for (const publish of credentialEmitters) publish(status);
  await flushVuePromises();
}

function inputByPlaceholder(placeholder: string) {
  return document.body.querySelector(
    `input[placeholder="${placeholder}"]`,
  ) as HTMLInputElement;
}

async function fillCloneForm() {
  const input = inputByPlaceholder('例如：课程老师女声');
  input.value = '新老师人声';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  for (const trigger of uploadEmitters) {
    trigger({
      file: new File(['voice'], 'teacher.wav', { type: 'audio/wav' }),
    });
  }
  await flushVuePromises();
}

function actionButton(label: string) {
  return [...document.body.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === label,
  ) as HTMLButtonElement;
}

async function mountProfiles() {
  const wrapper = mountVueComponent(VoiceProfiles);
  await flushVuePromises();
  return wrapper;
}

describe('voice profile shared credential behavior', () => {
  beforeEach(() => {
    vi.mocked(getVoiceProfilesApi).mockReset();
    vi.mocked(uploadFileApi).mockReset();
    vi.mocked(createVoiceProfileApi).mockReset();
    vi.mocked(cloneVoiceProfileApi).mockReset();
    vi.mocked(copyVoiceProfileToBailianApi).mockReset();
    vi.mocked(getVoiceProfilesApi).mockResolvedValue({
      items: [failedMiniMaxProfile],
      total: 1,
    } as any);
    vi.mocked(uploadFileApi).mockResolvedValue({
      assetId: 'asset-new',
      name: 'teacher.wav',
      url: '/teacher.wav',
    } as any);
    vi.mocked(createVoiceProfileApi).mockResolvedValue({
      id: 'new-1',
      status: 'ready',
    } as any);
    vi.mocked(cloneVoiceProfileApi).mockResolvedValue({
      ...failedMiniMaxProfile,
      status: 'ready',
    } as any);
    vi.mocked(copyVoiceProfileToBailianApi).mockResolvedValue({
      ...failedMiniMaxProfile,
      provider: 'bailian',
      status: 'ready',
    } as any);
    confirm.mockReset();
  });

  afterEach(() => {
    document.body.innerHTML = '';
    credentialEmitters.clear();
    uploadEmitters.clear();
    vi.clearAllMocks();
  });

  it('fails closed, then follows configured, saving, loading, and error credential states', async () => {
    const wrapper = await mountProfiles();

    for (const label of [
      '上传音频',
      '保存并克隆',
      '重新克隆',
      '迁移到百炼 Qwen',
    ]) {
      expect(actionButton(label).disabled).toBe(true);
    }

    await emitCredentialStatus(credentialStatus());
    expect(actionButton('上传音频').disabled).toBe(false);
    expect(actionButton('重新克隆').disabled).toBe(false);
    expect(actionButton('迁移到百炼 Qwen').disabled).toBe(false);
    await fillCloneForm();
    expect(actionButton('保存并克隆').disabled).toBe(false);

    for (const unavailable of [
      credentialStatus({ saving: true }),
      credentialStatus({ loading: true }),
      credentialStatus({ error: '读取失败' }),
    ]) {
      await emitCredentialStatus(unavailable);
      for (const label of [
        '上传音频',
        '保存并克隆',
        '重新克隆',
        '迁移到百炼 Qwen',
      ]) {
        expect(actionButton(label).disabled).toBe(true);
      }
    }

    await emitCredentialStatus(credentialStatus());
    for (const label of [
      '上传音频',
      '保存并克隆',
      '重新克隆',
      '迁移到百炼 Qwen',
    ]) {
      expect(actionButton(label).disabled).toBe(false);
    }
    wrapper.unmount();
  });

  it('does not upload when the credential child is still fail-closed', async () => {
    const wrapper = await mountProfiles();
    for (const trigger of uploadEmitters) {
      trigger({
        file: new File(['voice'], 'teacher.wav', { type: 'audio/wav' }),
      });
    }
    await flushVuePromises();

    expect(uploadFileApi).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('serializes queued sample changes and keeps the form locked until the first upload settles', async () => {
    let finishUpload: (value: Record<string, unknown>) => void = () => {};
    vi.mocked(uploadFileApi).mockImplementation(
      () =>
        new Promise((resolve) => {
          finishUpload = resolve;
        }) as any,
    );
    const wrapper = await mountProfiles();
    await emitCredentialStatus(credentialStatus());
    const input = inputByPlaceholder('例如：课程老师女声');
    input.value = '新老师人声';
    input.dispatchEvent(new Event('input', { bubbles: true }));

    for (const trigger of uploadEmitters) {
      trigger({
        file: new File(['first'], 'first.wav', { type: 'audio/wav' }),
      });
      trigger({
        file: new File(['second'], 'second.wav', { type: 'audio/wav' }),
      });
    }
    await flushVuePromises();

    expect(uploadFileApi).toHaveBeenCalledOnce();
    expect(actionButton('上传音频').disabled).toBe(true);
    expect(actionButton('保存并克隆').disabled).toBe(true);
    const submitButton = actionButton('保存并克隆');
    submitButton.disabled = false;
    submitButton.click();
    await flushVuePromises();
    expect(createVoiceProfileApi).not.toHaveBeenCalled();

    finishUpload({
      assetId: 'asset-first',
      name: 'first.wav',
      url: '/first.wav',
    });
    await flushVuePromises();

    expect(uploadFileApi).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain('first.wav');
    expect(wrapper.text()).not.toContain('second.wav');
    expect(actionButton('上传音频').disabled).toBe(false);
    expect(actionButton('保存并克隆').disabled).toBe(false);
    wrapper.unmount();
  });

  it('rechecks credentials after migration confirmation and keeps the modal open when they expire', async () => {
    const wrapper = await mountProfiles();
    await emitCredentialStatus(credentialStatus());
    actionButton('迁移到百炼 Qwen').click();
    expect(confirm).toHaveBeenCalledOnce();

    await emitCredentialStatus(credentialStatus({ saving: true }));
    await expect(confirm.mock.calls[0]?.[0].onOk()).rejects.toThrow(
      'bailian_voice_clone_unavailable',
    );
    expect(copyVoiceProfileToBailianApi).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('serializes retry and migration for the same profile with one shared busy state', async () => {
    let finishCopy: (value: typeof failedMiniMaxProfile) => void = () => {};
    vi.mocked(copyVoiceProfileToBailianApi).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishCopy = resolve;
        }) as any,
    );
    const wrapper = await mountProfiles();
    await emitCredentialStatus(credentialStatus());
    actionButton('迁移到百炼 Qwen').click();
    const firstMigration = confirm.mock.calls[0]?.[0].onOk();
    await flushVuePromises();

    expect(copyVoiceProfileToBailianApi).toHaveBeenCalledOnce();
    expect(actionButton('重新克隆').disabled).toBe(true);
    expect(actionButton('迁移到百炼 Qwen').disabled).toBe(true);
    expect(actionButton('重新克隆').dataset.loading).toBe('true');
    expect(actionButton('迁移到百炼 Qwen').dataset.loading).toBe('true');

    const retryButton = actionButton('重新克隆');
    retryButton.disabled = false;
    retryButton.click();
    const secondMigration = confirm.mock.calls[0]?.[0].onOk();
    await flushVuePromises();

    expect(cloneVoiceProfileApi).not.toHaveBeenCalled();
    expect(copyVoiceProfileToBailianApi).toHaveBeenCalledOnce();
    await expect(secondMigration).rejects.toThrow('voice_profile_busy');

    finishCopy(failedMiniMaxProfile);
    await firstMigration;
    await flushVuePromises();
    wrapper.unmount();
  });
});

describe('voice profile clone provider platform', () => {
  it('creates all new voice profiles with Aliyun Bailian Qwen', () => {
    expect(source).toContain("provider: 'bailian'");
    expect(source).not.toContain('const voiceProviderOptions');
    expect(source).not.toContain("value: 'minimax'");
    expect(source).not.toContain('v-model:value="form.provider"');
    expect(source).toContain('provider: form.provider');
    expect(source).toContain('阿里百炼 Qwen 声音复刻');
    expect(source).toContain('qwen-audio-3.0-tts-flash');
    expect(source).not.toContain('请先在芯之力模型配置中保存百炼 API Key');
    expect(source).toContain('保存公共 Key → 上传 10～20 秒干声样本 → 克隆 → 芯之力选择');
  });

  it('shows the clone platform in the profile list', () => {
    expect(source).toContain("dataIndex: 'provider'");
    expect(source).toContain('platformLabel(record.provider)');
    expect(source).toContain('platformColor(record.provider)');
  });

  it('extends voice options with provider for same-provider reuse', () => {
    const optionType = apiSource.slice(
      apiSource.indexOf('export interface VoiceOption'),
      apiSource.indexOf('}', apiSource.indexOf('export interface VoiceOption')),
    );
    expect(optionType).toContain('provider: string');
  });

  it('switches synthesis models when a Bailian Qwen voice is selected', () => {
    for (const pageSource of [testPageSource, contentPageSource]) {
      expect(pageSource).toContain("provider === 'bailian'");
      expect(pageSource).toContain('qwen-audio-3.0-tts-flash');
      expect(pageSource).toContain("'speech-02-hd'");
      expect(pageSource).toContain('watch(');
    }
  });
});

describe('voice cloning requires the shared Bailian credential', () => {
  it('shows the shared credential card before the new voice form', () => {
    expect(source).toContain(
      "import BailianCredentialsCard from './bailian-credentials-card.vue'",
    );
    expect(source.indexOf('<BailianCredentialsCard')).toBeLessThan(
      source.indexOf('<div class="card-title">新增人声</div>'),
    );
  });

  it('uses credential status as the single gate for cloning actions', () => {
    expect(source).toContain(
      'const credentialStatus = ref<BailianCredentialsCardStatus>',
    );
    expect(source).toContain('function handleCredentialStatusChange');
    expect(source).toContain('const canCloneVoice = computed(');
    expect(source).toContain('credentialStatus.value.apiKeySet &&');
    expect(source).toContain('!credentialStatus.value.loading &&');
    expect(source).toContain('!credentialStatus.value.error');
    expect(source).toContain('!credentialStatus.value.saving');
    expect(source).toContain('function ensureCanCloneVoice()');
    expect(source).toContain('请先保存百炼公共 API Key');
    expect(source).toContain('百炼凭证读取失败，可在上方重新加载');
  });

  it('disables upload, create, retry, and migration while the credential gate is closed', () => {
    expect(source).toContain(':disabled="!canCloneVoice || formSaving"');
    expect(source).toContain(':disabled="!canSubmit"');
    expect(source).toContain(
      ':disabled="!canCloneVoice || isProfileBusy(record.id)"',
    );
    expect(source).toContain('if (!ensureCanCloneVoice()) return;');
    expect(source).toContain('@status-change="handleCredentialStatusChange"');
  });

  it('keeps public credentials out of clone request bodies and form resets', () => {
    const createRequestSource = source.slice(
      source.indexOf('const result = await createVoiceProfileApi({'),
      source.indexOf('showBailianCloneFeedback(result);'),
    );
    const resetFormSource = source.slice(
      source.indexOf('function resetForm()'),
      source.indexOf('function search()'),
    );
    expect(createRequestSource).not.toContain('apiKey');
    expect(resetFormSource).not.toContain('credentialStatus');
  });
});

describe('copy MiniMax profile to Bailian', () => {
  it('posts the selected profile to the Bailian-copy endpoint with the clone timeout', () => {
    expect(bailianCopyApiSource).toContain(
      'export function copyVoiceProfileToBailianApi(id: string)',
    );
    expect(bailianCopyApiSource).toContain(
      '`/voice/profiles/${id}/copy-to-bailian`',
    );
    expect(bailianCopyApiSource).toContain('timeout: 180_000');
  });

  it('only exposes the Bailian-copy action for MiniMax profiles with a sample asset', () => {
    expect(source).toContain("record.provider === 'minimax'");
    expect(source).toContain('record.sampleAssetId');
    expect(source).toContain("record.status !== 'migrated'");
    expect(source).toContain('迁移到百炼 Qwen');
  });

  it('confirms migration to Qwen and deactivation of the old MiniMax profile', () => {
    expect(source).toContain("title: '迁移到百炼 Qwen'");
    expect(source).toContain('迁移成功后停用原 MiniMax 音色');
    expect(source).toContain('复用原音频样本');
    expect(source).toContain("['draft', 'failed'].includes(record.status)");
  });

  it('uses one per-profile Set for retry and copy loading and refreshes after the API call', () => {
    expect(source).toContain('const busyProfileIds = ref(new Set<string>())');
    expect(source).toContain(':loading="isProfileBusy(record.id)"');
    expect(source).toContain('if (!beginProfileOperation(record.id))');
    expect(source).toContain('await copyVoiceProfileToBailianApi(record.id)');
    expect(source).toContain('await load()');
  });

  it('delegates rejected copy errors to the shared request interceptor', () => {
    const copyHandlerSource = source.slice(
      source.indexOf('function copyProfileToBailian'),
      source.indexOf('function profileRecord'),
    );
    expect(copyHandlerSource).not.toContain('message.error');
    expect(copyHandlerSource).toContain('catch {');
  });
});

describe('Bailian copy feedback', () => {
  it.each([
    ['ready', '', 'success', '已迁移到百炼 Qwen，可到芯之力模型配置选择'],
    [
      'cloning',
      '',
      'info',
      '已提交百炼 Qwen 迁移，正在处理中，请稍后刷新查看状态',
    ],
    [
      'draft',
      '',
      'info',
      '已提交百炼 Qwen 迁移，正在处理中，请稍后刷新查看状态',
    ],
    ['failed', '百炼服务暂不可用', 'error', '百炼服务暂不可用'],
  ])(
    'maps %s responses to accurate user feedback',
    (status, lastError, type, content) => {
      expect(getBailianCopyFeedback({ lastError, status })).toEqual({
        content,
        type,
      });
    },
  );

  it('removes only the completed profile from concurrent copy loading', () => {
    const copying = updateCopyingProfileIds(new Set(), 'minimax-1', true);
    const concurrentCopying = updateCopyingProfileIds(
      copying,
      'minimax-2',
      true,
    );

    expect(
      updateCopyingProfileIds(concurrentCopying, 'minimax-1', false),
    ).toEqual(new Set(['minimax-2']));
  });
});

describe('Bailian clone feedback', () => {
  it.each([
    ['ready', '', 'success', '百炼 Qwen 音色克隆成功，可到芯之力模型配置选择'],
    ['cloning', '', 'info', '百炼 Qwen 音色正在克隆，请稍后刷新查看状态'],
    ['draft', '', 'info', '百炼 Qwen 音色正在克隆，请稍后刷新查看状态'],
    ['failed', 'API Key 无效', 'error', 'API Key 无效'],
  ])(
    'maps %s responses to accurate clone feedback',
    (status, lastError, type, content) => {
      expect(getBailianCloneFeedback({ lastError, status })).toEqual({
        content,
        type,
      });
    },
  );

  it('uses the returned clone status for create and retry actions', () => {
    expect(source).toContain('const result = await createVoiceProfileApi({');
    expect(source).toContain('showBailianCloneFeedback(result);');
    expect(source).toContain(
      'const result = await cloneVoiceProfileApi(record.id);',
    );
  });
});
