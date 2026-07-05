import type { Component } from 'vue';

import { createApp, nextTick } from 'vue';

export async function flushVuePromises() {
  await Promise.resolve();
  await Promise.resolve();
  await nextTick();
}

export function mountVueComponent(component: Component) {
  const root = document.createElement('div');
  document.body.append(root);
  const app = createApp(component);
  app.mount(root);

  return {
    button(label: string) {
      return [...root.querySelectorAll('button')].find(
        (item) => item.textContent?.trim() === label,
      );
    },
    text() {
      return root.textContent ?? '';
    },
    unmount() {
      app.unmount();
      root.remove();
    },
  };
}
