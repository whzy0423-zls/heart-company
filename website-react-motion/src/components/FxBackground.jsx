// 极光光晕背景（对应原 script.js 注入的 .fx-bg）
export default function FxBackground() {
  return (
    <div className="fx-bg" aria-hidden="true">
      <div className="orb o1"></div>
      <div className="orb o2"></div>
      <div className="orb o3"></div>
      <div className="orb o4"></div>
    </div>
  )
}
