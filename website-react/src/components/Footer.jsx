import siteConfig from '../data/siteConfig'

export default function Footer() {
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
        <div>{siteConfig.site.copyright}</div>
      </div>
    </footer>
  )
}
