import siteConfig from '../data/siteConfig'

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
        <div>{copyright}</div>
      </div>
    </footer>
  )
}
