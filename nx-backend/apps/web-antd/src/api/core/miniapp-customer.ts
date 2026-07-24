import { requestClient } from '#/api/request';

export interface MiniappCustomer {
  avatar: string;
  channel: string;
  createTime: string;
  gender: string;
  id: string;
  lastLoginAt: string;
  mainType: number;
  memberLevel: number;
  nickname: string;
  phone: string;
  scene: string;
}

export interface MiniappTestRecord {
  centers: unknown[];
  createTime: string;
  gender: string;
  id: string;
  resultType: number;
  scores: Record<string, number>;
  secondType: number;
}

export interface MiniappBooking {
  contactName: string;
  createTime: string;
  id: string;
  intent: string;
  kind: string;
  message: string;
  phone: string;
  preferredTime: string;
  signupId: string;
  status: string;
}

export interface MiniappPage<T> { items: T[]; total: number }
export interface MiniappCustomerDetail {
  bookings: MiniappPage<MiniappBooking>;
  testRecords: MiniappPage<MiniappTestRecord>;
  user: MiniappCustomer;
}

export function getMiniappCustomerListApi(params: Record<string, unknown>) {
  return requestClient.get<MiniappPage<MiniappCustomer>>('/miniapp/users', { params });
}

export function getMiniappCustomerDetailApi(id: string, params: Record<string, unknown>) {
  return requestClient.get<MiniappCustomerDetail>(`/miniapp/users/${id}`, { params });
}
