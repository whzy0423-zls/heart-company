import { requestClient } from '#/api/request';

interface SystemPageResult<T> {
  items: T[];
  total: number;
}

export interface SystemUser {
  avatar?: string;
  createTime?: string;
  deptId?: string;
  email?: string;
  id?: string;
  nickname: string;
  password?: string;
  remark?: string;
  roleIds: string[];
  status: number;
  username: string;
}

export interface SystemRole {
  code: string;
  createTime?: string;
  id?: string;
  menuIds: number[];
  name: string;
  permissions?: number[];
  remark?: string;
  status: number;
}

export interface SystemMenu {
  authCode?: string;
  children?: SystemMenu[];
  component?: string;
  id: number;
  meta?: {
    icon?: string;
    title?: string;
  };
  name: string;
  path: string;
  pid?: number;
  sort?: number;
  status: number;
  type: string;
}

export function getSystemUserListApi(params?: Record<string, any>) {
  return requestClient.get<SystemPageResult<SystemUser>>('/system/user/list', {
    params,
  });
}

export function saveSystemUserApi(data: SystemUser) {
  return requestClient.post<SystemUser>('/system/user', data);
}

export function deleteSystemUserApi(id: string) {
  return requestClient.delete<boolean>('/system/user', { params: { id } });
}

export function getSystemRoleListApi(params?: Record<string, any>) {
  return requestClient.get<SystemPageResult<SystemRole>>('/system/role/list', {
    params,
  });
}

export function saveSystemRoleApi(data: SystemRole) {
  return requestClient.post<SystemRole>('/system/role', data);
}

export function deleteSystemRoleApi(id: string) {
  return requestClient.delete<boolean>('/system/role', { params: { id } });
}

export function getSystemMenuListApi() {
  return requestClient.get<SystemMenu[]>('/system/menu/list');
}

export function saveSystemMenuApi(data: SystemMenu) {
  return requestClient.post<SystemMenu>('/system/menu', data);
}

export function deleteSystemMenuApi(id: number) {
  return requestClient.delete<boolean>('/system/menu', { params: { id } });
}
