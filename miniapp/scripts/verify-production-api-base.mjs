import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function loadEnvFile(path) {
  if (!existsSync(path)) return {};
  return Object.fromEntries(
    readFileSync(path, 'utf8')
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line && !line.startsWith('#'))
      .map((line) => line.replace(/^export\s+/, ''))
      .filter((line) => line.includes('='))
      .map((line) => {
        const index = line.indexOf('=');
        const key = line.slice(0, index).trim();
        const value = line.slice(index + 1).trim().replace(/^['"]|['"]$/g, '');
        return [key, value];
      }),
  );
}

const productionEnv = loadEnvFile(resolve('.env.production'));
const apiBase = String(process.env.VITE_API_BASE || productionEnv.VITE_API_BASE || '')
  .trim()
  .replace(/\/+$/, '');

function fail(message) {
  console.error(message);
  process.exit(1);
}

if (!apiBase) {
  fail('VITE_API_BASE is required for mp-weixin production builds.');
}

if (!apiBase.startsWith('https://')) {
  fail('VITE_API_BASE must be a HTTPS URL for mp-weixin production builds.');
}

if (apiBase.includes('api.example.com') || apiBase.includes('localhost') || apiBase.includes('127.0.0.1')) {
  fail('VITE_API_BASE must not use placeholder or local development hosts.');
}
