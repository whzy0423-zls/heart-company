import siteConfig from '../data/siteConfig'

const ICP_FILING_URL = 'https://beian.miit.gov.cn/#/Integrated/recordQuery'

export default function Footer() {
  const copyright = siteConfig.site.copyright.replace(/\s*·\s*仅作展示用途/g, '')

  return (
    <footer className="footer">
      <div className="wrap footer__inner">
        <div>
          <div className="brand">
            <img className="logo" src={siteConfig.site.logo} alt="" style={{ width: 28, height: 28 }} />
            {siteConfig.site.brandName}
          </div>
          <p style={{ marginTop: 8 }}>{siteConfig.site.footerTagline}</p>
        </div>
        <div className="footer__legal">
          <div>{copyright}</div>
          <a href={ICP_FILING_URL} target="_blank" rel="noopener noreferrer">
            鲁ICP备2026051312号-1
          </a>
          <a href="/legal/privacy-policy.html">隐私政策</a>
          <a href="/legal/privacy-rights.html">隐私权利</a>
        </div>
      </div>
    </footer>
  )
}
