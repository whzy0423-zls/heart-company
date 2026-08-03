import { readdirSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

describe('route module discovery', () => {
  it('keeps test files outside the auto-imported route modules directory', () => {
    const routeModuleFiles = readdirSync(
      resolve(process.cwd(), 'apps/web-antd/src/router/routes/modules'),
    );

    expect(routeModuleFiles.filter((file) => file.includes('.test.'))).toEqual(
      [],
    );
  });
});
