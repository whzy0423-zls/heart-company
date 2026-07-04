import type { AppCustomer } from '#/api';

export interface AppCustomerEditForm {
  memberLevel: string;
  status: string;
}

export const APP_CUSTOMER_WRITE_PERMISSION = 'Customer:App:Write';

export function canEditAppCustomer(accessCodes: string[] = []) {
  return accessCodes.includes(APP_CUSTOMER_WRITE_PERMISSION);
}

export function createAppCustomerEditForm(
  customer?: Pick<AppCustomer, 'memberLevel' | 'status'>,
): AppCustomerEditForm {
  return {
    memberLevel: customer?.memberLevel || 'free',
    status: customer?.status || 'active',
  };
}

export function buildAppCustomerUpdatePayload(form: AppCustomerEditForm) {
  return {
    memberLevel: form.memberLevel,
    status: form.status,
  };
}
