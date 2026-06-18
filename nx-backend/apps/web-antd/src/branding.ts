import { updatePreferences } from '@vben/preferences';

import { getAdminBrandingApi } from '#/api';

/** 启动屏读取的稳定 localStorage 键（与 loading.html 内联脚本约定一致）。 */
export const BRANDING_CACHE_KEY = 'nine-xing-admin-branding';

/**
 * 拉取后台品牌配置并应用：
 * - 运行时覆盖 preferences 的 app.name / logo.source（侧边栏、登录页、标题即时生效）
 * - 写入 localStorage，供下次启动屏 (loading.html) 渲染品牌
 *
 * 失败不阻塞启动（服务端不可用时退回构建期默认值）。
 */
export async function applyAdminBranding(): Promise<void> {
  try {
    const branding = await getAdminBrandingApi();
    if (!branding) return;

    const patch: Record<string, any> = {};
    if (branding.name) patch.app = { name: branding.name };
    if (branding.logo) patch.logo = { enable: true, source: branding.logo };
    if (Object.keys(patch).length > 0) {
      updatePreferences(patch);
    }

    localStorage.setItem(BRANDING_CACHE_KEY, JSON.stringify(branding));
  } catch {
    // 忽略：保持默认品牌
  }
}
