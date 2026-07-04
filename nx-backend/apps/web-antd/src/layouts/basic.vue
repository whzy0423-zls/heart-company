<script lang="ts" setup>
import type { NotificationItem } from '@vben/layouts';

import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';

import { AuthenticationLoginExpiredModal } from '@vben/common-ui';
import { useAppConfig, useWatermark } from '@vben/hooks';
import {
  BasicLayout,
  LockScreen,
  Notification,
  UserDropdown,
} from '@vben/layouts';
import { preferences, usePreferences } from '@vben/preferences';
import { useAccessStore, useUserStore } from '@vben/stores';
import { openWindow } from '@vben/utils';

import { notification } from 'ant-design-vue';

import { getSignupLeadListApi } from '#/api';
import { $t } from '#/locales';
import { useAuthStore } from '#/store';
import LoginForm from '#/views/_core/authentication/login.vue';

import {
  buildSignupEventsURL,
  extractSignupStreamEvents,
  shouldLogoutForSignupEventStatus,
  shouldPollSignupNoticeFallback,
  signupNoticeIdentity,
} from './signup-events';
import { toSignupNotification } from './signup-notice';

const notifications = ref<NotificationItem[]>([]);
const SIGNUP_NOTICE_LAST_ID_KEY_PREFIX = 'nx-signup-notice-last-id:v2';
const SIGNUP_NOTICE_FALLBACK_POLL_INTERVAL = 60_000;
let signupNoticeTimer: number | undefined;
let signupEventController: AbortController | undefined;
let signupEventConnecting = false;
let signupEventUnavailable = false;
let signupEventRetryTimer: number | undefined;
let signupNoticeBootstrapped = false;
let currentSignupNoticeIdentity = '';
const seenSignupNoticeIds = new Set<string>();

const router = useRouter();
const userStore = useUserStore();
const authStore = useAuthStore();
const accessStore = useAccessStore();
const { apiURL } = useAppConfig(import.meta.env, import.meta.env.PROD);
const { destroyWatermark, updateWatermark } = useWatermark();
const { isDark } = usePreferences();
const showDot = computed(() =>
  notifications.value.some((item) => !item.isRead),
);
const canReadSignupLeads = computed(() =>
  accessStore.accessCodes.includes('Customer:Signup:List'),
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
    openWindow(link, { target: '_blank' });
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
  if (!accessStore.accessToken || !canReadSignupLeads.value) return;
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
      .slice()
      .sort((a, b) => Number(a.id) - Number(b.id))
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

async function expireSignupNoticeAuth() {
  notification.warning({
    description:
      '当前登录状态和本地 Go 后端不一致，请重新登录后台后再测试报名推送。',
    message: '报名通知连接已失效',
    placement: 'topRight',
  });
  accessStore.setAccessToken(null);
  await authStore.logout();
}

function handleSignupEventData(data: string) {
  try {
    const lead = JSON.parse(data);
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
}

function disconnectSignupEvents() {
  signupEventController?.abort();
  signupEventController = undefined;
  signupEventConnecting = false;
}

function resetSignupNoticeSessionState() {
  seenSignupNoticeIds.clear();
  signupNoticeBootstrapped = false;
  notifications.value = [];
  signupEventUnavailable = false;
  if (signupEventRetryTimer) {
    window.clearTimeout(signupEventRetryTimer);
    signupEventRetryTimer = undefined;
  }
  disconnectSignupEvents();
}

function currentSignupIdentity() {
  return signupNoticeIdentity({
    accessToken: accessStore.accessToken,
    userId: userStore.userInfo?.id || userStore.userInfo?.userId,
    username: userStore.userInfo?.username,
  });
}

function scheduleSignupEventRetry() {
  signupEventUnavailable = true;
  if (signupEventRetryTimer) {
    window.clearTimeout(signupEventRetryTimer);
  }
  signupEventRetryTimer = window.setTimeout(() => {
    signupEventUnavailable = false;
    signupEventRetryTimer = undefined;
    connectSignupEvents();
  }, 15_000);
}

async function readSignupEventStream(
  controller: AbortController,
  token: string,
) {
  let shouldRetry = false;
  try {
    const response = await fetch(buildSignupEventsURL(apiURL), {
      headers: {
        Accept: 'text/event-stream',
        Authorization: `Bearer ${token}`,
      },
      signal: controller.signal,
    });
    if (shouldLogoutForSignupEventStatus(response.status)) {
      shouldRetry = false;
      await expireSignupNoticeAuth();
      return;
    }
    if (!response.ok || !response.body) {
      shouldRetry = true;
      return;
    }

    signupEventConnecting = false;
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const result = extractSignupStreamEvents(buffer);
      buffer = result.remaining;
      for (const event of result.events) {
        if (event.event === 'signup') {
          handleSignupEventData(event.data);
        }
      }
    }

    shouldRetry = !controller.signal.aborted;
  } catch {
    shouldRetry = !controller.signal.aborted;
  } finally {
    if (signupEventController === controller) {
      signupEventController = undefined;
    }
    signupEventConnecting = false;
    if (shouldRetry) {
      scheduleSignupEventRetry();
    }
  }
}

function connectSignupEvents() {
  if (
    !accessStore.accessToken ||
    !canReadSignupLeads.value ||
    signupEventController ||
    signupEventConnecting ||
    signupEventUnavailable
  )
    return;

  const controller = new AbortController();
  signupEventController = controller;
  signupEventConnecting = true;
  void readSignupEventStream(controller, accessStore.accessToken);
}

function refreshSignupNotices() {
  const nextIdentity = currentSignupIdentity();
  if (nextIdentity !== currentSignupNoticeIdentity) {
    currentSignupNoticeIdentity = nextIdentity;
    resetSignupNoticeSessionState();
  }
  if (!canReadSignupLeads.value) {
    disconnectSignupEvents();
    return;
  }
  pollSignupNotices();
  connectSignupEvents();
}

function pollSignupNoticesWhenStreamNeedsFallback() {
  if (!canReadSignupLeads.value) return;
  if (
    shouldPollSignupNoticeFallback({
      connecting: signupEventConnecting,
      controllerActive: Boolean(signupEventController),
      unavailable: signupEventUnavailable,
    })
  ) {
    pollSignupNotices();
  }
}

onMounted(() => {
  refreshSignupNotices();
  signupNoticeTimer = window.setInterval(() => {
    pollSignupNoticesWhenStreamNeedsFallback();
  }, SIGNUP_NOTICE_FALLBACK_POLL_INTERVAL);
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
  disconnectSignupEvents();
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
