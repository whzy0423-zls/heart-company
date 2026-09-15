<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Page } from '@vben/common-ui';
import { Button, Card, Input, Space, Table, Tag, message } from 'ant-design-vue';
import { createDistributionAgentApi, getDistributionAgentsApi, updateDistributionAgentStatusApi, type DistributionAgent } from '#/api/core/distribution';
const agents = ref<DistributionAgent[]>([]); const appUserId = ref(''); const loading = ref(false);
async function load() { loading.value = true; try { agents.value = (await getDistributionAgentsApi()).items; } catch { message.error('代理列表加载失败'); } finally { loading.value = false; } }
async function create() { const id = Number(appUserId.value); if (!id) { message.warning('请输入 App 用户 ID'); return; } try { await createDistributionAgentApi({ appUserId: id }); message.success('已开通一级代理'); appUserId.value = ''; await load(); } catch { message.error('开通失败'); } }
function agentOf(record: Record<string, any>) { return record as DistributionAgent; }
async function toggle(agent: DistributionAgent) { const status = agent.status === 'active' ? 'paused' : 'active'; try { await updateDistributionAgentStatusApi(agent.id, status); agent.status = status; message.success('状态已更新'); } catch { message.error('状态更新失败'); } }
onMounted(load);
</script>
<template><Page title="分销代理管理"><Card><Space><Input v-model:value="appUserId" placeholder="App 用户 ID" /><Button type="primary" @click="create">开通一级代理</Button></Space></Card><Table class="mt-4" :columns="[{ title: 'ID', dataIndex: 'id' }, { title: '用户', dataIndex: 'appUserId' }, { title: '代理号', dataIndex: 'agentCode' }, { title: '等级', dataIndex: 'level' }, { title: '状态', dataIndex: 'status' }, { title: '操作', key: 'action' }]" :data-source="agents" :loading="loading" :pagination="false" row-key="id"><template #bodyCell="{ column, record }"><template v-if="column.dataIndex === 'status'"><Tag :color="record.status === 'active' ? 'green' : 'default'">{{ record.status }}</Tag></template><template v-else-if="column.key === 'action'"><Button type="link" @click="toggle(agentOf(record))">{{ record.status === 'active' ? '暂停' : '恢复' }}</Button></template></template></Table></Page></template>
