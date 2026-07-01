<script lang="ts" setup>
import type { NotificationItem } from '@vben/layouts';

import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';

import { AuthenticationLoginExpiredModal } from '@vben/common-ui';
import { useWatermark } from '@vben/hooks';
import {
  BasicLayout,
  LockScreen,
  Notification,
  UserDropdown,
} from '@vben/layouts';
import { preferences, usePreferences } from '@vben/preferences';
import { useAccessStore, useUserStore } from '@vben/stores';

import { notification } from 'ant-design-vue';

import { getSignupLeadListApi } from '#/api';
import { $t } from '#/locales';
import { useAuthStore } from '#/store';
import LoginForm from '#/views/_core/authentication/login.vue';

import { toSignupNotification } from './signup-notice';

const notifications = ref<NotificationItem[]>([]);
const SIGNUP_NOTICE_LAST_ID_KEY_PREFIX = 'nx-signup-notice-last-id:v2';
let signupNoticeTimer: number | undefined;
let signupEventSource: EventSource | undefined;
let signupEventUnavailable = false;
let signupEventRetryTimer: number | undefined;
let signupNoticeBootstrapped = false;
const seenSignupNoticeIds = new Set<string>();

const router = useRouter();
const userStore = useUserStore();
const authStore = useAuthStore();
const accessStore = useAccessStore();
const { destroyWatermark, updateWatermark } = useWatermark();
const { isDark } = usePreferences();
const showDot = computed(() =>
  notifications.value.some((item) => !item.isRead),
);

const menus = computed(() => [
  {
    handler: () => {
      router.push({ name: 'Profile' });
    },
    icon: 'lucide:user',
    text: $t('page.auth.profile'),
  },
]);

const avatar = computed(() => {
  return userStore.userInfo?.avatar ?? preferences.app.defaultAvatar;
});

async function handleLogout() {
  await authStore.logout(false);
}

function handleNoticeClear() {
  notifications.value = [];
}

function markRead(id: number | string) {
  const item = notifications.value.find((item) => item.id === id);
  if (item) {
    item.isRead = true;
  }
}

function remove(id: number | string) {
  notifications.value = notifications.value.filter((item) => item.id !== id);
}

function handleMakeAll() {
  notifications.value.forEach((item) => (item.isRead = true));
}

const viewAll = () => {
  router.push('/message/management');
};

const handleClick = (item: NotificationItem) => {
  // 如果通知项有链接，点击时跳转
  if (item.link) {
    navigateTo(item.link, item.query, item.state);
  }
};

function navigateTo(
  link: string,
  query?: Record<string, any>,
  state?: Record<string, any>,
) {
  if (link.startsWith('http://') || link.startsWith('https://')) {
    // 外部链接，在新标签页打开
    window.open(link, '_blank');
  } else {
    // 内部路由链接，支持 query 参数和 state
    router.push({
      path: link,
      query: query || {},
      state,
    });
  }
}

watch(
  () => ({
    enable: preferences.app.watermark,
    content: preferences.app.watermarkContent,
    isDark: isDark.value,
  }),
  async ({ enable, content, isDark: isDarkValue }) => {
    if (enable) {
      const watermarkColor = isDarkValue
        ? 'rgba(255, 255, 255, 0.12)'
        : 'rgba(0, 0, 0, 0.12)';

      await updateWatermark({
        advancedStyle: {
          colorStops: [
            {
              color: watermarkColor,
              offset: 0,
            },
            {
              color: watermarkColor,
              offset: 1,
            },
          ],
          type: 'linear',
        },
        content:
          content ||
          `${userStore.userInfo?.username} - ${userStore.userInfo?.realName}`,
      });
    } else {
      destroyWatermark();
    }
  },
  {
    immediate: true,
  },
);

function signupNoticeStorageKey() {
  const username = userStore.userInfo?.username || 'anonymous';
  return `${SIGNUP_NOTICE_LAST_ID_KEY_PREFIX}:${window.location.origin}:${username}`;
}

function rememberSignupNoticeId(id: string) {
  localStorage.setItem(signupNoticeStorageKey(), id);
}

function readLastSignupNoticeId() {
  return Number(localStorage.getItem(signupNoticeStorageKey()) || '0');
}

function pushSignupNotice(notice: NotificationItem) {
  const duplicated = notifications.value.some((item) => item.id === notice.id);
  if (!duplicated) {
    notifications.value.unshift(notice);
  }
  if (notice.id) {
    seenSignupNoticeIds.add(String(notice.id));
  }
  notification.info({
    description: notice.message,
    message: notice.title,
    onClick: () => navigateTo('/message/management', { type: 'signup' }),
    placement: 'topRight',
  });
}

async function pollSignupNotices() {
  if (!accessStore.accessToken) return;
  try {
    const result = await getSignupLeadListApi({ page: 1, pageSize: 5 });
    const items = result.items ?? [];
    if (items.length === 0) return;

    const latestId = Math.max(...items.map((item) => Number(item.id) || 0));
    if (!signupNoticeBootstrapped) {
      items.forEach((item) => seenSignupNoticeIds.add(`signup-${item.id}`));
      rememberSignupNoticeId(String(latestId));
      signupNoticeBootstrapped = true;
      return;
    }

    const notices = items
      .filter((item) => !seenSignupNoticeIds.has(`signup-${item.id}`))
      .toSorted((a, b) => Number(a.id) - Number(b.id))
      .map((item) => toSignupNotification(item));
    if (notices.length === 0) return;
    for (const notice of notices) {
      pushSignupNotice(notice);
    }
    rememberSignupNoticeId(String(latestId));
  } catch (error: any) {
    const status = error?.response?.status;
    if (status === 401) {
      notification.warning({
        description:
          '当前登录状态和本地 Go 后端不一致，请重新登录后台后再测试报名推送。',
        message: '报名通知连接已失效',
        placement: 'topRight',
      });
      accessStore.setAccessToken(null);
      await authStore.logout();
    }
    // 轮询通知失败不打断主界面。
  }
}

function connectSignupEvents() {
  if (!accessStore.accessToken || signupEventSource || signupEventUnavailable)
    return;

  const url = `/api/signups/events?token=${encodeURIComponent(accessStore.accessToken)}`;
  signupEventSource = new EventSource(url);
  signupEventSource.addEventListener('signup', (event) => {
    try {
      const lead = JSON.parse((event as MessageEvent).data);
      const notice = toSignupNotification(lead);
      pushSignupNotice(notice);
      if (lead?.id) {
        const latestId = Math.max(
          readLastSignupNoticeId(),
          Number(lead.id) || 0,
        );
        rememberSignupNoticeId(String(latestId));
      }
    } catch {
      // 忽略单条通知格式异常，保持连接继续。
    }
  });
  signupEventSource.addEventListener('error', () => {
    signupEventSource?.close();
    signupEventSource = undefined;
    signupEventUnavailable = true;
    if (signupEventRetryTimer) {
      window.clearTimeout(signupEventRetryTimer);
    }
    signupEventRetryTimer = window.setTimeout(() => {
      signupEventUnavailable = false;
      signupEventRetryTimer = undefined;
      connectSignupEvents();
    }, 15_000);
  });
}

function refreshSignupNotices() {
  pollSignupNotices();
  connectSignupEvents();
}

onMounted(() => {
  refreshSignupNotices();
  signupNoticeTimer = window.setInterval(() => {
    pollSignupNotices();
  }, 2000);
  window.addEventListener('focus', refreshSignupNotices);
  document.addEventListener('visibilitychange', refreshSignupNotices);
});

onUnmounted(() => {
  if (signupNoticeTimer) {
    window.clearInterval(signupNoticeTimer);
  }
  if (signupEventRetryTimer) {
    window.clearTimeout(signupEventRetryTimer);
  }
  signupEventSource?.close();
  signupEventSource = undefined;
  window.removeEventListener('focus', refreshSignupNotices);
  document.removeEventListener('visibilitychange', refreshSignupNotices);
});
</script>

<template>
  <BasicLayout @clear-preferences-and-logout="handleLogout">
    <template #user-dropdown>
      <UserDropdown
        :avatar
        :menus
        :text="userStore.userInfo?.realName"
        description="ann.vben@gmail.com"
        tag-text="Pro"
        @logout="handleLogout"
        @clear-preferences-and-logout="handleLogout"
      />
    </template>
    <template #notification>
      <Notification
        :dot="showDot"
        :notifications="notifications"
        @clear="handleNoticeClear"
        @read="(item) => item.id && markRead(item.id)"
        @remove="(item) => item.id && remove(item.id)"
        @make-all="handleMakeAll"
        @on-click="handleClick"
        @view-all="viewAll"
      />
    </template>
    <template #extra>
      <AuthenticationLoginExpiredModal
        v-model:open="accessStore.loginExpired"
        :avatar
      >
        <LoginForm />
      </AuthenticationLoginExpiredModal>
    </template>
    <template #lock-screen>
      <LockScreen :avatar @to-login="handleLogout" />
    </template>
  </BasicLayout>
</template>
