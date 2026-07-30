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

const { confirm } = vi.hoisted(() => ({ confirm: vi.fn() }));

vi.mock('ant-design-vue', () => {
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
  Input.Password = Input;

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

  return {
    Alert,
    Button: passthrough('Button', 'button'),
    Card: passthrough('Card'),
    Input,
    Modal: { confirm },
    Space: passthrough('Space'),
    Tag: passthrough('Tag'),
    message: { error: vi.fn(), success: vi.fn() },
  };
});

vi.mock('#/api', () => ({
  getBailianCredentialsApi: vi.fn(),
  updateBailianCredentialsApi: vi.fn(),
}));

import { getBailianCredentialsApi, updateBailianCredentialsApi } from '#/api';

import BailianCredentialsCard from './bailian-credentials-card.vue';

const sharedConfigured = {
  apiKeySet: true,
  apiKeySuffix: '…abcd',
  source: 'shared' as const,
  version: 4,
};

function inputValue(value: string) {
  const input = document.body.querySelector(
    '[data-testid="bailian-api-key-input"]',
  ) as HTMLInputElement;
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function mountCard() {
  const states: Array<Record<string, unknown>> = [];
  const Root = defineComponent({
    setup() {
      return () =>
        h(BailianCredentialsCard, {
          onStatusChange: (state: Record<string, unknown>) =>
            states.push(state),
        });
    },
  });
  const wrapper = mountVueComponent(Root);
  await flushVuePromises();
  return { ...wrapper, states };
}

describe('BailianCredentialsCard', () => {
  beforeEach(() => {
    vi.mocked(getBailianCredentialsApi).mockResolvedValue(sharedConfigured);
    vi.mocked(updateBailianCredentialsApi).mockResolvedValue({
      ...sharedConfigured,
      version: 5,
    });
  });

  afterEach(() => {
    document.body.innerHTML = '';
    vi.clearAllMocks();
  });

  it('loads only the safe credential view and emits its state without exposing a key', async () => {
    const wrapper = await mountCard();

    expect(getBailianCredentialsApi).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain('百炼公共 API Key');
    expect(wrapper.text()).toContain(
      '同一个 Key 用于 Paraformer 实时识别、Qwen 克隆音色和 Qwen TTS',
    );
    expect(wrapper.text()).toContain('已配置');
    expect(wrapper.text()).toContain('…abcd');
    expect(document.body.querySelectorAll('input')).toHaveLength(1);
    expect(document.body.textContent).not.toContain('sk-plain-secret');
    expect(wrapper.states.at(-1)).toMatchObject({
      apiKeySet: true,
      error: null,
      loading: false,
      source: 'shared',
      version: 4,
    });
    wrapper.unmount();
  });

  it('saves an empty input as a keep-existing request using the loaded version', async () => {
    const wrapper = await mountCard();
    (wrapper.button('保存') as HTMLButtonElement).click();
    await flushVuePromises();

    expect(updateBailianCredentialsApi).toHaveBeenCalledWith({
      apiKey: '',
      clearApiKey: false,
      expectedVersion: 4,
    });
    expect(wrapper.text()).toContain('版本 5');
    expect(
      (
        document.body.querySelector(
          '[data-testid="bailian-api-key-input"]',
        ) as HTMLInputElement
      ).value,
    ).toBe('');
    wrapper.unmount();
  });

  it('does not overwrite on a CAS conflict and lets the user reload the latest state', async () => {
    vi.mocked(updateBailianCredentialsApi).mockRejectedValue({
      response: { status: 409 },
    });
    const wrapper = await mountCard();
    inputValue('new-key');
    (wrapper.button('保存') as HTMLButtonElement).click();
    await flushVuePromises();

    expect(wrapper.text()).toContain('配置已更新，请重新加载后再保存');
    expect(updateBailianCredentialsApi).toHaveBeenCalledOnce();
    (wrapper.button('重新加载') as HTMLButtonElement).click();
    await flushVuePromises();
    expect(getBailianCredentialsApi).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });

  it('requires a second confirmation before explicitly clearing the shared key', async () => {
    const wrapper = await mountCard();
    (wrapper.button('清空 Key') as HTMLButtonElement).click();

    expect(confirm).toHaveBeenCalledOnce();
    expect(updateBailianCredentialsApi).not.toHaveBeenCalled();
    await confirm.mock.calls[0]?.[0].onOk();
    await flushVuePromises();

    expect(updateBailianCredentialsApi).toHaveBeenCalledWith({
      apiKey: '',
      clearApiKey: true,
      expectedVersion: 4,
    });
    wrapper.unmount();
  });

  it('shows a stable unavailable state instead of unconfigured when first load fails, then recovers on retry', async () => {
    vi.mocked(getBailianCredentialsApi)
      .mockRejectedValueOnce(new Error('network down'))
      .mockResolvedValueOnce({
        apiKeySet: false,
        apiKeySuffix: '',
        source: 'none',
        version: 0,
      });
    const wrapper = await mountCard();

    expect(wrapper.text()).toContain('百炼凭证读取失败，请重新加载');
    expect(wrapper.text()).not.toContain('未配置');
    (wrapper.button('重新加载') as HTMLButtonElement).click();
    await flushVuePromises();
    expect(wrapper.text()).toContain('未配置');
    wrapper.unmount();
  });
});
