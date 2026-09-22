import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./profile.vue', import.meta.url), 'utf8');

assert.match(source, /createWechatPayTestOrderApi/);
assert.match(source, /测试微信支付 ¥0\.10/);
assert.match(source, /uni\.requestPayment/);
assert.match(source, /pay\.devMode/);
assert.match(source, /paymentTesting/);
assert.doesNotMatch(source, /createWechatPayTestOrderApi\([^)]*amount/);

console.log('profile payment test contract passed');
