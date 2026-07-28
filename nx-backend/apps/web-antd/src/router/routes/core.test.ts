import { describe, expect, it } from 'vitest';

import { coreRoutes } from './core';

describe('legacy backend menu redirects', () => {
  it('redirects removed Xinzhili model menu path to the consolidated model page', () => {
    const route = coreRoutes.find(
      (item) => item.name === 'LegacyXinzhiliModelConfig',
    );

    expect(route?.path).toBe('/settings/xinzhili-model');
    expect(route?.redirect).toBe('/settings/model');
    expect(route?.meta?.hideInMenu).toBe(true);
  });

  it('redirects removed theory library menu path to the current RAG knowledge page', () => {
    const route = coreRoutes.find(
      (item) => item.name === 'LegacyTheoryLibrary',
    );

    expect(route?.path).toBe('/theory/library');
    expect(route?.redirect).toBe('/rag/knowledge');
    expect(route?.meta?.hideInMenu).toBe(true);
  });
});
