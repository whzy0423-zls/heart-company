import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent } from 'vue';
import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

const mocks = vi.hoisted(() => ({
  accessCodes: [] as string[],
  uploadTasks: vi.fn(),
}));
vi.mock('@vben/common-ui', () => ({
  Page: defineComponent({ template: '<main><slot /></main>' }),
}));
vi.mock('@vben/stores', () => ({
  useAccessStore: () => ({
    get accessCodes() {
      return mocks.accessCodes;
    },
  }),
}));
vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  const simple = (name: string) =>
    defineComponent({ name, template: '<div><slot /></div>' });
  const Radio = Object.assign(simple('Radio'), {
    Button: simple('RadioButton'),
    Group: simple('RadioGroup'),
  });
  const Input = Object.assign(stubs.Input, {
    TextArea: simple('InputTextArea'),
  });
  return {
    ...stubs,
    Checkbox: simple('Checkbox'),
    Input,
    InputNumber: simple('InputNumber'),
    Progress: simple('Progress'),
    Radio,
    Tabs: defineComponent({
      props: ['items'],
      template:
        '<nav><span v-for="item in items" :key="item.key">{{ item.label }}</span></nav>',
    }),
  };
});
vi.mock('#/api/core/classroom', () => ({
  abortClassroomUploadApi: vi.fn(),
  completeClassroomUploadApi: vi.fn(),
  createClassroomContentApi: vi.fn(),
  createClassroomSeriesApi: vi.fn(),
  deleteClassroomContentApi: vi.fn(),
  deleteClassroomSeriesApi: vi.fn(),
  getClassroomContentsApi: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  getClassroomSeriesApi: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  getClassroomUploadTasksApi: mocks.uploadTasks,
  initiateClassroomUploadApi: vi.fn(),
  offlineClassroomContentApi: vi.fn(),
  offlineClassroomSeriesApi: vi.fn(),
  publishClassroomContentApi: vi.fn(),
  publishClassroomSeriesApi: vi.fn(),
  setClassroomContentPlaybackBlockedApi: vi.fn(),
  setClassroomContentPriceApi: vi.fn(),
  setClassroomSeriesPlaybackBlockedApi: vi.fn(),
  setClassroomSeriesPriceApi: vi.fn(),
  signClassroomUploadPartApi: vi.fn(),
  updateClassroomContentApi: vi.fn(),
  updateClassroomSeriesApi: vi.fn(),
}));

import ClassroomIndex from './index.vue';
import { getClassroomContentsApi } from '#/api/core/classroom';

describe('classroom management permission integration', () => {
  beforeEach(() => {
    mocks.accessCodes = ['Miniapp:Classroom:List'];
    mocks.uploadTasks.mockReset();
    vi.mocked(getClassroomContentsApi).mockResolvedValue({
      items: [],
      page: 1,
      pageSize: 50,
      total: 0,
    });
  });

  it('does not mount or request upload tasks without Upload permission', async () => {
    const wrapper = mountVueComponent(ClassroomIndex);
    await flushVuePromises();
    expect(wrapper.text()).not.toContain('上传任务');
    expect(mocks.uploadTasks).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
