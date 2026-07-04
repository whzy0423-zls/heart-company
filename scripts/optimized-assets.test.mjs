import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';

function statSize(file) {
  return fs.statSync(file).size;
}

function assertExists(file) {
  assert.ok(fs.existsSync(file), `${file} should exist`);
}

function assertNotExists(file) {
  assert.ok(!fs.existsSync(file), `${file} should not exist`);
}

function assertBelow(file, maxBytes) {
  const size = statSize(file);
  assert.ok(size <= maxBytes, `${file} is ${size} bytes, expected <= ${maxBytes}`);
}

for (let i = 1; i <= 9; i += 1) {
  const webp = path.join('website-react/public/assets/avatars', `${i}.webp`);
  assertExists(webp);
  assertBelow(webp, 40 * 1024);
  assertNotExists(path.join('website-react/public/assets/avatars', `${i}.png`));
}

for (const file of ['logo.png', 'favicon.png']) {
  const target = path.join('nx-backend/apps/web-antd/public', file);
  assertExists(target);
  assertBelow(target, 48 * 1024);
}

for (let i = 1; i <= 9; i += 1) {
  const avatar = path.join('miniapp/src/static/avatars', `${i}.png`);
  assertExists(avatar);
  assertBelow(avatar, 24 * 1024);
}
assertBelow('miniapp/src/static/wheel.png', 80 * 1024);


for (const file of [
  'website-react/public/assets/.DS_Store',
  'website-react/public/assets/avatars/.DS_Store',
  'miniapp/src/static/.DS_Store',
  'miniapp/src/static/avatars/.DS_Store',
]) {
  assertNotExists(file);
}

console.log('optimized asset tests passed');
