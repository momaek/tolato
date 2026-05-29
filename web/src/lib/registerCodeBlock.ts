import { setCustomComponents } from 'markstream-vue'
import CodeBlock from '@/components/chat/CodeBlock.vue'

// Side-effect module: imported for registration only. Runs exactly once
// (module init), so it never bumps markstream's custom-components revision on
// every ContentBlock mount — which would re-render all live renderers mid-stream.
//
// Swaps the heavy Monaco-based default code block for our lightweight
// shiki-highlighted one (distinct surface + copy button).
setCustomComponents({ code_block: CodeBlock })
