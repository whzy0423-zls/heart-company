interface FallbackProps {
  /**
   * 描述
   */
  description?: string;
  /**
   *  @zh_CN 首页路由地址
   *  @default /
   */
  homePath?: string;
  /**
   * @zh_CN 默认显示的图片
   * @default pageNotFoundSvg
   */
  image?: string;
  /**
   *  @zh_CN 内置类型
   */
  status?: '403' | '404' | '500' | 'coming-soon' | 'offline';
  /**
   *  @zh_CN 页面提示语
   */
  title?: string;
}

function resolveFallbackRetryPath(
  redirect: null | string | (null | string)[] | undefined,
  homePath = '/',
) {
  const rawRedirect = Array.isArray(redirect) ? redirect[0] : redirect;
  if (!rawRedirect) {
    return homePath;
  }

  try {
    const decoded = decodeURIComponent(rawRedirect);
    if (decoded.startsWith('/') && !decoded.startsWith('//')) {
      return decoded;
    }
  } catch {
    // Use homePath when the redirect query is malformed.
  }

  return homePath;
}

export { resolveFallbackRetryPath };
export type { FallbackProps };
