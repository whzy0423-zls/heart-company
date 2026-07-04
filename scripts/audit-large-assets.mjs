import { readdir, stat } from 'node:fs/promises';
import path from 'node:path';

const root = process.cwd();
const args = process.argv.slice(2);
const dirs = args.length > 0 ? args : [
  'website-react/public/assets',
  'website-react/dist/assets',
  '../nine-xing-app/assets',
];
const thresholdBytes = Number(process.env.ASSET_AUDIT_THRESHOLD_BYTES || 1024 * 1024);
const maxRows = Number(process.env.ASSET_AUDIT_MAX_ROWS || 80);
const large = [];

async function walk(abs) {
  let entries;
  try {
    entries = await readdir(abs, { withFileTypes: true });
  } catch {
    return;
  }
  for (const entry of entries) {
    const file = path.join(abs, entry.name);
    if (entry.isDirectory()) {
      await walk(file);
      continue;
    }
    if (!entry.isFile()) continue;
    const info = await stat(file);
    if (info.size >= thresholdBytes) {
      large.push({ file: path.relative(root, file), size: info.size });
    }
  }
}

for (const dir of dirs) {
  await walk(path.resolve(root, dir));
}

large.sort((a, b) => b.size - a.size);
console.log(`Large assets >= ${(thresholdBytes / 1024 / 1024).toFixed(2)} MiB`);
for (const item of large.slice(0, maxRows)) {
  console.log(`${(item.size / 1024 / 1024).toFixed(2).padStart(8)} MiB  ${item.file}`);
}
if (large.length > maxRows) {
  console.log(`... ${large.length - maxRows} more`);
}
if (large.length === 0) {
  console.log('No large assets found.');
}
