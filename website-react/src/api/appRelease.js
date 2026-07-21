const DEFAULT_API_BASE = import.meta.env?.VITE_API_BASE_URL || '/api'
const LOAD_ERROR_MESSAGE = '暂时无法加载 App 版本信息，请稍后重试。'
const NETWORK_ERROR_MESSAGE = '网络连接失败，请稍后重试。'

export class AppReleaseAPIError extends Error {
  constructor(message, { status = 0, cause } = {}) {
    super(message)
    this.name = 'AppReleaseAPIError'
    this.status = status
    this.cause = cause
  }
}

function normalizeAPIBase(apiBase) {
  return String(apiBase || DEFAULT_API_BASE).replace(/\/+$/, '')
}

export function buildLatestAppReleaseDownloadURL(apiBase = DEFAULT_API_BASE) {
  return `${normalizeAPIBase(apiBase)}/public/app-release/download?platform=android`
}

export async function getLatestAppRelease({
  apiBase = DEFAULT_API_BASE,
  fetchImpl = globalThis.fetch,
  signal,
} = {}) {
  try {
    const response = await fetchImpl(
      `${normalizeAPIBase(apiBase)}/public/app-release/latest?platform=android`,
      {
        headers: { Accept: 'application/json' },
        signal,
      },
    )
    const body = await response.json().catch(() => ({}))

    if (!response.ok || body?.code !== 0) {
      throw new AppReleaseAPIError(LOAD_ERROR_MESSAGE, { status: response.status })
    }

    return body.data
  } catch (error) {
    if (error?.name === 'AbortError') throw error
    if (error instanceof AppReleaseAPIError) throw error
    throw new AppReleaseAPIError(NETWORK_ERROR_MESSAGE, { cause: error })
  }
}
