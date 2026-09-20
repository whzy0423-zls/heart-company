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
const serverMainSource = readFileSync(
  resolve('apps/server/internal/server/server.go'),
  'utf8',
);
const appRoutesSource = readFileSync(
  resolve('apps/web-antd/src/router/routes/modules/app.ts'),
  'utf8',
);
const commissionSource = readFileSync(
  resolve('apps/web-antd/src/views/app/distribution-commissions.vue'),
  'utf8',
);
const settlementSource = readFileSync(
  resolve('apps/web-antd/src/views/app/distribution-settlements.vue'),
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

  it('shows first-level agent development counts and child agent details', () => {
    for (const expected of [
      '发展概况',
      '直属客户',
      '下级二级代理',
      '下级三级代理',
      '查看下级',
      '下级代理明细',
      'selectedParentAgent',
      'childAgentsForSelectedParent',
      'secondLevelAgentCount',
      'thirdLevelAgentCount',
      'directUserCount',
      'parentAgentId',
    ]) {
      expect(source).toContain(expected);
    }
  });

  it('types distribution agent hierarchy metrics returned by backend', () => {
    for (const expected of [
      'parentAgentId: number',
      'rootAgentId: number',
      'path: string',
      'directUserCount: number',
      'secondLevelAgentCount: number',
      'thirdLevelAgentCount: number',
    ]) {
      expect(apiSource).toContain(expected);
    }
  });

  it('backend returns hierarchy metrics for admin agent list', () => {
    const listHandler = serverSource.slice(
      serverSource.indexOf('func (s *Server) adminDistributionAgents'),
      serverSource.indexOf('func (s *Server) adminDistributionAgentStatus'),
    );
    for (const expected of [
      'DirectUserCount',
      'SecondLevelAgentCount',
      'ThirdLevelAgentCount',
      'COUNT(*) FROM distribution_user_relations rel WHERE rel.direct_agent_id=a.id',
      'child.level=2',
      'child.level=3',
    ]) {
      expect(listHandler).toContain(expected);
    }
  });

  it('renders business analytics with charts, yuan amounts, and ranking table', () => {
    for (const expected of [
      '经营数据分析',
      '累计分成金额',
      '待结算金额',
      '订单金额',
      '分成金额',
      '代理经营排行',
      '近 30 天经营趋势',
      'formatYuan',
      'getDistributionAnalyticsApi',
      'trendChartOption',
      'rankingColumns',
    ]) {
      expect(source).toContain(expected);
    }
    expect(source).not.toContain('金额(分)');
    expect(source).not.toContain('佣金(分)');
  });

  it('types and requests distribution analytics from the admin API', () => {
    for (const expected of [
      'DistributionAnalytics',
      'DistributionAnalyticsSummary',
      'DistributionAnalyticsTrendItem',
      'DistributionAnalyticsAgentRanking',
      "getDistributionAnalyticsApi = () => requestClient.get<DistributionAnalytics>('/admin/distribution/analytics')",
      'totalCommissionAmount: number',
      'pendingCommissionAmount: number',
      'agentRankings: DistributionAnalyticsAgentRanking[]',
    ]) {
      expect(apiSource).toContain(expected);
    }
  });

  it('backend exposes distribution analytics route and response fields', () => {
    const analyticsHandler = serverSource.slice(
      serverSource.indexOf('type distributionAnalyticsSummary'),
      serverSource.indexOf('func (s *Server) adminDistributionAgentCreate'),
    );
    for (const expected of [
      'distributionAnalyticsSummary',
      'distributionAnalyticsTrendItem',
      'distributionAnalyticsAgentRanking',
      'totalCommissionAmount',
      'pendingCommissionAmount',
      'agentRankings',
      'generateDistributionTrend',
      'ORDER BY commission_amount DESC',
    ]) {
      expect(analyticsHandler).toContain(expected);
    }
  });

  it('commission and settlement tables display monetary values in yuan', () => {
    for (const pageSource of [commissionSource, settlementSource]) {
      expect(pageSource).toContain('formatYuan');
      expect(pageSource).toContain('元');
      expect(pageSource).not.toContain('分)');
    }
    expect(commissionSource).toContain('订单金额(元)');
    expect(commissionSource).toContain('佣金(元)');
    expect(settlementSource).toContain('金额(元)');
  });

  it('supports app-agent backend login and restricted agent permissions', () => {
    for (const expected of [
      'tryAppAgentBackendLogin',
      'AuthenticateWithPassword',
      'TokenKindBackend',
      'Roles:     []string{"agent"}',
      'HomePath:  "/app/distribution"',
      'agentAccessCodes',
      'isAgentBackendUser',
      'CurrentAppAgentProfile',
    ]) {
      expect(serverMainSource).toContain(expected);
    }
    expect(appRoutesSource).toContain('Agent:Distribution:View');
    expect(appRoutesSource).toContain('Agent:Distribution:Write');
    expect(appRoutesSource).toContain('hideInMenu: true');
  });

  it('uses agent-scoped distribution APIs with date filters and child agent creation', () => {
    for (const expected of [
      '/agent/distribution/profile',
      '/agent/distribution/analytics',
      '/agent/distribution/agents',
      'getAgentDistributionAnalyticsApi',
      'createAgentDistributionChildApi',
      'startDate?: string',
      'endDate?: string',
    ]) {
      expect(apiSource + serverMainSource).toContain(expected);
    }
    for (const expected of [
      'currentDistributionAgent',
      "agent.agent_path LIKE $1 || '%'",
      'current.Level >= 3',
      'user was not directly invited by this agent',
      'agent cannot create child',
    ]) {
      expect(serverSource).toContain(expected);
    }
  });

  it('shows current agent code in agent backoffice for sharing', () => {
    for (const expected of [
      '我的代理号',
      'currentAgent?.agentCode',
      '复制代理号',
      'copyCurrentAgentCode',
      'navigator.clipboard.writeText(currentAgent.value.agentCode)',
      '代理号已复制，可以分享给别人',
    ]) {
      expect(source).toContain(expected);
    }
  });

  it('provides a centrally configured poster composer for agent sharing', () => {
    const posterSource = readFileSync(
      resolve('apps/web-antd/src/views/app/distribution-poster-composer.vue'),
      'utf8',
    );
    for (const expected of [
      '上传海报模板',
      '生成并下载 PNG',
      'templateUrl',
      'qrSize',
      'inviteWidth',
      'inviteCode',
      'landingUrl',
      'QRCode.toDataURL',
      'toDataURL',
      'getPosterConfigApi',
      'savePosterConfigApi',
    ]) {
      expect(posterSource).toContain(expected);
    }
    expect(source).toContain('<DistributionPosterComposer');
  });

  it('formats distribution detail timestamps as year-month-day hour-minute-second', () => {
    for (const expected of [
      'formatDateTime',
      "dayjs(value).format('YYYY-MM-DD HH:mm:ss')",
      'formatDateTime(record.boundAt)',
      'formatDateTime(record.paidAt)',
      'formatDateTime(row.boundAt)',
      'formatDateTime(row.paidAt)',
    ]) {
      expect(source).toContain(expected);
    }
  });

  it('keeps distribution detail tables scrollable and exports visible rows as Excel', () => {
    for (const expected of [
      'fixedTableScroll',
      ':scroll="fixedTableScroll"',
      '导出用户消费',
      '导出订单明细',
      'exportDistributionExcel',
      'application/vnd.ms-excel;charset=utf-8',
      '下级用户消费汇总',
      '下级订单明细',
      "message.warning('暂无可导出的数据')",
    ]) {
      expect(source).toContain(expected);
    }
  });

});
