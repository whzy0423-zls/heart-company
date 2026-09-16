<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Page } from '@vben/common-ui';
import { Table, Tag, message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
type Commission={ID:number;OrderID:number;AgentID:number;AppUserID:number;Level:number;OrderAmount:number;RateBPS:number;Amount:number;RuleVersion:number;Status:string;CreatedAt:string};const items=ref<Commission[]>([]);const loading=ref(false);
async function load(){loading.value=true;try{items.value=(await requestClient.get<{items:Commission[]}>('/admin/distribution/commissions')).items;}catch{message.error('佣金明细加载失败')}finally{loading.value=false}}onMounted(load);
</script>
<template><Page title="佣金明细"><Table :columns="[{title:'ID',dataIndex:'ID'},{title:'订单',dataIndex:'OrderID'},{title:'代理',dataIndex:'AgentID'},{title:'订单金额(分)',dataIndex:'OrderAmount'},{title:'佣金(分)',dataIndex:'Amount'},{title:'规则版本',dataIndex:'RuleVersion'},{title:'状态',dataIndex:'Status'}]" :data-source="items" :loading="loading" :pagination="false" row-key="ID"><template #bodyCell="{column,record}"><template v-if="column.dataIndex==='Status'"><Tag>{{record.Status}}</Tag></template></template></Table></Page></template>
