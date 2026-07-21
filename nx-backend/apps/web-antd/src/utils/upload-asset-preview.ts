import type { MaybeRefOrGetter } from 'vue';

import { computed, onScopeDispose, onUnmounted, ref, toValue, watch } from 'vue';

import { isSafePreviewURL } from './safe-preview-url';

export const UPLOAD_ASSET_PREVIEW_CACHE_LIMIT = 24;
export const UPLOAD_ASSET_PREVIEW_CONCURRENCY_LIMIT = 4;

export function normalizeUploadAssetSource(source?: string) {
  return source?.trim() || '';
}

export function isProtectedUploadAssetSource(source?: string) {
  const cleanSource = normalizeUploadAssetSource(source);
  if (!cleanSource) return false;
  if (isProtectedUploadAssetPath(cleanSource)) return true;
  if (typeof window === 'undefined') return false;

  try {
    const url = new URL(cleanSource, window.location.origin);
    return (
      url.origin === window.location.origin &&
      isProtectedUploadAssetPath(url.pathname)
    );
  } catch {
    return false;
  }
}

function isProtectedUploadAssetPath(pathname: string) {
  return (
    pathname.startsWith('/api/app-release-icons/') ||
    pathname.startsWith('/api/upload-assets/') ||
    pathname.startsWith('/api/uploads/')
  );
}

export function withUploadAssetPreviewToken(
  source?: string,
  _token?: null | string,
) {
  return normalizeUploadAssetSource(source);
}

export async function createUploadAssetObjectURL(
  source: string,
  token?: null | string,
  signal?: AbortSignal,
) {
  const cleanSource = normalizeUploadAssetSource(source);
  if (!isProtectedUploadAssetSource(cleanSource)) {
    return isSafePreviewURL(cleanSource) ? cleanSource : '';
  }
  if (!token) {
    return '';
  }

  const init: RequestInit = {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  };
  if (signal) {
    init.signal = signal;
  }

  const response = await fetch(cleanSource, init);
  if (!response.ok) {
    throw new Error(`Preview asset request failed: ${response.status}`);
  }
  return URL.createObjectURL(await response.blob());
}

export function useUploadAssetPreviewUrl(
  source: MaybeRefOrGetter<string | undefined>,
  token: MaybeRefOrGetter<null | string | undefined>,
) {
  const previewUrl = ref('');
  let currentObjectURL = '';
  let requestID = 0;

  function revokeCurrentObjectURL() {
    if (currentObjectURL) {
      URL.revokeObjectURL(currentObjectURL);
      currentObjectURL = '';
    }
  }

  watch(
    () => [normalizeUploadAssetSource(toValue(source)), toValue(token)] as const,
    async ([nextSource, nextToken]) => {
      const id = ++requestID;
      revokeCurrentObjectURL();
      if (!nextSource) {
        previewUrl.value = '';
        return;
      }
      if (!isProtectedUploadAssetSource(nextSource)) {
        previewUrl.value = isSafePreviewURL(nextSource) ? nextSource : '';
        return;
      }
      previewUrl.value = '';
      try {
        const objectURL = await createUploadAssetObjectURL(
          nextSource,
          nextToken,
        );
        if (id !== requestID) {
          if (objectURL.startsWith('blob:')) {
            URL.revokeObjectURL(objectURL);
          }
          return;
        }
        currentObjectURL = objectURL.startsWith('blob:') ? objectURL : '';
        previewUrl.value = objectURL;
      } catch {
        if (id === requestID) {
          previewUrl.value = '';
        }
      }
    },
    { immediate: true },
  );

  onUnmounted(() => {
    requestID++;
    revokeCurrentObjectURL();
  });

  return computed(() => previewUrl.value);
}

export function useUploadAssetPreviewResolver(
  token: MaybeRefOrGetter<null | string | undefined>,
) {
  const version = ref(0);
  const cache = new Map<string, string>();
  const pending = new Set<string>();
  const queue: Array<{
    cleanSource: string;
    key: string;
    requestEpoch: number;
    tokenValue: string;
  }> = [];
  const activeRequests = new Map<string, AbortController>();
  let cacheEpoch = 0;

  function revokeObjectURL(objectURL: string) {
    if (objectURL.startsWith('blob:')) {
      URL.revokeObjectURL(objectURL);
    }
  }

  function rememberCachedPreview(key: string, objectURL: string) {
    const existing = cache.get(key);
    if (existing) {
      revokeObjectURL(existing);
      cache.delete(key);
    }
    cache.set(key, objectURL);

    while (cache.size > UPLOAD_ASSET_PREVIEW_CACHE_LIMIT) {
      const oldest = cache.keys().next();
      if (oldest.done) break;
      const oldestKey = oldest.value;
      const oldestURL = cache.get(oldestKey);
      cache.delete(oldestKey);
      if (oldestURL) {
        revokeObjectURL(oldestURL);
      }
    }
  }

  function revokeAll() {
    cacheEpoch++;
    for (const controller of activeRequests.values()) {
      controller.abort();
    }
    activeRequests.clear();
    queue.length = 0;
    for (const objectURL of cache.values()) {
      revokeObjectURL(objectURL);
    }
    cache.clear();
    pending.clear();
    version.value++;
  }

  function drainQueue() {
    while (
      activeRequests.size < UPLOAD_ASSET_PREVIEW_CONCURRENCY_LIMIT &&
      queue.length > 0
    ) {
      const task = queue.shift();
      if (!task) break;
      if (!pending.has(task.key) || task.requestEpoch !== cacheEpoch) {
        pending.delete(task.key);
        continue;
      }

      const controller = new AbortController();
      activeRequests.set(task.key, controller);
      createUploadAssetObjectURL(
        task.cleanSource,
        task.tokenValue,
        controller.signal,
      )
        .then((objectURL) => {
          if (
            task.requestEpoch !== cacheEpoch ||
            controller.signal.aborted ||
            !pending.has(task.key)
          ) {
            revokeObjectURL(objectURL);
            return;
          }
          rememberCachedPreview(task.key, objectURL);
        })
        .catch(() => {
          cache.delete(task.key);
        })
        .finally(() => {
          activeRequests.delete(task.key);
          pending.delete(task.key);
          version.value++;
          drainQueue();
        });
    }
  }

  function resolve(source?: string) {
    version.value;
    const cleanSource = normalizeUploadAssetSource(source);
    if (!cleanSource) return '';
    if (!isProtectedUploadAssetSource(cleanSource)) {
      return isSafePreviewURL(cleanSource) ? cleanSource : '';
    }

    const tokenValue = toValue(token);
    if (!tokenValue) return '';

    const key = `${cleanSource}\n${tokenValue}`;
    const cached = cache.get(key);
    if (cached) {
      cache.delete(key);
      cache.set(key, cached);
      return cached;
    }
    if (!pending.has(key)) {
      const requestEpoch = cacheEpoch;
      pending.add(key);
      queue.push({ cleanSource, key, requestEpoch, tokenValue });
      drainQueue();
    }
    return '';
  }

  watch(() => toValue(token), revokeAll);
  onScopeDispose(revokeAll);

  return { resolve, revokeAll };
}
