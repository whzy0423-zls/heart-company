import { defineComponent, h } from 'vue';

function passthrough(name: string, tag = 'div') {
  return defineComponent({
    inheritAttrs: false,
    name,
    setup(_, { attrs, slots }) {
      return () => h(tag, attrs, slots.default?.());
    },
  });
}

export const Alert = defineComponent({
  inheritAttrs: false,
  name: 'Alert',
  props: {
    message: { default: '', type: String },
  },
  setup(props, { attrs, slots }) {
    return () =>
      h('div', { ...attrs, role: 'alert' }, [
        props.message,
        slots.default?.(),
        slots.action?.(),
      ]);
  },
});

export const Button = passthrough('Button', 'button');
export const Card = defineComponent({
  inheritAttrs: false,
  name: 'Card',
  setup(_, { attrs, slots }) {
    return () =>
      h('div', attrs, [
        slots.extra?.(),
        slots.default?.(),
      ]);
  },
});
export const Col = passthrough('Col');
export const Drawer = passthrough('Drawer');
export const Empty = Object.assign(passthrough('Empty'), {
  PRESENTED_IMAGE_SIMPLE: 'simple',
});

const DescriptionsRoot = passthrough('Descriptions') as any;
DescriptionsRoot.Item = passthrough('DescriptionsItem');
export const Descriptions = DescriptionsRoot;

const FormRoot = passthrough('Form') as any;
FormRoot.Item = passthrough('FormItem');
export const Form = FormRoot;

export const Input = passthrough('Input', 'input');
export const Modal = passthrough('Modal');
export const Row = passthrough('Row');
export const Select = passthrough('Select');
export const Space = passthrough('Space');
export const Statistic = defineComponent({
  name: 'Statistic',
  props: {
    title: { default: '', type: String },
    value: { default: 0, type: [Number, String] },
  },
  setup(props) {
    return () => h('div', [props.title, String(props.value)]);
  },
});
export const Table = defineComponent({
  name: 'Table',
  props: {
    columns: { default: () => [], type: Array },
    dataSource: { default: () => [], type: Array },
  },
  setup(props, { slots }) {
    return () =>
      h(
        'div',
        { class: 'mock-table' },
        (props.dataSource as Record<string, any>[]).flatMap((record) =>
          (props.columns as Record<string, any>[]).map((column) =>
            h(
              'div',
              { class: 'mock-cell' },
              [
                String(record[column.dataIndex] ?? ''),
                slots.bodyCell?.({ column, record }),
              ],
            ),
          ),
        ),
      );
  },
});

export const Tooltip = defineComponent({
  inheritAttrs: false,
  name: 'Tooltip',
  props: {
    overlayStyle: { default: () => ({}), type: Object },
    placement: { default: 'topLeft', type: String },
    title: { default: '', type: [Number, String] },
  },
  setup(props, { attrs, slots }) {
    return () =>
      h(
        'span',
        {
          ...attrs,
          class: ['tooltip-stub', attrs.class],
          'data-placement': props.placement,
          'data-title': String(props.title ?? ''),
        },
        slots.default?.(),
      );
  },
});

export const Tag = passthrough('Tag', 'span');
export const Textarea = passthrough('Textarea', 'textarea');

export const message = {
  error: () => undefined,
  success: () => undefined,
  warning: () => undefined,
};
