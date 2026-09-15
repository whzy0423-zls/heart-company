<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Page } from '@vben/common-ui';
import { ElMessage } from 'element-plus';
import { getDistributionAgentsApi, createDistributionAgentApi, updateDistributionAgentStatusApi, type DistributionAgent } from '#/api/core/distribution';
const agents=ref<DistributionAgent[]>([]); const appUserId=ref(''); const loading=ref(false);
async function load(){loading.value=true;try{agents.value=(await getDistributionAgentsApi()).items;}finally{loading.value=false;}}
async function create(){const id=Number(appUserId.value);if(!id)return ElMessage.warning('请输入 App 用户 ID');await createDistributionAgentApi({appUserId:id});appUserId.value='';ElMessage.success('已开通一级代理');await load();}
async function toggle(a:DistributionAgent){const status=a.status==='active'?'paused':'active';await updateDistributionAgentStatusApi(a.id,status);a.status=status;ElMessage.success('状态已更新');}
onMounted(load);
</script>
<template><Page title="分销代理管理"><div class="toolbar"><input v-model="appUserId" placeholder="App 用户 ID"/><button @click="create">开通一级代理</button></div><table v-loading="loading"><thead><tr><th>ID</th><th>用户 ID</th><th>代理号</th><th>等级</th><th>状态</th><th>操作</th></tr></thead><tbody><tr v-for="a in agents" :key="a.id"><td>{{a.id}}</td><td>{{a.appUserId}}</td><td>{{a.agentCode}}</td><td>一级/二级/三级[{{a.level}}]</td><td>{{a.status}}</td><td><button @click="toggle(a)">{{a.status==='active'?'暂停':'恢复'}}</button></td></tr></tbody></table></Page></template>
<style scoped>.toolbar{display:flex;gap:8px;margin-bottom:16px}input{padding:8px}button{padding:8px 12px;cursor:pointer}table{width:100%;background:white;border-collapse:collapse}th,td{padding:12px;border-bottom:1px solid #eee;text-align:left}</style>
