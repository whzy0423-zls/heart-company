<script>
export default {
  onLaunch() {
    // 应用启动
  },
}
</script>

<style>
page {
  min-height: 100%;
  background:
    radial-gradient(circle at 18% -8%, rgba(90,160,255,.20) 0, rgba(90,160,255,0) 34%),
    radial-gradient(circle at 100% 4%, rgba(255,152,86,.16) 0, rgba(255,152,86,0) 30%),
    linear-gradient(180deg, #fbfcff 0%, #f6f8fc 48%, #eef3f8 100%);
  color: #12151b;
  font-family: -apple-system, BlinkMacSystemFont, "SF Pro Display", "SF Pro Text", "PingFang SC", "Microsoft YaHei", sans-serif;
  line-height: 1.62;
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
}
.wrap {
  width: 100%;
  max-width: 900rpx;
  margin: 0 auto;
  padding: calc(28rpx + constant(safe-area-inset-top)) 28rpx calc(72rpx + constant(safe-area-inset-bottom));
  padding: calc(28rpx + env(safe-area-inset-top)) 28rpx calc(72rpx + env(safe-area-inset-bottom));
  box-sizing: border-box;
}
.page-stack {
  display: flex;
  flex-direction: column;
  gap: 24rpx;
  padding-bottom: calc(24rpx + constant(safe-area-inset-bottom) + var(--window-bottom, 0px));
  padding-bottom: calc(24rpx + env(safe-area-inset-bottom) + var(--window-bottom, 0px));
}
.btn-primary,
.btn-ghost,
.btn-soft {
  min-height: 88rpx;
  padding: 0 30rpx;
  border-radius: 999rpx;
  border: none;
  font-size: 30rpx;
  font-weight: 800;
  letter-spacing: 0;
  line-height: 1.2;
  display: flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  transition: opacity .18s ease, transform .18s ease, box-shadow .18s ease;
  touch-action: manipulation;
}
.btn-primary {
  background: linear-gradient(135deg, #2563eb 0%, #3b82f6 54%, #5aa0ff 100%);
  color: #fff;
  box-shadow: 0 22rpx 48rpx -24rpx rgba(37, 99, 235, .72);
}
.btn-primary[disabled],
.btn-ghost[disabled],
.btn-soft[disabled] {
  opacity: .48;
  box-shadow: none;
}
.btn-primary:active,
.btn-ghost:active,
.btn-soft:active,
.card[role="button"]:active {
  opacity: .82;
  transform: scale(.985);
}
.btn-primary::after,
.btn-ghost::after,
.btn-soft::after,
button::after { border: none; }
.btn-ghost {
  background: rgba(255,255,255,.82);
  color: #1e293b;
  border: 2rpx solid rgba(15,23,42,.08);
  box-shadow: 0 16rpx 34rpx -28rpx rgba(15,23,42,.38);
}
.btn-soft {
  background: linear-gradient(120deg, rgba(37,99,235,.10), rgba(249,115,22,.10));
  color: #2563eb;
}
.card {
  position: relative;
  overflow: hidden;
  background: rgba(255,255,255,.76);
  border: 2rpx solid rgba(255,255,255,.92);
  border-radius: 32rpx;
  padding: 32rpx;
  box-shadow: 0 24rpx 60rpx -38rpx rgba(15,23,42,.45);
  box-sizing: border-box;
  backdrop-filter: blur(18rpx);
}
.card::before {
  content: "";
  position: absolute;
  left: 20rpx;
  right: 20rpx;
  top: 0;
  height: 2rpx;
  background: linear-gradient(90deg, transparent, rgba(255,255,255,.95), transparent);
}
.eyebrow {
  align-self: flex-start;
  display: inline-flex;
  min-height: 40rpx;
  align-items: center;
  padding: 6rpx 18rpx;
  border-radius: 999rpx;
  background: rgba(37,99,235,.10);
  border: 2rpx solid rgba(37,99,235,.12);
  color: #2563eb;
  font-size: 22rpx;
  font-weight: 900;
  letter-spacing: 3rpx;
}
.gradient-title {
  color: #0f172a;
  font-size: 52rpx;
  line-height: 1.14;
  font-weight: 900;
  letter-spacing: -.8rpx;
}
.muted { color: #64748b; }
.section-title,
.sec-title {
  display: block;
  color: #0f172a;
  font-size: 32rpx;
  font-weight: 900;
  line-height: 1.28;
  letter-spacing: -.2rpx;
}
.section-lead,
.sec-txt {
  display: block;
  color: #475569;
  font-size: 27rpx;
  line-height: 1.72;
}
.chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 52rpx;
  height: 52rpx;
  padding: 0 16rpx;
  border-radius: 17rpx;
  background: linear-gradient(135deg, #60a5fa, #2563eb);
  color: #fff;
  font-size: 24rpx;
  font-weight: 900;
  box-shadow: 0 14rpx 34rpx -18rpx rgba(37,99,235,.72);
  box-sizing: border-box;
}
.field-control {
  min-height: 88rpx;
  width: 100%;
  box-sizing: border-box;
  background: rgba(255,255,255,.82);
  border: 2rpx solid rgba(15,23,42,.08);
  border-radius: 22rpx;
  padding: 20rpx 24rpx;
  color: #0f172a;
  font-size: 28rpx;
}
.subtle-divider {
  height: 2rpx;
  background: rgba(15,23,42,.07);
}
button {
  line-height: 1.2;
  margin: 0;
}
input {
  line-height: 1.4;
}
@media (prefers-reduced-motion: reduce) {
  .btn-primary,
  .btn-ghost,
  .btn-soft {
    transition: none;
  }
}
</style>
