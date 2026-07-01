const apiBase = String(process.env.VITE_API_BASE || '').trim().replace(/\/+$/, '');

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
