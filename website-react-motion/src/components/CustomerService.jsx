import { useEffect, useId, useState } from 'react'
import siteConfig from '../data/siteConfig'

const CUSTOMER_SERVICE_TITLE = '芯之力 小助手'
const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

export default function CustomerService() {
  const [open, setOpen] = useState(false)
  const titleId = useId()
  const qr = siteConfig?.site?.customerServiceQr?.trim()
  const qrSrc = qr ? `${API_BASE}/public/customer-service-qr` : ''

  useEffect(() => {
    if (!open) return undefined
    const onKeyDown = (event) => {
      if (event.key === 'Escape') setOpen(false)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open])

  if (!qr) return null

  return (
    <>
      <button
        className="customer-service-fab"
        type="button"
        aria-label="联系客服"
        onClick={() => setOpen(true)}
      >
        <span className="customer-service-fab__icon" aria-hidden="true">✦</span>
        <span>联系客服</span>
      </button>

      {open && (
        <div
          className="customer-service-modal"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setOpen(false)
          }}
        >
          <section
            className="customer-service-modal__card"
            role="dialog"
            aria-modal="true"
            aria-labelledby={titleId}
          >
            <button
              className="customer-service-modal__close"
              type="button"
              aria-label="关闭客服二维码弹窗"
              onClick={() => setOpen(false)}
            >
              ×
            </button>
            <p className="eyebrow">在线咨询</p>
            <h2 id={titleId}>{CUSTOMER_SERVICE_TITLE}</h2>
            <p className="customer-service-modal__lead">
              扫码添加小助手，获取课程咨询与成长支持。
            </p>
            <div className="customer-service-modal__qr">
              <img src={qrSrc} alt="芯之力 小助手二维码" />
            </div>
          </section>
        </div>
      )}
    </>
  )
}
