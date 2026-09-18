import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';
import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';
const state = vi.hoisted(() => ({ codes: ['Agent:Distribution:View'], get: vi.fn(), save: vi.fn(), qr: vi.fn() }));
vi.mock('@vben/stores', () => ({ useAccessStore: () => ({ accessCodes: state.codes, accessToken: 'token' }) }));
vi.mock('#/api/core/distribution-poster', () => ({ getPosterConfigApi: state.get, savePosterConfigApi: state.save }));
vi.mock('#/api/core/upload', () => ({ uploadFileApi: vi.fn() }));
vi.mock('#/utils/upload-asset-preview', () => ({ createUploadAssetObjectURL: async (s: string) => s }));
vi.mock('qrcode', () => ({ default: { toDataURL: state.qr } }));
vi.mock('ant-design-vue', async () => {
  const { defineComponent, h } = await import('vue');
  const Box = defineComponent({ setup: (_, { slots }) => () => h('div', [slots.title?.(), slots.extra?.(), slots.default?.()]) });
  const Input = defineComponent({ props: ['value'], emits: ['update:value'], setup: (p, { emit }) => () => h('input', { value: p.value, onInput: (e: Event) => emit('update:value', (e.target as HTMLInputElement).value) }) });
  return { Alert: defineComponent({ props: ['message'], setup: p => () => h('p', p.message) }),
    Button: defineComponent({ setup: (_, { slots }) => () => h('button', slots.default?.()) }),
    Card: Box, Form: Object.assign(Box, { Item: Box }), Input: Object.assign(Input, { TextArea: Input }),
    InputNumber: Input, Slider: Box, Space: Box, Upload: Box, Typography: { Text: Box, Paragraph: Box }, message: { success: vi.fn(), error: vi.fn() } };
});
import Composer from './distribution-poster-composer.vue';
async function settle() { for (let i = 0; i < 12; i++) await flushVuePromises(); }
function mount(editable = false) { return mountVueComponent(defineComponent({ setup: () => () => h(Composer, { editable, agentCode: 'A123' }) })); }
const config = { templateUrl: '/api/upload-assets/1', landingUrl: 'https://example.com/fixed', qrImageUrl: '', qrSize: 176, qrX: 62, qrY: 1010, inviteX: 286, inviteY: 1100, inviteWidth: 350, inviteFontSize: 26 };
describe('central poster configuration', () => {
  beforeEach(() => {
    state.codes = ['Agent:Distribution:View'];
    state.get.mockReset().mockResolvedValue(config);
    state.save.mockReset().mockResolvedValue({});
    state.qr.mockReset().mockResolvedValue('data:image/png;base64,AA==');
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ clearRect: vi.fn(), drawImage: vi.fn(), fillRect: vi.fn(), fillText: vi.fn(), beginPath: vi.fn(), roundRect: vi.fn(), fill: vi.fn() } as any);
    vi.stubGlobal('Image', class {
      naturalWidth = 720; naturalHeight = 1280; onload?: () => void;
      set src(_: string) { queueMicrotask(() => this.onload?.()); }
    });
  });
  it('agent only edits invitation and never changes the fixed QR', async () => {
    const wrapper = mount(true); await settle();
    expect(wrapper.text()).not.toContain('上传海报模板');
    expect(wrapper.text()).not.toContain('保存并发布');
    const inputs = document.body.querySelectorAll('input');
    expect(inputs).toHaveLength(1);
    expect(document.body.querySelector('[data-element]')).toBeNull();
    inputs[0]!.value = 'B456'; inputs[0]!.dispatchEvent(new Event('input', { bubbles: true }));
    await settle();
    expect(state.qr).toHaveBeenCalled();
    for (const call of state.qr.mock.calls) expect(call[0]).toBe(config.landingUrl);
    expect(state.save).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it('admin reloads and saves all shared fields without storing preview invitation', async () => {
    state.codes = ['Customer:App:List', 'Customer:App:Write'];
    const wrapper = mount(true); await settle();
    expect(wrapper.text()).toContain('上传海报模板');
    const target = document.body.querySelector('[data-element="qr"]') as HTMLElement;
    target.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', shiftKey: true, bubbles: true }));
    await settle();
    wrapper.button('保存并发布')!.click(); await settle();
    expect(state.save).toHaveBeenCalledWith({ ...config, qrX: 72 });
    wrapper.unmount();
  });
  it('fixed QR image takes precedence over URL', async () => {
    state.get.mockResolvedValue({ ...config, qrImageUrl: '/api/upload-assets/2' });
    const wrapper = mount(); await settle();
    expect(state.qr).not.toHaveBeenCalled();
    expect(wrapper.button('生成并下载 PNG')?.disabled).toBe(false);
    wrapper.unmount();
  });
  it('load failure disables export instead of inventing a QR', async () => {
    state.get.mockRejectedValue(new Error('offline'));
    const wrapper = mount(); await settle();
    expect(wrapper.text()).toContain('海报配置加载失败');
    expect(wrapper.button('生成并下载 PNG')?.disabled).toBe(true);
    expect(state.qr).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
