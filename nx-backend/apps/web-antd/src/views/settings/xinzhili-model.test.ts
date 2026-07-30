import type { XinzhiliModelConfigView } from '#/api';

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { defineComponent, h, onBeforeUnmount, onMounted } from 'vue';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

import { normalizeXinzhiliModelConfigView } from './xinzhili-model-normalize';

function passthrough(name: string, tag = 'div') {
  return defineComponent({
    name,
    inheritAttrs: false,
    setup(_, { attrs, slots }) {
      return () => h(tag, attrs, slots.default?.());
    },
  });
}

const { credentialEmitters, mocks } = vi.hoisted(() => ({
  credentialEmitters: new Set<(status: Record<string, unknown>) => void>(),
  mocks: {
    accessCodes: [] as string[],
    credentialStatus: {
      apiKeySet: false,
      error: null as null | string,
      loading: true,
      saving: false,
      source: 'none',
      version: 0,
    },
  },
}));

vi.mock('@vben/stores', () => ({
  useAccessStore: () => ({
    get accessCodes() {
      return mocks.accessCodes;
    },
  }),
}));

vi.mock('ant-design-vue', () => {
  const Alert = defineComponent({
    name: 'AntAlertMock',
    inheritAttrs: false,
    props: {
      description: { default: '', type: String },
      message: { default: '', type: String },
    },
    setup(props, { attrs, slots }) {
      return () =>
        h('div', attrs, [props.message, props.description, slots.action?.()]);
    },
  });
  const Button = defineComponent({
    name: 'AntButtonMock',
    inheritAttrs: false,
    props: {
      disabled: { default: false, type: Boolean },
      href: { default: '', type: String },
      loading: { default: false, type: Boolean },
    },
    setup(props, { attrs, slots }) {
      return () =>
        h(
          props.href ? 'a' : 'button',
          {
            ...attrs,
            disabled: props.disabled || props.loading,
            href: props.href || undefined,
          },
          slots.default?.(),
        );
    },
  });
  const Input = defineComponent({
    name: 'AntInputMock',
    inheritAttrs: false,
    props: { value: { default: '', type: [Number, String] } },
    emits: ['update:value'],
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
  Input.Password = Input;
  const Form = passthrough('Form') as any;
  Form.Item = defineComponent({
    name: 'FormItem',
    inheritAttrs: false,
    props: { label: { default: '', type: String } },
    setup(props, { attrs, slots }) {
      return () => h('label', attrs, [props.label, slots.default?.()]);
    },
  });

  return {
    Alert,
    Button,
    Col: passthrough('Col'),
    Divider: passthrough('Divider'),
    Form,
    Input,
    InputNumber: Input,
    Row: passthrough('Row'),
    Select: passthrough('Select'),
    Switch: passthrough('Switch', 'button'),
    Textarea: Input,
    message: { success: vi.fn(), warning: vi.fn() },
  };
});

vi.mock('../site-config/components/editor-shell.vue', () => ({
  default: defineComponent({
    name: 'EditorShell',
    props: {
      loading: { default: false, type: Boolean },
      saveDisabled: { default: false, type: Boolean },
      saving: { default: false, type: Boolean },
    },
    emits: ['save'],
    setup(props, { emit, slots }) {
      return () =>
        h('main', [
          slots.default?.(),
          h(
            'button',
            {
              'data-testid': 'save-model',
              disabled: props.loading || props.saveDisabled || props.saving,
              onClick: () => emit('save'),
            },
            '保存',
          ),
        ]);
    },
  }),
}));

vi.mock('../voice/bailian-credentials-card.vue', () => ({
  default: defineComponent({
    name: 'BailianCredentialsCard',
    emits: ['status-change'],
    setup(_, { emit }) {
      const publish = (status: Record<string, unknown>) =>
        emit('status-change', status);
      credentialEmitters.add(publish);
      onMounted(() => publish({ ...mocks.credentialStatus }));
      onBeforeUnmount(() => credentialEmitters.delete(publish));
      return () => h('div', { 'data-testid': 'credential-card' });
    },
  }),
}));

vi.mock('#/api', () => ({
  getVoiceOptionsApi: vi.fn(),
  getXinzhiliModelConfigApi: vi.fn(),
  updateXinzhiliModelConfigApi: vi.fn(),
}));

import { message } from 'ant-design-vue';

import {
  getVoiceOptionsApi,
  getXinzhiliModelConfigApi,
  updateXinzhiliModelConfigApi,
} from '#/api';

import XinzhiliModelSettings from './xinzhili-model.vue';

const viewSource = readFileSync(
  resolve(__dirname, 'xinzhili-model.vue'),
  'utf8',
);
const apiSource = readFileSync(
  resolve(__dirname, '../../api/core/xinzhili-model-config.ts'),
  'utf8',
);
const apiIndexSource = readFileSync(
  resolve(__dirname, '../../api/core/index.ts'),
  'utf8',
);

function config(
  provider: 'bailian' | 'minimax' | 'openai-compatible' = 'bailian',
  enabled = true,
): XinzhiliModelConfigView {
  let ttsModel = 'custom-tts';
  if (provider === 'bailian') ttsModel = 'qwen3-tts-vc-2026-01-22';
  if (provider === 'minimax') ttsModel = 'speech-02-hd';

  return {
    commonPrompt: '',
    enabled,
    enabledModes: ['normal'],
    modePrompts: {},
    realtimeAsr: {
      apiKey: '',
      apiKeySet: true,
      apiKeySuffix: 'asr-old',
      endpoint: 'wss://dashscope.aliyuncs.com/api-ws/v1/inference',
      model: 'paraformer-realtime-v2',
      provider: 'aliyun-bailian',
      region: 'cn-beijing',
    },
    timing: {
      argumentCandidateSilenceMs: 350,
      comfortEndSilenceMs: 1200,
      comfortFirstPromptMs: 5000,
      comfortSecondPromptMs: 12_000,
      deepListeningEndSilenceMs: 1500,
      deepListeningPromptMs: 12_000,
      maxProactivePrompts: 2,
      normalEndSilenceMs: 700,
      partialStableMs: 150,
    },
    tts: {
      apiKey: '',
      apiKeySet: true,
      apiKeySuffix: 'tts-old',
      endpoint:
        provider === 'bailian' ? 'https://dashscope.aliyuncs.com/api/v1' : '',
      format: 'mp3',
      groupId: '',
      model: ttsModel,
      provider,
      voice: '',
    },
    version: 3,
  };
}

function credentialStatus(
  overrides: Partial<typeof mocks.credentialStatus> = {},
) {
  return {
    apiKeySet: true,
    error: null,
    loading: false,
    saving: false,
    source: 'shared',
    version: 2,
    ...overrides,
  };
}

async function emitCredentialStatus(status: Record<string, unknown>) {
  for (const publish of credentialEmitters) publish(status);
  await flushVuePromises();
}

async function mountSettings(view = config()) {
  vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue(view);
  const wrapper = mountVueComponent(XinzhiliModelSettings);
  await flushVuePromises();
  return wrapper;
}

function saveButton() {
  return document.body.querySelector(
    '[data-testid="save-model"]',
  ) as HTMLButtonElement;
}

describe('xinzhili model configuration page contract', () => {
  it('normalizes nullable collections from older backend responses', () => {
    const normalized = normalizeXinzhiliModelConfigView({
      ...config(),
      enabledModes: null,
      modePrompts: null,
    });

    expect(normalized.enabledModes).toEqual(['normal']);
    expect(normalized.modePrompts).toEqual({});
  });

  it('uses the dedicated versioned API and preserves blank private secrets', () => {
    expect(apiSource).toContain("'/xinzhili-model-config'");
    expect(apiSource).toContain('expectedVersion: number');
    expect(apiSource).toContain('apiKeySet: boolean');
    expect(apiIndexSource).toContain(
      "export * from './xinzhili-model-config';",
    );
    expect(viewSource).toContain('留空表示不修改');
    expect(viewSource).toContain('saved.version');
  });

  it('keeps normal enabled and handles body-marker version conflicts explicitly', () => {
    expect(viewSource).toContain("value: 'normal'");
    expect(viewSource).toContain(':disabled="mode.value === \'normal\'"');
    expect(viewSource).toContain('config_version_conflict');
    expect(viewSource).toContain('配置已被其他管理员修改，请重新加载');
  });

  it('keeps all TTS structure fields and removes duplicate ASR secret fields', () => {
    expect(viewSource).toContain("value: 'openai-compatible'");
    expect(viewSource).toContain("value: 'minimax'");
    expect(viewSource).toContain('form.realtimeAsr.endpoint');
    expect(viewSource).toContain('form.realtimeAsr.region');
    expect(viewSource).toContain('form.tts.endpoint');
    expect(viewSource).toContain('form.tts.model');
    expect(viewSource).toContain('form.tts.groupId');
    expect(viewSource).toContain('form.tts.voice');
    expect(viewSource).not.toContain('form.realtimeAsr.apiKey"');
    expect(viewSource).not.toContain('asrKeySet');
    expect(viewSource).not.toContain('asrKeySuffix');
  });
});

describe('xinzhili shared Bailian credential behavior', () => {
  beforeEach(() => {
    mocks.accessCodes = ['System:XinzhiliModel:Config'];
    mocks.credentialStatus = credentialStatus({
      apiKeySet: false,
      loading: true,
      source: 'none',
      version: 0,
    });
    vi.mocked(getVoiceOptionsApi).mockResolvedValue([]);
    vi.mocked(updateXinzhiliModelConfigApi).mockImplementation(
      async (payload) => ({
        ...config(payload.tts.provider, payload.enabled),
        realtimeAsr: {
          ...config().realtimeAsr,
          ...payload.realtimeAsr,
          apiKeySet: true,
          apiKeySuffix: 'shared',
        },
        tts: {
          ...config(payload.tts.provider).tts,
          ...payload.tts,
          apiKeySet: true,
          apiKeySuffix: 'private',
        },
        version: payload.expectedVersion + 1,
      }),
    );
  });

  afterEach(() => {
    document.body.innerHTML = '';
    credentialEmitters.clear();
    vi.clearAllMocks();
  });

  it('always renders the shared card but only exposes the voice-management link with its permission', async () => {
    const configOnly = await mountSettings(config('bailian', false));
    expect(
      document.body.querySelector('[data-testid="credential-card"]'),
    ).toBeTruthy();
    expect(document.body.textContent).not.toContain('前往人声管理克隆音色');
    configOnly.unmount();

    mocks.accessCodes = ['System:XinzhiliModel:Config', 'Voice:Profile:Manage'];
    const voiceManager = await mountSettings(config('bailian', false));
    const link = [...document.body.querySelectorAll('a')].find((item) =>
      item.textContent?.includes('前往人声管理克隆音色'),
    );
    expect(link?.getAttribute('href')).toBe('/voice/profiles');
    voiceManager.unmount();
  });

  it('fails closed when enabled until the shared credential is ready and blocks concurrent card saves', async () => {
    const wrapper = await mountSettings(config('bailian', true));
    expect(saveButton().disabled).toBe(true);
    expect(wrapper.text()).toContain('正在读取百炼公共 API Key');

    await emitCredentialStatus(credentialStatus());
    expect(saveButton().disabled).toBe(false);

    await emitCredentialStatus(credentialStatus({ saving: true }));
    expect(saveButton().disabled).toBe(true);
    expect(wrapper.text()).toContain('百炼公共 API Key 正在保存');

    await emitCredentialStatus(credentialStatus({ error: '读取失败' }));
    expect(saveButton().disabled).toBe(true);
    expect(wrapper.text()).toContain('可在上方重新加载');
    wrapper.unmount();
  });

  it('allows disabled model structure to save during credential load/error but not during credential save', async () => {
    const wrapper = await mountSettings(config('bailian', false));
    expect(saveButton().disabled).toBe(false);

    await emitCredentialStatus(
      credentialStatus({ apiKeySet: false, error: '读取失败' }),
    );
    expect(saveButton().disabled).toBe(false);

    await emitCredentialStatus(credentialStatus({ saving: true }));
    expect(saveButton().disabled).toBe(true);
    wrapper.unmount();
  });

  it('submits blank ASR and Bailian TTS keys without rendering duplicate secret inputs', async () => {
    mocks.credentialStatus = credentialStatus();
    const wrapper = await mountSettings(config('bailian', true));

    expect(
      document.body.querySelector('[data-testid="private-tts-api-key"]'),
    ).toBeNull();
    saveButton().click();
    await flushVuePromises();

    expect(updateXinzhiliModelConfigApi).toHaveBeenCalledWith(
      expect.objectContaining({
        realtimeAsr: expect.objectContaining({ apiKey: '' }),
        tts: expect.objectContaining({ apiKey: '', provider: 'bailian' }),
      }),
    );
    wrapper.unmount();
  });

  it.each(['minimax', 'openai-compatible'] as const)(
    'keeps the %s private TTS key input and submits its replacement value',
    async (provider) => {
      mocks.credentialStatus = credentialStatus();
      const wrapper = await mountSettings(config(provider, true));
      const input = document.body.querySelector(
        '[data-testid="private-tts-api-key"]',
      ) as HTMLInputElement;
      expect(input.placeholder).toContain('tts-old');
      expect(input.placeholder).toContain('留空表示不修改');

      input.value = `${provider}-private-key`;
      input.dispatchEvent(new Event('input', { bubbles: true }));
      saveButton().click();
      await flushVuePromises();

      expect(updateXinzhiliModelConfigApi).toHaveBeenCalledWith(
        expect.objectContaining({
          realtimeAsr: expect.objectContaining({ apiKey: '' }),
          tts: expect.objectContaining({
            apiKey: `${provider}-private-key`,
            provider,
          }),
        }),
      );
      wrapper.unmount();
    },
  );

  it('recognizes requestClient body-marker conflicts without an Axios status', async () => {
    mocks.credentialStatus = credentialStatus();
    vi.mocked(updateXinzhiliModelConfigApi).mockRejectedValueOnce({
      code: -1,
      error: 'config_version_conflict',
      message: 'config_version_conflict',
    });
    const wrapper = await mountSettings(config('bailian', true));
    saveButton().click();
    await flushVuePromises();

    expect(message.warning).toHaveBeenCalledWith(
      '配置已被其他管理员修改，请重新加载',
    );
    wrapper.unmount();
  });
});
