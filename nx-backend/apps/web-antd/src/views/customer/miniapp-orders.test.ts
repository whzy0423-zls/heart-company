/** @vitest-environment happy-dom */
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { defineComponent, h } from 'vue';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

const mocks = vi.hoisted(() => ({ list: vi.fn(), reconcile: vi.fn(), route: { query: {} as Record<string, string> } }));
vi.mock('vue-router', () => ({ useRoute: () => mocks.route }));

vi.mock('@vben/common-ui', () => ({ Page: { template: '<main><slot /></main>' } }));
vi.mock('@vben/icons', () => ({ IconifyIcon: { template: '<span />' } }));
vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  return {
    ...stubs,
    Avatar: { template: '<span><slot /></span>' },
    Select: defineComponent({
      inheritAttrs: false,
      props: { options: { default: () => [], type: Array }, value: { default: '', type: String } },
      emits: ['update:value'],
      setup(props, { attrs, emit }) {
        return () => h('select', {
          ...attrs,
          value: props.value,
          onChange: (event: Event) => emit('update:value', (event.target as HTMLSelectElement).value),
        }, (props.options as Array<{ label: string; value: string }>).map((option) =>
          h('option', { value: option.value }, option.label)));
      },
    }),
  };
});
vi.mock('#/api', () => ({ getMiniappOrderListApi: mocks.list, reconcileMiniappOrdersApi: mocks.reconcile }));

import MiniappOrders from './miniapp-orders.vue';

const courseOrder = {
  id: 93, outTradeNo: 'crs93-20261001', wxUserId: 42, phone: '', nickname: '',
  product: 'course_booking', title: '个人成长基础课', amount: 1999, status: 'paid',
  transactionId: 'wx-transaction', createTime: '2026/10/01 10:00:00', updateTime: '2026/10/01 10:01:00',
};

describe('miniapp course booking orders', () => {
  beforeEach(() => {
    mocks.list.mockReset();
    mocks.reconcile.mockReset();
    mocks.route.query = {};
    mocks.list.mockResolvedValue({ items: [courseOrder], total: 1, summary: { total: 1, paid: 1, pending: 0, paidAmount: 1999 } });
    mocks.reconcile.mockResolvedValue({ checked: 0, paid: 0, closed: 0, failed: 0 });
  });

  it('opens a payment notification with its order number filter', async () => {
    mocks.route.query.keyword = courseOrder.outTradeNo;
    const wrapper = mountVueComponent(MiniappOrders);
    await flushVuePromises();
    expect(mocks.list).toHaveBeenLastCalledWith(expect.objectContaining({ keyword: courseOrder.outTradeNo }));
    wrapper.unmount();
  });

  it('displays course title, amount, status, and WeChat user when contact data is absent', async () => {
    const wrapper = mountVueComponent(MiniappOrders);
    await flushVuePromises();
    expect(wrapper.text()).toContain('课程报名');
    expect(wrapper.text()).toContain('个人成长基础课');
    expect(wrapper.text()).toContain('¥19.99');
    expect(wrapper.text()).toContain('已支付');
    expect(wrapper.text()).toContain('微信用户');
    expect(wrapper.text()).toContain('用户 #42');
    document.querySelector<HTMLButtonElement>('button[aria-label="查看订单详情"]')?.click();
    await flushVuePromises();
    expect(wrapper.text()).toContain('微信用户 / #42');
    expect(wrapper.text()).not.toContain('undefined');
    wrapper.unmount();
  });

  it('requests course_booking orders and clears that filter on reset', async () => {
    const wrapper = mountVueComponent(MiniappOrders);
    await flushVuePromises();
    const select = document.querySelector<HTMLSelectElement>('select.filter-select');
    expect(select?.querySelector('option[value="course_booking"]')?.textContent).toBe('课程报名');
    select!.value = 'course_booking';
    select!.dispatchEvent(new Event('change', { bubbles: true }));
    await flushVuePromises();
    wrapper.button('查询')?.click();
    await flushVuePromises();
    expect(mocks.list).toHaveBeenLastCalledWith(expect.objectContaining({ product: 'course_booking', page: 1 }));
    wrapper.button('重置')?.click();
    await flushVuePromises();
    expect(mocks.list).toHaveBeenLastCalledWith(expect.objectContaining({ product: undefined, page: 1 }));
    wrapper.unmount();
  });
});
