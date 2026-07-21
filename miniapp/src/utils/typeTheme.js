const FALLBACK_THEME = Object.freeze({
  accent: '#68727C',
  soft: '#ECEAE5',
  ink: '#17212B',
})

export const TYPE_THEME = Object.freeze({
  1: Object.freeze({ accent: '#315BEA', soft: '#E4E9FC', ink: '#17317F' }),
  2: Object.freeze({ accent: '#C9472D', soft: '#F5DDD6', ink: '#7A2D20' }),
  3: Object.freeze({ accent: '#B86A12', soft: '#F5E7D5', ink: '#704115' }),
  4: Object.freeze({ accent: '#8065B5', soft: '#ECE4F4', ink: '#4D376B' }),
  5: Object.freeze({ accent: '#347B62', soft: '#DCECE5', ink: '#20513F' }),
  6: Object.freeze({ accent: '#42658D', soft: '#DEE7F0', ink: '#29425E' }),
  7: Object.freeze({ accent: '#C47B18', soft: '#F6EACB', ink: '#6E4A0C' }),
  8: Object.freeze({ accent: '#A43D35', soft: '#F1DEDB', ink: '#652621' }),
  9: Object.freeze({ accent: '#5D7766', soft: '#E2E9E3', ink: '#394B40' }),
})

export function typeTheme(typeId) {
  const normalized = Number(typeId)
  return Number.isInteger(normalized) && TYPE_THEME[normalized]
    ? TYPE_THEME[normalized]
    : FALLBACK_THEME
}
