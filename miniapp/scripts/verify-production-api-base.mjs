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
  fail('VITE_API_BASE is required for production builds.');
}

if (!apiBase.startsWith('https://')) {
  fail('VITE_API_BASE must be a HTTPS URL for production builds.');
}

let parsed;
try {
  parsed = new URL(apiBase);
} catch {
  fail('VITE_API_BASE must be a valid HTTPS URL for production builds.');
}

const host = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, '');

function isPrivateIPv4(value) {
  const parts = value.split('.').map((part) => Number(part));
  if (parts.length !== 4 || parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)) {
    return false;
  }
  return (
    parts[0] === 10 ||
    parts[0] === 127 ||
    (parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31) ||
    (parts[0] === 192 && parts[1] === 168) ||
    (parts[0] === 169 && parts[1] === 254) ||
    parts[0] === 0
  );
}

if (
  host === 'localhost' ||
  host.endsWith('.localhost') ||
  host.endsWith('.local') ||
  host === '::1' ||
  isPrivateIPv4(host) ||
  host === 'api.example.com' ||
  host.endsWith('.example.com') ||
  host === 'api.yourdomain.com' ||
  host.endsWith('.yourdomain.com')
) {
  fail('VITE_API_BASE must not use placeholder or local development hosts.');
}
