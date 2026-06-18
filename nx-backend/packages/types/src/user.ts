import type { BasicUserInfo } from '@vben-core/typings';

/** 用户信息 */
interface UserInfo extends BasicUserInfo {
  /**
   * 邮箱
   */
  email?: string;
  /**
   * 用户描述
   */
  desc: string;
  /**
   * 首页地址
   */
  homePath: string;
  /**
   * 手机号
   */
  phone?: string;
  /**
   * 个人简介
   */
  remark?: string;

  /**
   * accessToken
   */
  token: string;
}

export type { UserInfo };
