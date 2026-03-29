import type {
  AssistantTurn,
  ContentBlock,
  TimelineRow,
  ToolUseBlock,
  Turn,
} from '@/shared/types/console'

function getOrCreateAssistantTurn(turns: Turn[], createdAt: string): AssistantTurn {
  const last = turns[turns.length - 1]
  if (last && last.type === 'assistant') {
    return last
  }
  const turn: AssistantTurn = {
    type: 'assistant',
    id: `turn-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
    createdAt,
    status: 'completed',
    blocks: [],
  }
  turns.push(turn)
  return turn
}

function findLastPendingToolBlock(blocks: ContentBlock[]): ToolUseBlock | undefined {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const block = blocks[i]
    if (block.type === 'tool_use' && !block.result) {
      return block
    }
  }
  return undefined
}

export function assembleRowsIntoTurns(rows: TimelineRow[]): Turn[] {
  const turns: Turn[] = []

  for (const row of rows) {
    switch (row.kind) {
      case 'user_message':
        turns.push({
          type: 'user',
          id: row.id,
          createdAt: row.createdAt,
          text: row.text,
        })
        break

      case 'assistant_text': {
        const turn = getOrCreateAssistantTurn(turns, row.createdAt)
        turn.blocks.push({
          type: 'text',
          text: row.markdown,
          rowId: row.id,
        })
        break
      }

      case 'tool_call_meta': {
        const turn = getOrCreateAssistantTurn(turns, row.createdAt)
        turn.blocks.push({
          type: 'tool_use',
          toolName: row.label.replace(/\(.*$/, '').trim(),
          argsPreview: row.label.includes('(')
            ? row.label.slice(row.label.indexOf('(') + 1, -1)
            : undefined,
          callRowId: row.id,
        })
        break
      }

      case 'tool_result_meta': {
        const turn = getOrCreateAssistantTurn(turns, row.createdAt)
        const pending = findLastPendingToolBlock(turn.blocks)
        if (pending) {
          pending.result = {
            label: row.label,
            tone: row.tone,
            rowId: row.id,
          }
        }
        break
      }
    }
  }

  return turns
}
