const blockedRawContentTags = new Set([
  'iframe',
  'math',
  'object',
  'script',
  'style',
  'svg',
]);

const allowedTags = new Set([
  'a',
  'blockquote',
  'br',
  'code',
  'del',
  'em',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'hr',
  'img',
  'li',
  'ol',
  'p',
  'pre',
  's',
  'span',
  'strong',
  'u',
  'ul',
]);

const allowedStyleProperties = new Set([
  'background-color',
  'color',
  'text-align',
]);

const controlChars = /[\u0000-\u001F\u007F]/;

function currentBaseURL() {
  if (typeof window === 'undefined') {
    return 'https://localhost/';
  }
  return window.location.href;
}

function isSafeURL(raw: string, protocols: Set<string>) {
  const value = raw.trim();
  if (!value || controlChars.test(value) || value.startsWith('//')) return false;

  try {
    const url = new URL(value, currentBaseURL());
    return protocols.has(url.protocol);
  } catch {
    return false;
  }
}

function isSafeStyleValue(property: string, raw: string) {
  const value = raw.trim();
  const lower = value.toLowerCase();
  if (!value || lower.includes('expression') || lower.includes('url(')) {
    return false;
  }
  if (property === 'text-align') {
    return ['center', 'end', 'justify', 'left', 'right', 'start'].includes(lower);
  }
  return (
    /^#[\da-f]{3,8}$/i.test(value) ||
    /^[a-z]+$/i.test(value) ||
    /^(rgb|rgba|hsl|hsla)\([%,.\d\s-]+\)$/i.test(value) ||
    /^var\(--[\w-]+\)$/i.test(value)
  );
}

function sanitizeStyle(raw: string) {
  const declarations = raw
    .split(';')
    .map((item) => item.trim())
    .filter(Boolean);
  const safe = declarations.flatMap((item) => {
    const [propertyRaw, ...valueParts] = item.split(':');
    const property = propertyRaw?.trim().toLowerCase();
    const value = valueParts.join(':').trim();
    if (
      !property ||
      !allowedStyleProperties.has(property) ||
      !isSafeStyleValue(property, value)
    ) {
      return [];
    }
    return [`${property}: ${value}`];
  });
  return safe.length > 0 ? `${safe.join('; ')};` : '';
}

function appendSanitizedChildren(source: Element, target: Element | DocumentFragment) {
  for (const child of source.childNodes) {
    target.append(sanitizeNode(child));
  }
}

function sanitizeElement(source: Element): Node {
  const tagName = source.tagName.toLowerCase();
  if (blockedRawContentTags.has(tagName)) {
    return document.createDocumentFragment();
  }
  if (!allowedTags.has(tagName)) {
    const fragment = document.createDocumentFragment();
    appendSanitizedChildren(source, fragment);
    return fragment;
  }

  const target = document.createElement(tagName);
  const style = source.getAttribute('style');
  if (style) {
    const safeStyle = sanitizeStyle(style);
    if (safeStyle) target.setAttribute('style', safeStyle);
  }

  if (tagName === 'a') {
    const href = source.getAttribute('href');
    if (href && isSafeURL(href, new Set(['http:', 'https:', 'mailto:', 'tel:']))) {
      target.setAttribute('href', href.trim());
    }
    const title = source.getAttribute('title');
    if (title) target.setAttribute('title', title);
    const targetValue = source.getAttribute('target');
    if (targetValue === '_blank') {
      target.setAttribute('target', '_blank');
      const rel = source.getAttribute('rel');
      const relParts = new Set(
        `${rel || ''} noopener noreferrer`
          .split(/\s+/)
          .map((item) => item.trim())
          .filter(Boolean),
      );
      target.setAttribute('rel', [...relParts].join(' '));
    }
  }

  if (tagName === 'img') {
    const src = source.getAttribute('src');
    if (src && isSafeURL(src, new Set(['blob:', 'http:', 'https:']))) {
      target.setAttribute('src', src.trim());
    }
    for (const attr of ['alt', 'title']) {
      const value = source.getAttribute(attr);
      if (value) target.setAttribute(attr, value);
    }
    for (const attr of ['width', 'height']) {
      const value = source.getAttribute(attr);
      if (value && /^\d{1,5}$/.test(value.trim())) {
        target.setAttribute(attr, value.trim());
      }
    }
  }

  appendSanitizedChildren(source, target);
  return target;
}

function sanitizeNode(node: Node): Node {
  if (node.nodeType === Node.TEXT_NODE) {
    return document.createTextNode(node.textContent || '');
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return document.createDocumentFragment();
  }
  return sanitizeElement(node as Element);
}

export function sanitizeTipTapHTML(content = '') {
  if (typeof document === 'undefined') return '';

  const template = document.createElement('template');
  template.innerHTML = content;
  const fragment = document.createDocumentFragment();
  for (const child of template.content.childNodes) {
    fragment.append(sanitizeNode(child));
  }
  const container = document.createElement('div');
  container.append(fragment);
  return container.innerHTML;
}
