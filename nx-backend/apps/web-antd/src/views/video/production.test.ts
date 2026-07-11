import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const repoRoot = resolve(webRoot, '../..');
const readWeb = (path: string) => readFileSync(resolve(webRoot, path), 'utf8');
const readRepo = (path: string) => readFileSync(resolve(repoRoot, path), 'utf8');

describe('production workbench mode entry', () => {
  it('registers production mode without an unavailable short route', () => {
    const routes = readWeb('src/router/routes/modules/video.ts');

    expect(routes).toContain("path: 'production'");
    expect(routes).toContain("name: 'VideoProduction'");
    expect(routes).toContain("#/views/video/production/index.vue");
    expect(routes).not.toContain("path: 'production/short'");
    expect(routes).not.toContain("name: 'VideoProductionShort'");
  });

  it('offers a real project creation center', () => {
    const page = readWeb('src/views/video/production/index.vue');

    expect(page).toContain('制片工作台');
    expect(page).toContain('listProjectsApi');
    expect(page).toContain('createProjectApi');
    expect(page).toContain('新建项目');
    expect(page).toContain('查看全部项目');
    expect(page).not.toContain('短片制');
  });



  it('does not use marketing mode cards', () => {
    const page = readWeb('src/views/video/production/index.vue');
    expect(page).not.toContain('ACard');
    expect(page).not.toContain('production-hero');
  });

  it('seeds backend-access menus for production mode', () => {
    const db = readRepo('apps/server/internal/db/db.go');

    expect(db).toContain('VideoProduction');
    expect(db).toContain('/video/production');
    expect(db).toContain('/video/production/index');
    expect(db).not.toContain('VideoProductionShort');
    expect(db).not.toContain('/video/production/short');
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

  it('documents activePath and activeMenu compatibility in backend menu metadata', () => {
    const db = readRepo('apps/server/internal/db/db.go');

    expect(db).toContain('activePath is consumed by the Vben menu');
    expect(db).toContain('activeMenu');
  });

});
