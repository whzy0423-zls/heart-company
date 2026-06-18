import type { SiteConfig } from '#/api';

import { computed, onMounted, ref } from 'vue';

import { message } from 'ant-design-vue';

import { getSiteConfigApi, updateSiteConfigApi } from '#/api';

const config = ref<SiteConfig>();
const loading = ref(false);
const saving = ref(false);
let loaded = false;

export function useSiteConfigEditor() {
  const metrics = computed(() => {
    const current = config.value;
    return {
      courseCount: current?.home.courses.items.length ?? 0,
      drawerNavCount: current?.navigation.drawer.length ?? 0,
      homeSectionCount: current ? Object.keys(current.home).length : 0,
      mainNavCount: current?.navigation.main.length ?? 0,
      quoteCount: current?.home.quotes.items.length ?? 0,
      stageCount: current?.home.stages.items.length ?? 0,
      tabCount: current?.navigation.tabs.length ?? 0,
      typeCount: current?.types.length ?? 0,
    };
  });

  async function loadConfig(force = false) {
    if (loaded && !force) return;
    loading.value = true;
    try {
      config.value = await getSiteConfigApi();
      loaded = true;
    } finally {
      loading.value = false;
    }
  }

  async function saveConfig() {
    if (!config.value) return;
    if (!config.value.site.brandName?.trim()) {
      message.warning('请填写品牌名称');
      return;
    }
    if (!config.value.site.logo?.trim()) {
      message.warning('请上传站点 Logo');
      return;
    }
    saving.value = true;
    try {
      config.value = await updateSiteConfigApi(config.value);
      message.success('已保存官网配置');
    } finally {
      saving.value = false;
    }
  }

  function linesToArray(value: string) {
    return value
      .split('\n')
      .map((item) => item.trim())
      .filter(Boolean);
  }

  onMounted(() => {
    void loadConfig();
  });

  return {
    config,
    linesToArray,
    loadConfig,
    loading,
    metrics,
    saveConfig,
    saving,
  };
}
