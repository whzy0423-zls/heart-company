<script setup lang="ts">
import ProfileBase from './base-setting.vue';
</script>

<template>
  <div class="profile-page">
    <div class="profile-page__header">
      <div>
        <p class="profile-page__eyebrow">账号资料</p>
        <h1>个人中心</h1>
      </div>
    </div>
    <ProfileBase />
  </div>
</template>

<style scoped>
.profile-page {
  min-height: calc(100vh - 96px);
  padding: 24px;
  background:
    linear-gradient(180deg, hsl(var(--accent) / 40%), transparent 220px),
    hsl(var(--background));
}

.profile-page__header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  max-width: 1120px;
  margin: 0 auto 18px;
}

.profile-page__eyebrow {
  margin: 0 0 6px;
  font-size: 12px;
  font-weight: 600;
  color: hsl(var(--muted-foreground));
  text-transform: uppercase;
}

.profile-page h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 650;
  line-height: 1.25;
  color: hsl(var(--foreground));
  letter-spacing: 0;
}

@media (max-width: 768px) {
  .profile-page {
    padding: 16px;
  }
}
</style>
