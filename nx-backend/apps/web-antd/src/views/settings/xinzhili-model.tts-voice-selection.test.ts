import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';

import type { XinzhiliModelConfigView } from '#/api';
import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

function passthrough(name: string, tag = 'div') {
  return defineComponent({
    inheritAttrs: false,
    name,
    setup(_, { attrs, slots }) {
      return () => h(tag, attrs, slots.default?.());
    },
  });
}

vi.mock('ant-design-vue', () => {
  const Input = defineComponent({
    emits: ['update:value'],
    inheritAttrs: false,
    name: 'Input',
    props: { value: { default: '', type: [Number, String] } },
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
    inheritAttrs: false,
    name: 'FormItem',
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
      emits: ['change', 'update:value'],
      inheritAttrs: false,
      name: 'Select',
      props: {
        options: { default: () => [], type: Array },
        placeholder: { default: '', type: String },
        value: { default: '', type: String },
      },
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
      comfortSecondPromptMs: 12000,
      deepListeningEndSilenceMs: 1500,
      deepListeningPromptMs: 12000,
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
});
