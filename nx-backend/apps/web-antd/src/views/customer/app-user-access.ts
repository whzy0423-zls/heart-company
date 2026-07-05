export const APP_CUSTOMER_LIST_PERMISSION = 'Customer:App:List';
export const APP_USER_INSIGHTS_LIST_PERMISSION = 'Customer:UserInsights:List';

function hasAccessCode(accessCodes: string[] = [], code: string) {
  return accessCodes.includes(code);
}

export function canViewAppCustomers(accessCodes: string[] = []) {
  return hasAccessCode(accessCodes, APP_CUSTOMER_LIST_PERMISSION);
}

export function canViewUserInsights(accessCodes: string[] = []) {
  return hasAccessCode(accessCodes, APP_USER_INSIGHTS_LIST_PERMISSION);
}
