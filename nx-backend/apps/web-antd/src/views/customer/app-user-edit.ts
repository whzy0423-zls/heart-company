import type { AppCustomer } from '#/api';

export interface AppCustomerEditForm {
  email?: string;
  memberLevel: string;
  status: string;
}

export const APP_CUSTOMER_WRITE_PERMISSION = 'Customer:App:Write';

export function canEditAppCustomer(accessCodes: string[] = []) {
  return accessCodes.includes(APP_CUSTOMER_WRITE_PERMISSION);
}

export function createAppCustomerEditForm(
  customer?: Pick<AppCustomer, 'email' | 'memberLevel' | 'status'>,
): AppCustomerEditForm {
  const form: AppCustomerEditForm = {
    memberLevel: customer?.memberLevel || 'free',
    status: customer?.status || 'active',
  };
  if (customer?.email) form.email = customer.email;
  return form;
}

export function buildAppCustomerUpdatePayload(form: AppCustomerEditForm) {
  return {
    memberLevel: form.memberLevel,
    status: form.status,
    ...(form.email !== undefined ? { email: form.email.trim() } : {}),
  };
}
