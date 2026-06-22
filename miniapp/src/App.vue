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
  background: linear-gradient(180deg, #fbfcff 0%, #f7f8fb 46%, #eef3f8 100%);
  color: #12151b;
  font-family: -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif;
  line-height: 1.7;
}
.wrap {
  padding: 32rpx;
  box-sizing: border-box;
}
.page-stack {
  display: flex;
  flex-direction: column;
  gap: 24rpx;
  padding-bottom: 64rpx;
}
.btn-primary {
  min-height: 92rpx;
  background: linear-gradient(120deg, #2b7fff 0%, #5aa0ff 100%);
  color: #fff;
  border-radius: 999rpx;
  font-size: 31rpx;
  font-weight: 800;
  border: none;
  box-shadow: 0 18rpx 42rpx -22rpx rgba(43, 127, 255, .8);
  letter-spacing: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}
.btn-primary[disabled] {
  opacity: .58;
  box-shadow: none;
}
.btn-primary::after { border: none; }
.btn-ghost {
  min-height: 88rpx;
  background: rgba(255,255,255,.72);
  color: #12151b;
  border: 2rpx solid rgba(255,255,255,.92);
  border-radius: 999rpx;
  font-size: 29rpx;
  font-weight: 700;
  box-shadow: 0 12rpx 32rpx -22rpx rgba(28,40,70,.45);
  display: flex;
  align-items: center;
  justify-content: center;
}
.btn-ghost::after { border: none; }
.card {
  position: relative;
  overflow: hidden;
  background: rgba(255,255,255,.72);
  border: 2rpx solid rgba(255,255,255,.9);
  border-radius: 30rpx;
  padding: 32rpx;
  box-shadow: 0 22rpx 56rpx -34rpx rgba(28,40,70,.46);
  box-sizing: border-box;
}
.card::before {
  content: "";
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  height: 2rpx;
  background: linear-gradient(90deg, transparent, rgba(255,255,255,.95), transparent);
}
.eyebrow {
  align-self: flex-start;
  display: inline-flex;
  padding: 8rpx 20rpx;
  border-radius: 999rpx;
  background: rgba(43,127,255,.1);
  border: 2rpx solid rgba(43,127,255,.14);
  color: #2b7fff;
  font-size: 22rpx;
  font-weight: 900;
  letter-spacing: 3rpx;
}
.gradient-title {
  color: #12151b;
  font-size: 52rpx;
  line-height: 1.16;
  font-weight: 900;
  letter-spacing: 0;
}
.muted {
  color: #767d89;
}
.section-title {
  display: block;
  color: #12151b;
  font-size: 34rpx;
  font-weight: 900;
  line-height: 1.32;
  letter-spacing: 0;
}
.section-lead {
  display: block;
  color: #3c424d;
  font-size: 26rpx;
  line-height: 1.7;
}
.chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 52rpx;
  height: 52rpx;
  padding: 0 16rpx;
  border-radius: 16rpx;
  background: linear-gradient(135deg, #5aa0ff, #2b7fff);
  color: #fff;
  font-size: 24rpx;
  font-weight: 900;
  box-shadow: 0 14rpx 34rpx -18rpx rgba(43,127,255,.7);
  box-sizing: border-box;
}
.field-control {
  min-height: 92rpx;
  width: 100%;
  box-sizing: border-box;
  background: rgba(255,255,255,.72);
  border: 2rpx solid rgba(20,24,32,.08);
  border-radius: 20rpx;
  padding: 22rpx 24rpx;
  color: #12151b;
  font-size: 28rpx;
}
.subtle-divider {
  height: 2rpx;
  background: rgba(20,24,32,.07);
}
button {
  line-height: 1.2;
}
</style>
