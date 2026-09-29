import type { AgentDistributionAnalytics, DistributionAnalytics } from '../../api/core/distribution';
import type { distributionAnalyticsScope } from './distribution-analytics-scope';

import { computed, ref, shallowRef } from 'vue';

type AnalyticsScope = ReturnType<typeof distributionAnalyticsScope>;

export function useDistributionAnalytics(options: {
  initialAnalytics: DistributionAnalytics;
  initialScope: AnalyticsScope;
  loadAdmin: () => Promise<DistributionAnalytics>;
  loadAgent: (params?: { endDate?: string; startDate?: string }) => Promise<AgentDistributionAnalytics>;
  onError: () => void;
}) {
  const loaded = shallowRef({
    analytics: options.initialAnalytics,
    agentAnalytics: null as AgentDistributionAnalytics | null,
    scope: options.initialScope,
  });
  const analyticsLoading = ref(false);
  let requestSequence = 0;

  async function load(scope: AnalyticsScope) {
    const requestId = ++requestSequence;
    analyticsLoading.value = true;
    try {
      const result = scope.showDateFilter
        ? { analytics: options.initialAnalytics, agentAnalytics: await options.loadAgent(scope.params), scope }
        : { analytics: await options.loadAdmin(), agentAnalytics: null, scope };
      if (requestId !== requestSequence) return false;
      // Publish the response and its date labels as one snapshot.
      loaded.value = result;
      return true;
    } catch {
      if (requestId === requestSequence) options.onError();
      return false;
    } finally {
      if (requestId === requestSequence) analyticsLoading.value = false;
    }
  }

  function dispose() {
    requestSequence += 1;
    analyticsLoading.value = false;
  }

  return {
    analytics: computed(() => loaded.value.analytics),
    agentAnalytics: computed(() => loaded.value.agentAnalytics),
    analyticsScope: computed(() => loaded.value.scope),
    analyticsLoading,
    dispose,
    load,
  };
}
