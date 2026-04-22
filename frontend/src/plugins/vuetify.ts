import type { ThemeDefinition } from 'vuetify'

// Colours mirror WuApi's design system (see Obsidian vault:
// 99-WuNest-Plan/Tech-Choices.md → "Design tokens"). We intentionally keep
// the Vuetify theme minimal — most styling lives in SCSS custom properties
// under src/styles/global.scss.

const nestDark: ThemeDefinition = {
  dark: true,
  colors: {
    background: '#080808',
    surface: '#141414',
    'surface-variant': '#1a1a1a',
    primary: '#ef4444',       // WuApi red — primary CTA
    secondary: '#f7c948',     // gold accent
    error: '#ef4444',
    info: '#60a5fa',
    success: '#22c55e',
    warning: '#f7c948',
  },
}

const nestLight: ThemeDefinition = {
  dark: false,
  colors: {
    // Dossier-inspired: warm off-white paper + dark ink. Accent intentionally
    // matches the dark theme's coral so the brand identity carries across.
    background: '#FAFAF7',
    surface: '#F3F2EC',
    'surface-variant': '#FAFAF7',
    primary: '#ef4444',       // coral (synced with nestDark.primary)
    secondary: '#b45309',     // bronze (decorative, light-friendly)
    error: '#ef4444',
    info: '#1e6fd9',
    success: '#4c8b4f',
    warning: '#c07a1f',
  },
}

export const theme = {
  defaultTheme: 'nestDark',
  themes: {
    nestDark,
    nestLight,
  },
}
