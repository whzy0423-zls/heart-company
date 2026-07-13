import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';

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
  Input.TextArea = passthrough('InputTextArea', 'textarea');

  const Form = passthrough('Form') as any;
  Form.Item = defineComponent({
    inheritAttrs: false,
    name: 'FormItem',
    props: {
      label: { default: '', type: String },
      required: { default: false, type: Boolean },
      rules: { default: undefined, type: [Array, Object] },
    },
    setup(props, { attrs, slots }) {
      return () =>
        h('label', attrs, [
          props.label,
          props.required ? '（必选）' : '',
          slots.default?.(),
        ]);
    },
  });

  return {
    Alert: defineComponent({
      inheritAttrs: false,
      name: 'Alert',
      props: {
        description: { default: '', type: String },
        message: { default: '', type: String },
      },
      setup(props, { attrs }) {
        return () => h('div', attrs, [props.message, props.description]);
      },
    }),
    Button: passthrough('Button', 'button'),
    Col: passthrough('Col'),
    Divider: passthrough('Divider'),
    Form,
    Input,
    message: { error: vi.fn(), success: vi.fn() },
    Row: passthrough('Row'),
    Select: defineComponent({
      inheritAttrs: false,
      name: 'Select',
      props: {
        options: { default: () => [], type: Array },
        placeholder: { default: '', type: String },
        value: { default: '', type: String },
      },
      setup(props, { attrs }) {
        return () =>
          h('div', attrs, [
            props.value || props.placeholder,
            ...(props.options as Array<{ label: string }>).map(
              (option) => option.label,
            ),
          ]);
      },
    }),
    Switch: passthrough('Switch', 'button'),
  };
});

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/settings/model' }),
}));

vi.mock('../site-config/components/editor-shell.vue', () => ({
  default: {
    name: 'EditorShell',
    template:
      '<main><slot /><button data-testid="save-model" @click="$emit(\'save\')">保存</button></main>',
  },
}));

vi.mock('#/api', () => ({
  getModelConfigApi: vi.fn(),
  testChatModelApi: vi.fn(),
  updateModelConfigApi: vi.fn(),
}));

import {
  getModelConfigApi,
  testChatModelApi,
  updateModelConfigApi,
} from '#/api';
import { message } from 'ant-design-vue';

import ModelSettings from './model.vue';

function modelConfig(chat: Record<string, unknown> = {}) {
  return {
    admin: {
      apiBase: '',
      apiKeySet: false,
      groupId: '',
      model: '',
      provider: 'openai-compatible',
      timeoutSeconds: 30,
    },
    analysis: {
      apiBase: '',
      apiKeySet: false,
      groupId: '',
      model: 'MiniMax-M3',
    },
    assist: { enabled: true, systemPrompt: '' },
    chat: {
      apiBase: 'https://legacy.example.com',
      apiKeySet: true,
      model: 'abab6.5s-chat',
      ...chat,
    },
    dailyQuiz: {
      apiBase: '',
      apiKeySet: false,
      groupId: '',
      model: '',
      provider: '',
      timeoutSeconds: 30,
    },
    image: { apiBase: '', apiKeySet: false, model: '' },
    video: { apiBase: '', apiKeySet: false, model: '' },
  };
}

async function setChatTimeout(value: string) {
  const timeout = document.body.querySelector(
    '[data-testid="chat-model-section"] input[type="number"]',
  ) as HTMLInputElement;
  timeout.value = value;
  timeout.dispatchEvent(new Event('input', { bubbles: true }));
  await flushVuePromises();
}

describe('model settings compatible chat configuration', () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  beforeEach(() => {
    vi.mocked(getModelConfigApi).mockReset();
    vi.mocked(testChatModelApi).mockReset();
    vi.mocked(updateModelConfigApi).mockReset();
    vi.mocked(message.error).mockReset();
    vi.mocked(updateModelConfigApi).mockResolvedValue(modelConfig() as any);
    vi.mocked(testChatModelApi).mockResolvedValue({
      apiBase: '',
      latencyMs: 1,
      message: 'ok',
      model: '',
      ok: true,
    });
  });

  it('keeps a legacy chat provider unconfigured and offers only OpenAI or Anthropic', async () => {
    vi.mocked(getModelConfigApi).mockResolvedValue(modelConfig() as any);

    const wrapper = mountVueComponent(ModelSettings);
    await flushVuePromises();

    const chat = document.body.querySelector(
      '[data-testid="chat-model-section"]',
    );
    expect(chat).not.toBeNull();
    expect(chat?.textContent).toContain('对话模型尚未配置');
    expect(chat?.textContent).toContain('请选择协议');
    expect(chat?.textContent).toContain('OpenAI 协议');
    expect(chat?.textContent).toContain('Anthropic 协议');
    expect(chat?.textContent).not.toContain('Group ID');
    expect(chat?.textContent).not.toContain('MiniMax');

    const timeout = chat?.querySelector('input[type="number"]');
    expect(timeout?.getAttribute('min')).toBe('1');
    wrapper.unmount();
  });

  it('saves provider and timeout without a chat groupId', async () => {
    vi.mocked(getModelConfigApi).mockResolvedValue(
      modelConfig({
        provider: 'openai-compatible',
        timeoutSeconds: 45,
      }) as any,
    );
    vi.mocked(updateModelConfigApi).mockResolvedValue(
      modelConfig({
        provider: 'openai-compatible',
        timeoutSeconds: 45,
      }) as any,
    );

    const wrapper = mountVueComponent(ModelSettings);
    await flushVuePromises();

    wrapper.button('保存')?.click();
    await flushVuePromises();

    expect(updateModelConfigApi).toHaveBeenCalledOnce();
    const payload = vi.mocked(updateModelConfigApi).mock.calls[0]?.[0];
    expect(payload?.chat).toEqual(
      expect.objectContaining({
        provider: 'openai-compatible',
        timeoutSeconds: 45,
      }),
    );
    expect(payload?.chat).not.toHaveProperty('groupId');
    wrapper.unmount();
  });

  it('tests provider and timeout with a minimal request and no chat groupId', async () => {
    vi.mocked(getModelConfigApi).mockResolvedValue(
      modelConfig({
        provider: 'anthropic-compatible',
        timeoutSeconds: 60,
      }) as any,
    );

    const wrapper = mountVueComponent(ModelSettings);
    await flushVuePromises();

    await setChatTimeout('75');

    wrapper.button('测试连通性')?.click();
    await flushVuePromises();

    expect(testChatModelApi).toHaveBeenCalledOnce();
    const payload = vi.mocked(testChatModelApi).mock.calls[0]?.[0];
    expect(payload).toEqual(
      expect.objectContaining({
        provider: 'anthropic-compatible',
        timeoutSeconds: 75,
      }),
    );
    expect(payload).not.toHaveProperty('groupId');
    expect(wrapper.text()).toContain('最小探测请求');
    expect(wrapper.text()).not.toContain('不消耗生成额度');
    wrapper.unmount();
  });

  it.each([
    ['empty', ''],
    ['zero', '0'],
    ['negative', '-1'],
    ['fractional', '1.5'],
  ])('blocks save for an %s chat timeout', async (_label, value) => {
    vi.mocked(getModelConfigApi).mockResolvedValue(
      modelConfig({
        provider: 'openai-compatible',
        timeoutSeconds: 30,
      }) as any,
    );

    const wrapper = mountVueComponent(ModelSettings);
    await flushVuePromises();
    await setChatTimeout(value);

    wrapper.button('保存')?.click();
    await flushVuePromises();

    expect(updateModelConfigApi).not.toHaveBeenCalled();
    expect(message.error).toHaveBeenCalledWith(
      '对话模型超时时间必须是大于 0 的整数',
    );
    wrapper.unmount();
  });

  it.each([
    ['empty', ''],
    ['zero', '0'],
    ['negative', '-1'],
    ['fractional', '1.5'],
  ])('blocks connection test for an %s chat timeout', async (_label, value) => {
    vi.mocked(getModelConfigApi).mockResolvedValue(
      modelConfig({
        provider: 'anthropic-compatible',
        timeoutSeconds: 30,
      }) as any,
    );

    const wrapper = mountVueComponent(ModelSettings);
    await flushVuePromises();
    await setChatTimeout(value);

    wrapper.button('测试连通性')?.click();
    await flushVuePromises();

    expect(testChatModelApi).not.toHaveBeenCalled();
    expect(message.error).toHaveBeenCalledWith(
      '对话模型超时时间必须是大于 0 的整数',
    );
    wrapper.unmount();
  });
});
