import { ref, watch } from 'vue'

export type Theme = 'dark' | 'light'

const theme = ref<Theme>((localStorage.getItem('theme') as Theme) || 'dark')

// Apply theme once at module load
function applyTheme(t: Theme) {
  const root = document.documentElement
  root.classList.remove('dark', 'light')
  root.classList.add(t)
  localStorage.setItem('theme', t)
}
applyTheme(theme.value)

// Single watcher registered at module level (not per call)
watch(theme, (t) => applyTheme(t))

export function useTheme() {
  function toggleTheme() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
  }

  function setTheme(t: Theme) {
    theme.value = t
  }

  return {
    theme,
    toggleTheme,
    setTheme,
  }
}
