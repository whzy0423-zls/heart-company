import { rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import path from 'node:path';

const projectRoot = process.cwd();
const targets = [
  ['dist/assets/uploads', 'uploaded runtime assets'],
];

for (const [relative, label] of targets) {
  const target = path.join(projectRoot, relative);
  if (!existsSync(target)) {
    console.log(`[prune-dist-assets] no ${relative} directory found`);
    continue;
  }
  await rm(target, { recursive: true, force: true });
  console.log(`[prune-dist-assets] removed ${relative} (${label}) from build output`);
}
