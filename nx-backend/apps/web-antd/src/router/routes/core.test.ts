import { describe, expect, it } from 'vitest';

import { coreRoutes } from './core';

describe('legacy backend menu redirects', () => {
  it('does not hijack the restored Xinzhili model menu path', () => {
    const route = coreRoutes.find(
      (item) => item.name === 'LegacyXinzhiliModelConfig',
    );

    expect(route).toBeUndefined();
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
