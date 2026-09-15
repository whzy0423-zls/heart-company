<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Page } from '@vben/common-ui';
import { Button, Card, InputNumber, Space, Table, message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
type Rule={id:number;version:number;status:string;createdAt:string;activatedAt:string};const rules=ref<Rule[]>([]);const loading=ref(false);const rates=ref({1:0,2:0,3:0});
async function load(){loading.value=true;try{rules.value=(await requestClient.get<{items:Rule[]}>('/admin/distribution/rules')).items;}catch{message.error('规则加载失败')}finally{loading.value=false}}
async function create(){try{await requestClient.post('/admin/distribution/rules',{name:'分销规则',rates:rates.value});message.success('规则草稿已创建');await load()}catch{message.error('规则创建失败')}}
function ruleOf(r: Record<string, any>) { return r as Rule; }
async function activate(r:Rule){try{await requestClient.post(`/admin/distribution/rules/${r.id}/activate`,{});message.success('规则已启用');await load()}catch{message.error('规则启用失败')}}
onMounted(load);
</script>
<template><Page title="佣金规则"><Card><Space><span>一级</span><InputNumber v-model:value="rates[1]" :min="0" :max="10000" /><span>二级</span><InputNumber v-model:value="rates[2]" :min="0" :max="10000" /><span>三级</span><InputNumber v-model:value="rates[3]" :min="0" :max="10000" /><Button type="primary" @click="create">创建草稿</Button></Space></Card><Table class="mt-4" :columns="[{title:'版本',dataIndex:'version'},{title:'状态',dataIndex:'status'},{title:'创建时间',dataIndex:'createdAt'},{title:'操作',key:'action'}]" :data-source="rules" :loading="loading" :pagination="false" row-key="id"><template #bodyCell="{column,record}"><template v-if="column.key==='action'"><Button v-if="record.status==='draft'" type="link" @click="activate(ruleOf(record))">启用</Button></template></template></Table></Page></template>
