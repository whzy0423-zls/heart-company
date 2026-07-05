export function resultPersonaText(profile, gender) {
  if (!profile) return ''
  if (gender === 'male') return profile.male || profile.base || ''
  if (gender === 'female') return profile.female || profile.base || ''
  return profile.base || profile.summary || profile.male || profile.female || ''
}
