interface OpenWindowOptions {
  noopener?: boolean;
  noreferrer?: boolean;
  target?: '_blank' | '_parent' | '_self' | '_top' | string;
}

const CONTROL_RE = /[\u0000-\u001F\u007F]/;
const SAFE_WINDOW_PROTOCOLS = new Set(['blob:', 'http:', 'https:']);

function isSafeWindowURL(value: string): boolean {
  const source = value.trim();
  if (!source || CONTROL_RE.test(source) || source.startsWith('//')) {
    return false;
  }
  try {
    const base = typeof location === 'undefined' ? 'https://window.local' : location.href;
    const url = new URL(source, base);
    return SAFE_WINDOW_PROTOCOLS.has(url.protocol);
  } catch {
    return false;
  }
}

/**
 * 新窗口打开URL。
 *
 * @param url - 需要打开的网址。
 * @param options - 打开窗口的选项。
 */
function openWindow(url: string, options: OpenWindowOptions = {}): void {
  if (!isSafeWindowURL(url)) {
    return;
  }
  // 解构并设置默认值
  const { noopener = true, noreferrer = true, target = '_blank' } = options;

  // 基于选项创建特性字符串
  const features = [noopener && 'noopener=yes', noreferrer && 'noreferrer=yes']
    .filter(Boolean)
    .join(',');

  // 打开窗口
  window.open(url, target, features);
}

/**
 * 在新窗口中打开路由。
 * @param path
 */
function openRouteInNewWindow(path: string) {
  const { hash, origin } = location;
  const fullPath = path.startsWith('/') ? path : `/${path}`;
  const url = `${origin}${hash && !fullPath.startsWith('/#') ? '/#' : ''}${fullPath}`;
  openWindow(url, { target: '_blank' });
}

export { isSafeWindowURL, openRouteInNewWindow, openWindow };
