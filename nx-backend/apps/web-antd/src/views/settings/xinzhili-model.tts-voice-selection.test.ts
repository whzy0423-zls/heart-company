import type { XinzhiliModelConfigView } from '#/api';

import { defineComponent, h, onMounted } from 'vue';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

function passthrough(name: string, tag = 'div') {
  return defineComponent({
    name,
    inheritAttrs: false,
    setup(_, { attrs, slots }) {
      return () => h(tag, attrs, slots.default?.());
    },
  });
}

vi.mock('@vben/stores', () => ({
  useAccessStore: () => ({
    accessCodes: ['System:XinzhiliModel:Config'],
  }),
}));

vi.mock('ant-design-vue', () => {
  const Input = defineComponent({
    name: 'Input',
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
    Alert: passthrough('Alert'),
    Button: passthrough('Button', 'button'),
    Col: passthrough('Col'),
    Divider: passthrough('Divider'),
    Form,
    Input,
    InputNumber: Input,
    Row: passthrough('Row'),
    Select: defineComponent({
      name: 'Select',
      inheritAttrs: false,
      props: {
        options: { default: () => [], type: Array },
        placeholder: { default: '', type: String },
        value: { default: '', type: String },
      },
      emits: ['change', 'update:value'],
      setup(props, { attrs, emit }) {
        const optionNodes = () =>
          (
            props.options as Array<{
              label: string;
              options?: Array<{ label: string; value: string }>;
              value?: string;
            }>
          ).flatMap((option) =>
            option.options
              ? option.options.map((child) =>
                  h('option', { value: child.value }, child.label),
                )
              : [h('option', { value: option.value }, option.label)],
          );
        return () =>
          h(
            'select',
            {
              ...attrs,
              onChange: (event: Event) => {
                const value = (event.target as HTMLSelectElement).value;
                emit('update:value', value);
                emit('change', value);
              },
              value: props.value,
            },
            [
              h('option', { disabled: true, value: '' }, props.placeholder),
              ...optionNodes(),
            ],
          );
      },
    }),
    Switch: passthrough('Switch', 'button'),
    Textarea: Input,
    message: { success: vi.fn(), warning: vi.fn() },
  };
});

vi.mock('../site-config/components/editor-shell.vue', () => ({
  default: {
    name: 'EditorShell',
    template:
      '<main><slot /><button data-testid="save-model" @click="$emit(\'save\')">保存</button></main>',
  },
}));

vi.mock('../voice/bailian-credentials-card.vue', () => ({
  default: defineComponent({
    name: 'BailianCredentialsCard',
    emits: ['status-change'],
    setup(_, { emit }) {
      onMounted(() =>
        emit('status-change', {
          apiKeySet: true,
          error: null,
          loading: false,
          saving: false,
          source: 'shared',
          version: 1,
        }),
      );
      return () => h('div', { 'data-testid': 'credential-card' });
    },
  }),
}));

vi.mock('#/api', () => ({
  getVoiceOptionsApi: vi.fn(),
  getXinzhiliModelConfigApi: vi.fn(),
  updateXinzhiliModelConfigApi: vi.fn(),
}));

import {
  getVoiceOptionsApi,
  getXinzhiliModelConfigApi,
  updateXinzhiliModelConfigApi,
} from '#/api';

import XinzhiliModelSettings from './xinzhili-model.vue';

const bailianClone = {
  id: 'clone:bailian-profile',
  label: '百炼已复刻音色',
  model: 'qwen3-tts-vc-2026-01-22',
  provider: 'bailian',
  source: 'clone' as const,
  voiceId: 'bailian-voice-id',
  voiceName: '百炼已复刻音色',
};
const minimaxClone = {
  id: 'clone:minimax-profile',
  label: 'MiniMax 已克隆音色',
  model: 'speech-02-hd',
  provider: 'minimax',
  source: 'clone' as const,
  voiceId: 'minimax-voice-id',
  voiceName: 'MiniMax 已克隆音色',
};

function config(
  provider: 'bailian' | 'minimax' | 'openai-compatible' = 'minimax',
): XinzhiliModelConfigView {
  return {
    commonPrompt: '',
    enabled: false,
    enabledModes: ['normal'],
    modePrompts: {},
    realtimeAsr: {
      apiKey: '',
      apiKeySet: false,
      apiKeySuffix: '',
      endpoint: 'wss://dashscope.aliyuncs.com/api-ws/v1/inference',
      model: 'paraformer-realtime-v2' as const,
      provider: 'aliyun-bailian' as const,
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
      apiKeySuffix: '1234',
      endpoint: '',
      format: 'mp3' as const,
      groupId: '',
      model: '',
      provider,
      voice: '',
    },
    version: 3,
  };
}

function selectValue(select: HTMLSelectElement, value: string) {
  select.value = value;
  select.dispatchEvent(new Event('change', { bubbles: true }));
}

function inputByTestId(testId: string) {
  return document.body.querySelector(
    `[data-testid="${testId}"]`,
  ) as HTMLInputElement;
}

function setInput(input: HTMLInputElement, value: string) {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

describe('xinzhili Bailian TTS voice selection', () => {
  beforeEach(() => {
    vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue(config());
    vi.mocked(getVoiceOptionsApi).mockResolvedValue([
      bailianClone,
      minimaxClone,
    ]);
    vi.mocked(updateXinzhiliModelConfigApi).mockImplementation(
      async (data) => ({
        ...config(data.tts.provider),
        tts: {
          ...config(data.tts.provider).tts,
          ...data.tts,
          apiKeySet: true,
          apiKeySuffix: '1234',
        },
        version: data.expectedVersion + 1,
      }),
    );
  });

  afterEach(() => {
    document.body.innerHTML = '';
    vi.clearAllMocks();
  });

  async function mountSettings() {
    const wrapper = mountVueComponent(XinzhiliModelSettings);
    await flushVuePromises();
    return wrapper;
  }

  it('saves a selected ready Bailian clone as its Bailian voice id', async () => {
    const wrapper = await mountSettings();
    const selects = document.body.querySelectorAll('select');

    selectValue(selects[0]!, 'bailian');
    await flushVuePromises();
    selectValue(document.body.querySelectorAll('select')[1]!, bailianClone.id);
    await flushVuePromises();
    (
      document.body.querySelector(
        '[data-testid="save-model"]',
      ) as HTMLButtonElement
    ).click();
    await flushVuePromises();

    expect(updateXinzhiliModelConfigApi).toHaveBeenCalledWith(
      expect.objectContaining({
        tts: expect.objectContaining({
          apiKey: '',
          model: bailianClone.model,
          provider: 'bailian',
          voice: bailianClone.voiceId,
        }),
      }),
    );
    expect(
      vi.mocked(updateXinzhiliModelConfigApi).mock.calls[0]?.[0].tts.voice,
    ).not.toBe(bailianClone.id);
    wrapper.unmount();
  });

  it('only displays voices from the active MiniMax or Bailian provider', async () => {
    const wrapper = await mountSettings();

    expect(document.body.querySelectorAll('select')[1]!.textContent).toContain(
      minimaxClone.label,
    );
    expect(
      document.body.querySelectorAll('select')[1]!.textContent,
    ).not.toContain(bailianClone.label);

    selectValue(document.body.querySelectorAll('select')[0]!, 'bailian');
    await flushVuePromises();

    expect(document.body.querySelectorAll('select')[1]!.textContent).toContain(
      bailianClone.label,
    );
    expect(
      document.body.querySelectorAll('select')[1]!.textContent,
    ).not.toContain(minimaxClone.label);
    wrapper.unmount();
  });

  it('hides the existing voice picker for OpenAI-compatible TTS', async () => {
    const wrapper = await mountSettings();

    expect(document.body.querySelectorAll('select')).toHaveLength(2);
    selectValue(document.body.querySelector('select')!, 'openai-compatible');
    await flushVuePromises();

    expect(document.body.querySelectorAll('select')).toHaveLength(1);
    wrapper.unmount();
  });

  it('restores the Bailian voice picker for the native DashScope endpoint saved under the legacy provider', async () => {
    vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue({
      ...config('openai-compatible'),
      tts: {
        ...config('openai-compatible').tts,
        endpoint: 'https://dashscope.aliyuncs.com/api/v1',
        model: 'qwen3-tts-vc-2026-01-22',
      },
    });

    const wrapper = await mountSettings();
    const selects = document.body.querySelectorAll('select');

    expect(selects).toHaveLength(2);
    expect(selects[0]!.value).toBe('bailian');
    expect(selects[1]!.textContent).toContain(bailianClone.label);
    wrapper.unmount();
  });

  it('defaults an unconfigured legacy TTS record to Bailian voice reuse', async () => {
    vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue(
      config('openai-compatible'),
    );

    const wrapper = await mountSettings();
    const selects = document.body.querySelectorAll('select');

    expect(selects).toHaveLength(2);
    expect(selects[0]!.value).toBe('bailian');
    expect(selects[1]!.textContent).toContain(bailianClone.label);
    wrapper.unmount();
  });

  it('clears the MiniMax private key and provider-bound fields when switching to OpenAI-compatible', async () => {
    vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue({
      ...config('minimax'),
      tts: {
        ...config('minimax').tts,
        endpoint: 'https://api.minimax.chat/v1/t2a_v2',
        model: 'speech-02-hd',
        voice: 'minimax-old-voice',
      },
    });
    const wrapper = await mountSettings();
    setInput(inputByTestId('private-tts-api-key'), 'minimax-draft-key');

    selectValue(document.body.querySelector('select')!, 'openai-compatible');
    await flushVuePromises();

    expect(inputByTestId('private-tts-api-key').value).toBe('');
    expect(inputByTestId('private-tts-api-key').placeholder).toBe(
      '请输入 API Key',
    );
    expect(inputByTestId('tts-endpoint').value).toBe('');
    expect(inputByTestId('tts-model').value).toBe('');
    expect(inputByTestId('tts-voice').value).toBe('');
    wrapper.unmount();
  });

  it('forces the official Bailian preset when switching from a custom endpoint', async () => {
    vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue({
      ...config('openai-compatible'),
      tts: {
        ...config('openai-compatible').tts,
        endpoint: 'https://custom.example.com/tts',
        model: 'custom-model',
        voice: 'custom-voice',
      },
    });
    const wrapper = await mountSettings();

    selectValue(document.body.querySelector('select')!, 'bailian');
    await flushVuePromises();

    expect(inputByTestId('tts-endpoint').value).toBe(
      'https://dashscope.aliyuncs.com/api/v1',
    );
    expect(inputByTestId('tts-model').value).toBe('qwen-audio-3.0-tts-flash');
    expect(inputByTestId('tts-voice').value).toBe('');
    expect(inputByTestId('private-tts-api-key')).toBeNull();
    wrapper.unmount();
  });

  it('clears the official Bailian preset when switching to OpenAI-compatible', async () => {
    vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue({
      ...config('bailian'),
      tts: {
        ...config('bailian').tts,
        endpoint: 'https://dashscope.aliyuncs.com/api/v1',
        model: 'qwen3-tts-vc-2026-01-22',
        voice: 'bailian-old-voice',
      },
    });
    const wrapper = await mountSettings();

    selectValue(document.body.querySelector('select')!, 'openai-compatible');
    await flushVuePromises();

    expect(inputByTestId('tts-endpoint').value).toBe('');
    expect(inputByTestId('tts-model').value).toBe('');
    expect(inputByTestId('tts-voice').value).toBe('');
    expect(inputByTestId('private-tts-api-key').placeholder).toBe(
      '请输入 API Key',
    );
    wrapper.unmount();
  });

  it('forces the MiniMax endpoint and model when switching providers', async () => {
    vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue({
      ...config('openai-compatible'),
      tts: {
        ...config('openai-compatible').tts,
        endpoint: 'https://custom.example.com/tts',
        model: 'custom-model',
      },
    });
    const wrapper = await mountSettings();

    selectValue(document.body.querySelector('select')!, 'minimax');
    await flushVuePromises();

    expect(inputByTestId('tts-endpoint').value).toBe(
      'https://api.minimax.chat/v1/t2a_v2',
    );
    expect(inputByTestId('tts-model').value).toBe('speech-02-hd');
    wrapper.unmount();
  });

  it('preserves a private key for same-origin path edits and clears it across origins', async () => {
    vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue({
      ...config('minimax'),
      tts: {
        ...config('minimax').tts,
        endpoint: 'https://api.minimax.chat/v1/t2a_v2',
        model: 'speech-02-hd',
      },
    });
    const wrapper = await mountSettings();
    setInput(inputByTestId('private-tts-api-key'), 'minimax-draft-key');

    setInput(
      inputByTestId('tts-endpoint'),
      'https://api.minimax.chat/v1/t2a_v2/preview/',
    );
    await flushVuePromises();
    expect(inputByTestId('private-tts-api-key').value).toBe(
      'minimax-draft-key',
    );
    expect(inputByTestId('private-tts-api-key').placeholder).toContain('1234');

    setInput(inputByTestId('tts-endpoint'), 'https://proxy.example.com/v1');
    await flushVuePromises();
    expect(inputByTestId('private-tts-api-key').value).toBe('');
    expect(inputByTestId('private-tts-api-key').placeholder).toBe(
      '请输入 API Key',
    );
    wrapper.unmount();
  });

  it.each([
    'http://dashscope.aliyuncs.com/api/v1',
    'https://dashscope.aliyuncs.com.evil.example/api/v1',
    'https://user@dashscope.aliyuncs.com/api/v1',
    'https://dashscope.aliyuncs.com:8443/api/v1',
    'https://dashscope.aliyuncs.com/api/v1?token=bad',
    'https://dashscope.aliyuncs.com/api/v1#fragment',
    'https://proxy.example.com/v1',
    'https://dashscope.aliyuncs.com/evil/v1',
    'https://dashscope.aliyuncs.com/api/v1/not-a-tts-endpoint',
    'https://dashscope.aliyuncs.com/compatible-mode/v1/not-a-tts-endpoint',
    'https://dashscope.aliyuncs.com/evil/../api/v1',
    'https://dashscope.aliyuncs.com/%61pi/v1',
    'https://dashscope.aliyuncs.com/%2e%2e/api/v1',
  ])(
    'treats non-official native Bailian endpoint %s as private',
    async (endpoint) => {
      vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue({
        ...config('bailian'),
        tts: {
          ...config('bailian').tts,
          endpoint,
          model: 'qwen-audio-3.0-tts-flash',
        },
      });
      const wrapper = await mountSettings();
      const input = inputByTestId('private-tts-api-key');
      expect(input).toBeTruthy();
      setInput(input, 'native-private-key');
      (
        document.body.querySelector(
          '[data-testid="save-model"]',
        ) as HTMLButtonElement
      ).click();
      await flushVuePromises();

      expect(updateXinzhiliModelConfigApi).toHaveBeenCalledWith(
        expect.objectContaining({
          tts: expect.objectContaining({
            apiKey: 'native-private-key',
            endpoint,
            provider: 'bailian',
          }),
        }),
      );
      wrapper.unmount();
    },
  );

  it.each([
    'https://dashscope.aliyuncs.com/api/v1',
    'https://dashscope.aliyuncs.com:443/compatible-mode/v1/',
  ])(
    'uses the shared key for official DashScope endpoint %s',
    async (endpoint) => {
      vi.mocked(getXinzhiliModelConfigApi).mockResolvedValue({
        ...config('openai-compatible'),
        tts: {
          ...config('openai-compatible').tts,
          endpoint,
          model: 'qwen-audio-tts-latest',
        },
      });
      const wrapper = await mountSettings();
      expect(inputByTestId('private-tts-api-key')).toBeNull();
      (
        document.body.querySelector(
          '[data-testid="save-model"]',
        ) as HTMLButtonElement
      ).click();
      await flushVuePromises();

      expect(updateXinzhiliModelConfigApi).toHaveBeenCalledWith(
        expect.objectContaining({
          tts: expect.objectContaining({ apiKey: '', endpoint }),
        }),
      );
      wrapper.unmount();
    },
  );
});
