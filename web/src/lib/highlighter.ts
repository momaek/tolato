import { getSingletonHighlighter } from 'shiki'
import type { Highlighter } from 'shiki'

// Two GitHub themes, one per app theme. Languages are NOT pre-loaded — shiki
// dynamic-imports each grammar on first use, so the initial chunk stays small
// and we only pay for languages that actually appear in chat.
const LIGHT_THEME = 'github-light'
const DARK_THEME = 'github-dark'

let highlighterPromise: Promise<Highlighter> | null = null

function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = getSingletonHighlighter({
      themes: [LIGHT_THEME, DARK_THEME],
      langs: [],
    })
  }
  return highlighterPromise
}

/**
 * Render a fenced code block to themed HTML. Unknown / unsupported languages
 * fall back to plain `text` highlighting rather than throwing, so an exotic
 * fence label never breaks the message. Returns shiki's `<pre class="shiki">`
 * markup; the caller strips its background to blend with the surrounding card.
 */
export async function highlightCode(
  code: string,
  language: string,
  dark: boolean,
): Promise<string> {
  const hl = await getHighlighter()
  let lang = (language || '').trim().toLowerCase() || 'text'

  if (lang !== 'text' && !hl.getLoadedLanguages().includes(lang)) {
    try {
      await hl.loadLanguage(lang as Parameters<Highlighter['loadLanguage']>[0])
    } catch {
      lang = 'text'
    }
  }

  return hl.codeToHtml(code, {
    lang,
    theme: dark ? DARK_THEME : LIGHT_THEME,
  })
}
