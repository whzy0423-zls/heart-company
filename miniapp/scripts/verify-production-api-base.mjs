import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { validateProductionApiBase } from '../src/apiBaseValidation.mjs';

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
const apiBase = process.env.VITE_API_BASE || productionEnv.VITE_API_BASE || '';
const validation = validateProductionApiBase(apiBase);

function fail(message) {
  console.error(message);
  process.exit(1);
}

if (validation.reason === 'required') {
  fail('VITE_API_BASE is required for production builds.');
}

if (validation.reason === 'https') {
  fail('VITE_API_BASE must be a HTTPS URL for production builds.');
}

if (validation.reason === 'invalid') {
  fail('VITE_API_BASE must be a valid HTTPS URL for production builds.');
}

if (validation.reason === 'blocked') {
  fail('VITE_API_BASE must not use placeholder or local development hosts.');
}
