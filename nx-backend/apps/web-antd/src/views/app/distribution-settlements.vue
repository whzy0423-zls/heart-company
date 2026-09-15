<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Page } from '@vben/common-ui';
import { Table, Tag, message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
type Settlement={id:number;agentId:number;periodStart:string;periodEnd:string;amount:number;status:string;createdAt:string};const items=ref<Settlement[]>([]);const loading=ref(false);
async function load(){loading.value=true;try{items.value=(await requestClient.get<{items:Settlement[]}>('/admin/distribution/settlements')).items;}catch{message.error('结算列表加载失败')}finally{loading.value=false}}onMounted(load);
</script>
<template><Page title="分销结算"><Table :columns="[{title:'批次',dataIndex:'id'},{title:'代理',dataIndex:'agentId'},{title:'周期',key:'period'},{title:'金额(分)',dataIndex:'amount'},{title:'状态',dataIndex:'status'},{title:'操作',key:'action'}]" :data-source="items" :loading="loading" :pagination="false" row-key="id"><template #bodyCell="{column,record}"><template v-if="column.key==='period'">{{record.periodStart}} 至 {{record.periodEnd}}</template><template v-else-if="column.dataIndex==='status'"><Tag>{{record.status}}</Tag></template><template v-else-if="column.key==='action'"><span v-if="record.status==='draft'">待审核</span><span v-else-if="record.status==='approved'">待打款</span><span v-else>—</span></template></template></Table></Page></template>
