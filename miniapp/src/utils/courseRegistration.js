// Only an explicit boolean false disables course registration. Legacy site
// catalogs and the independent video classroom switch keep their own behavior.
export function isCourseRegistrationEnabled(config) {
  return config?.home?.miniappCourses?.enabled !== false
}
