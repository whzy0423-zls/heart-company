import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const repoRoot = resolve(webRoot, '../..');
const readWeb = (path: string) => readFileSync(resolve(webRoot, path), 'utf8');
const readRepo = (path: string) => readFileSync(resolve(repoRoot, path), 'utf8');

describe('production workbench mode entry', () => {
  it('registers production mode and short mode routes under /video', () => {
    const routes = readWeb('src/router/routes/modules/video.ts');

    expect(routes).toContain("path: 'production'");
    expect(routes).toContain("name: 'VideoProduction'");
    expect(routes).toContain("#/views/video/production/index.vue");
    expect(routes).toContain("path: 'production/short'");
    expect(routes).toContain("name: 'VideoProductionShort'");
    expect(routes).toContain("#/views/video/production/short.vue");
    expect(routes).toContain("activePath: '/video/production'");
  });

  it('offers project mode and short mode choices with stable target paths', () => {
    const page = readWeb('src/views/video/production/index.vue');

    expect(page).toContain('制片工作台');
    expect(page).toContain('项目制');
    expect(page).toContain('短片制');
    expect(page).toContain('/video/projects');
    expect(page).toContain('/video/production/short');
  });



  it('imports Ant Design components used by a-prefixed mode cards', () => {
    const page = readWeb('src/views/video/production/index.vue');
    const short = readWeb('src/views/video/production/short.vue');

    for (const alias of ['Button as AButton', 'Card as ACard', 'Tag as ATag']) {
      expect(page).toContain(alias);
    }
    for (const alias of [
      'Button as AButton',
      'Card as ACard',
      'Result as AResult',
      'Space as ASpace',
      'Step as AStep',
      'Steps as ASteps',
    ]) {
      expect(short).toContain(alias);
    }
  });

  it('seeds backend-access menus for production mode', () => {
    const db = readRepo('apps/server/internal/db/db.go');

    expect(db).toContain('VideoProduction');
    expect(db).toContain('/video/production');
    expect(db).toContain('/video/production/index');
    expect(db).toContain('VideoProductionShort');
    expect(db).toContain('/video/production/short');
    expect(db).toContain('ActivePath: "/video/production"');
    expect(db).toContain('metaMap["activePath"]');
  });

  it('keeps the existing project list visible as the project-mode core entry', () => {
    const db = readRepo('apps/server/internal/db/db.go');
    const projectMenuLine = db
      .split('\n')
      .find((line) => line.includes('Name: "VideoProjects"'));

    expect(projectMenuLine).toContain('Path: "/video/projects"');
    expect(projectMenuLine).toContain('Title: "项目列表"');
    expect(projectMenuLine).not.toContain('HideInMenu: true');
    expect(projectMenuLine).not.toContain('ActivePath: "/video/production"');
  });

  it('keeps short mode as a clear placeholder with project-mode escape route', () => {
    const page = readWeb('src/views/video/production/short.vue');

    expect(page).toContain('短片制工作台');
    expect(page).toContain('功能规划中');
    expect(page).toContain('脚本输入');
    expect(page).toContain('快速分镜');
    expect(page).toContain('/video/production');
    expect(page).toContain('/video/projects');
  });

  it('documents activePath and activeMenu compatibility in backend menu metadata', () => {
    const db = readRepo('apps/server/internal/db/db.go');

    expect(db).toContain('activePath is consumed by the Vben menu');
    expect(db).toContain('activeMenu');
  });

});
