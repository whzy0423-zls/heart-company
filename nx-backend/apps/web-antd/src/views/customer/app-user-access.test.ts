import { describe, expect, it } from 'vitest';

import {
  canViewAppCustomers,
  canViewUserInsights,
} from './app-user-access';

describe('app customer access helpers', () => {
  it('requires the App customer list permission for App customer links', () => {
    expect(canViewAppCustomers([])).toBe(false);
    expect(canViewAppCustomers(['Customer:UserInsights:List'])).toBe(false);
    expect(canViewAppCustomers(['Customer:App:List'])).toBe(true);
  });

  it('requires the user insights list permission for 360 links', () => {
    expect(canViewUserInsights([])).toBe(false);
    expect(canViewUserInsights(['Customer:App:List'])).toBe(false);
    expect(canViewUserInsights(['Customer:UserInsights:List'])).toBe(true);
  });
});
