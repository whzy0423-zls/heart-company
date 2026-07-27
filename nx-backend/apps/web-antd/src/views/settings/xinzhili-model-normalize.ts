import type { XinzhiliModelConfigView } from '#/api';

type LegacyXinzhiliModelConfigView = Omit<
  XinzhiliModelConfigView,
  'enabledModes' | 'modePrompts'
> & {
  enabledModes: XinzhiliModelConfigView['enabledModes'] | null;
  modePrompts: XinzhiliModelConfigView['modePrompts'] | null;
};

export function normalizeXinzhiliModelConfigView(
  data: LegacyXinzhiliModelConfigView,
): XinzhiliModelConfigView {
  const enabledModes = Array.isArray(data.enabledModes)
    ? data.enabledModes
    : [];
  const modePrompts =
    data.modePrompts &&
    typeof data.modePrompts === 'object' &&
    !Array.isArray(data.modePrompts)
      ? data.modePrompts
      : {};

  return {
    ...data,
    enabledModes: enabledModes.includes('normal')
      ? [...enabledModes]
      : ['normal', ...enabledModes],
    modePrompts: { ...modePrompts },
  };
}
