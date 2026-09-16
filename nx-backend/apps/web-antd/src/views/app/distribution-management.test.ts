import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const source = readFileSync(
  resolve('apps/web-antd/src/views/app/distribution-management.vue'),
  'utf8',
);
const apiSource = readFileSync(
  resolve('apps/web-antd/src/api/core/distribution.ts'),
  'utf8',
);
const serverSource = readFileSync(
  resolve('apps/server/internal/server/distribution.go'),
  'utf8',
);

describe('distribution management page', () => {
  it('creates agents by searching and selecting current App customers', () => {
    for (const expected of [
      '新增代理',
      '搜索并选择 App 客户',
      '输入手机号、昵称或账号搜索',
      'searchAppCustomers',
      'selectedCustomerId',
      'getAppCustomerListApi',
      '代理号由后台自动生成',
      'createDistributionAgentApi({ appUserId: selectedCustomerId.value })',
      'Customer:App:Write',
      '请先选择 App 客户',
      '创建后用户会立即获得一级代理身份',
      'customerIsExistingAgent',
      '已是代理',
      ':disabled="customerIsExistingAgent(customer.id)"',
    ]) {
      expect(source).toContain(expected);
    }
    expect(source).not.toContain('自定义代理号');
    expect(source).not.toContain('createForm.agentCode');
  });

  it('does not let frontend submit custom agent codes', () => {
    expect(apiSource).toContain('createDistributionAgentApi');
    expect(apiSource).toContain('data: { appUserId: number }');
    expect(apiSource).not.toContain('agentCode?: string');
  });

  it('backend generates agent code from App user id', () => {
    const createHandler = serverSource.slice(
      serverSource.indexOf('func (s *Server) adminDistributionAgentCreate'),
      serverSource.indexOf('func (s *Server) appDistributionOverview'),
    );
    expect(createHandler).toContain('code := "A" + strconv.FormatInt(in.AppUserID, 10)');
    expect(createHandler).not.toContain('AgentCode string');
    expect(createHandler).not.toContain('strings.TrimSpace(in.AgentCode)');
  });
});
