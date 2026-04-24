// Slash-commands framework. Client-side, registry-based.
//
// User types `/cmd arg1 arg2` in the composer and hits Send. The input
// is parsed here; if the first token matches a registered command, the
// handler fires and the message is NOT sent to the model (by default).
// Commands can insert into draft, trigger store actions, or queue
// another slash / attachment.
//
// This is intentionally NOT wired through the backend — slash commands
// are purely UI-side conveniences. The backend just sees whatever
// text the command produced (or nothing, if the command chose to
// suppress send).
//
// Registration API:
//
//   registerCommand({
//     name: 'continue',
//     aliases: ['c'],
//     description: 'Continue the last assistant message',
//     run: (ctx) => {
//       ctx.runAction('continue')  // fires Chat.vue's continue
//       return { suppressSend: true }
//     },
//   })
//
// The registry lives as a module-scope map so any module can register
// at import time. Chat.vue imports the core set below + listens for
// command palette shortcuts.

import type { Ref } from 'vue'

// ─── Types ─────────────────────────────────────────────────────────

/** Context passed to every command's run() handler. Lets commands
 *  mutate the draft, run high-level chat actions, or report a toast. */
export interface SlashContext {
  /** Raw positional args after the command name, whitespace-split. */
  args: string[]
  /** Everything after the command name as a single string (preserves
   *  spaces). Useful for /imagine /setvar where args are free-form. */
  rest: string
  /** Writable reference to the composer draft. Mutating this replaces
   *  the pending outbound text. */
  draft: Ref<string>
  /** Fire a high-level chat action — keys are resolved by Chat.vue's
   *  switch. Avoids passing the whole chats store into every command. */
  runAction: (name: SlashAction, payload?: any) => void | Promise<void>
  /** Emit a transient toast notification. Level drives color. */
  toast: (level: 'info' | 'success' | 'error', text: string) => void
}

/** High-level actions slash commands can trigger. Chat.vue wires
 *  each to its existing method (regenerate / continueAssistant / etc). */
export type SlashAction =
  | 'continue'
  | 'regenerate'
  | 'swipe-next'
  | 'swipe-prev'
  | 'delete-last'
  | 'hide-last'
  | 'show-last'
  | 'summarize'
  | 'imagine'
  | 'setvar'
  | 'getvar'
  | 'clear-draft'

/** Command result — handler can tell the framework whether to still
 *  send the (possibly modified) draft. `suppressSend: true` is the
 *  common case for action-style commands. */
export interface SlashResult {
  /** If true, don't send the message after the command runs (the
   *  command was the whole point of the user's input). */
  suppressSend?: boolean
  /** If set, replaces draft for the post-command send. */
  replaceDraft?: string
}

export interface SlashCommand {
  name: string
  aliases?: string[]
  description: string
  usage?: string
  run: (ctx: SlashContext) => SlashResult | Promise<SlashResult>
}

// ─── Registry ──────────────────────────────────────────────────────

const registry = new Map<string, SlashCommand>()

export function registerCommand(cmd: SlashCommand) {
  registry.set(cmd.name.toLowerCase(), cmd)
  for (const alias of cmd.aliases ?? []) {
    registry.set(alias.toLowerCase(), cmd)
  }
}

export function listCommands(): SlashCommand[] {
  // Canonical entries only (dedup aliases pointing at the same cmd).
  const seen = new Set<SlashCommand>()
  const out: SlashCommand[] = []
  for (const cmd of registry.values()) {
    if (!seen.has(cmd)) {
      seen.add(cmd)
      out.push(cmd)
    }
  }
  return out.sort((a, b) => a.name.localeCompare(b.name))
}

/** Look up a command by name or alias. */
export function findCommand(name: string): SlashCommand | undefined {
  return registry.get(name.toLowerCase())
}

// ─── Parse / dispatch ──────────────────────────────────────────────

/** Tries to dispatch a slash command. Returns undefined when the
 *  input isn't a slash command (doesn't start with `/` or unknown
 *  command — in which case the caller proceeds with normal send). */
export async function tryDispatch(input: string, partialCtx: Omit<SlashContext, 'args' | 'rest'>): Promise<SlashResult | undefined> {
  const trimmed = input.trim()
  if (!trimmed.startsWith('/')) return undefined
  const body = trimmed.slice(1)
  const spaceIdx = body.search(/\s/)
  const name = spaceIdx < 0 ? body : body.slice(0, spaceIdx)
  const rest = spaceIdx < 0 ? '' : body.slice(spaceIdx + 1).trim()
  const args = rest.length ? rest.split(/\s+/) : []

  const cmd = findCommand(name)
  if (!cmd) {
    partialCtx.toast('error', `Unknown command: /${name}`)
    return { suppressSend: true }
  }
  try {
    return await cmd.run({ ...partialCtx, args, rest })
  } catch (err) {
    partialCtx.toast('error', `/${name}: ${(err as Error).message}`)
    return { suppressSend: true }
  }
}

// ─── Built-in commands ─────────────────────────────────────────────
//
// Register on module load so Chat.vue doesn't need to import each.
// The commands here are "structural" — framework-level actions.
// Domain-specific commands (/imagine, /summarize) register themselves
// from their own modules.

registerCommand({
  name: 'help',
  aliases: ['?', 'commands'],
  description: 'List available commands',
  run: (ctx) => {
    const lines = listCommands().map(c => `/${c.name} — ${c.description}`)
    ctx.toast('info', lines.join('\n'))
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'clear',
  description: 'Clear the current draft',
  run: (ctx) => {
    ctx.draft.value = ''
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'continue',
  aliases: ['cont'],
  description: 'Extend the last assistant message',
  run: (ctx) => {
    ctx.runAction('continue')
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'regen',
  aliases: ['regenerate', 'retry'],
  description: 'Regenerate the last assistant message',
  run: (ctx) => {
    ctx.runAction('regenerate')
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'swipe',
  aliases: ['next'],
  description: 'Generate another variant of the last message',
  run: (ctx) => {
    ctx.runAction('swipe-next')
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'back',
  aliases: ['prev'],
  description: 'Show the previous swipe variant',
  run: (ctx) => {
    ctx.runAction('swipe-prev')
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'hide',
  description: 'Hide the last message from the UI (model still sees it)',
  run: (ctx) => {
    ctx.runAction('hide-last')
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'show',
  aliases: ['unhide'],
  description: 'Un-hide the last hidden message',
  run: (ctx) => {
    ctx.runAction('show-last')
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'delete',
  aliases: ['del', 'rm'],
  description: 'Delete the last message',
  run: (ctx) => {
    ctx.runAction('delete-last')
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'summarize',
  aliases: ['summary', 'mem'],
  description: 'Regenerate the rolling memory summary',
  run: (ctx) => {
    ctx.runAction('summarize')
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'setvar',
  description: 'Set a chat variable: /setvar name value',
  usage: '/setvar <name> <value>',
  run: (ctx) => {
    const name = ctx.args[0]
    const value = ctx.args.slice(1).join(' ')
    if (!name) {
      ctx.toast('error', '/setvar requires a name')
      return { suppressSend: true }
    }
    ctx.runAction('setvar', { name, value })
    ctx.toast('success', `${name} = ${value || '(empty)'}`)
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'getvar',
  description: 'Read a chat variable: /getvar name',
  usage: '/getvar <name>',
  run: (ctx) => {
    const name = ctx.args[0]
    if (!name) {
      ctx.toast('error', '/getvar requires a name')
      return { suppressSend: true }
    }
    ctx.runAction('getvar', { name })
    return { suppressSend: true }
  },
})

registerCommand({
  name: 'imagine',
  aliases: ['sd', 'image', 'img'],
  description: 'Generate an image from a prompt (via OpenRouter)',
  usage: '/imagine <prompt>',
  run: (ctx) => {
    const prompt = ctx.rest.trim()
    if (!prompt) {
      ctx.toast('error', '/imagine requires a prompt')
      return { suppressSend: true }
    }
    ctx.runAction('imagine', { prompt })
    return { suppressSend: true }
  },
})
