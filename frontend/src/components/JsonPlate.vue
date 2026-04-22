<script setup lang="ts">
import { computed, ref } from 'vue'

// JsonPlate renders a JSON object/array as a compact stat-block card.
// Top-level keys become rows; values are rendered recursively:
//   - primitives (string/number/bool) inline
//   - arrays as chip rows
//   - nested objects as indented sub-plates
// There's a toggle to flip back to raw JSON for power users.

const props = defineProps<{
  data: unknown
  label?: string
}>()

const rawMode = ref(false)

const prettyJson = computed(() => {
  try {
    return JSON.stringify(props.data, null, 2)
  } catch {
    return String(props.data)
  }
})

type Primitive = string | number | boolean | null

function isPrimitive(v: unknown): v is Primitive {
  return v === null || ['string', 'number', 'boolean'].includes(typeof v)
}

function isPrimitiveArray(v: unknown): boolean {
  return Array.isArray(v) && v.every(isPrimitive)
}

function formatKey(k: string): string {
  // Pretty-print snake_case / camelCase keys without touching their meaning.
  const spaced = k.replace(/_/g, ' ').replace(/([a-z])([A-Z])/g, '$1 $2')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

function primitiveClass(v: Primitive): string {
  if (v === null) return 'nest-v-null'
  if (typeof v === 'boolean') return v ? 'nest-v-true' : 'nest-v-false'
  if (typeof v === 'number') return 'nest-v-num'
  return 'nest-v-str'
}

function primitiveDisplay(v: Primitive): string {
  if (v === null) return '—'
  if (typeof v === 'boolean') return v ? 'yes' : 'no'
  return String(v)
}

const isArrayTop = computed(() => Array.isArray(props.data))
const rows = computed<Array<[string, unknown]>>(() => {
  if (Array.isArray(props.data)) {
    return props.data.map((v, i) => [String(i + 1), v])
  }
  if (props.data && typeof props.data === 'object') {
    return Object.entries(props.data as Record<string, unknown>)
  }
  return []
})
</script>

<template>
  <div class="nest-plate">
    <div class="nest-plate-head">
      <div class="nest-plate-label">
        <v-icon size="14" class="mr-1">mdi-view-dashboard-variant-outline</v-icon>
        <span>{{ label || (isArrayTop ? 'list' : 'data') }}</span>
      </div>
      <button
        class="nest-plate-toggle"
        :title="rawMode ? 'Показать таблицей' : 'Показать JSON'"
        @click="rawMode = !rawMode"
      >
        <v-icon size="14">{{ rawMode ? 'mdi-table' : 'mdi-code-braces' }}</v-icon>
      </button>
    </div>

    <pre v-if="rawMode" class="nest-plate-raw">{{ prettyJson }}</pre>

    <div v-else class="nest-plate-body">
      <div
        v-for="[k, v] in rows"
        :key="k"
        class="nest-plate-row"
      >
        <div class="nest-plate-key">{{ formatKey(k) }}</div>
        <div class="nest-plate-val">
          <!-- Primitive -->
          <span
            v-if="isPrimitive(v)"
            :class="primitiveClass(v)"
          >{{ primitiveDisplay(v) }}</span>

          <!-- Flat list of primitives → chips -->
          <div v-else-if="isPrimitiveArray(v)" class="nest-plate-chips">
            <span
              v-for="(item, idx) in (v as Primitive[])"
              :key="idx"
              class="nest-plate-chip"
            >{{ primitiveDisplay(item) }}</span>
          </div>

          <!-- Nested object / mixed array → recurse -->
          <JsonPlate v-else :data="v" />
        </div>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.nest-plate {
  border: 1px solid var(--nest-border-subtle);
  border-left: 3px solid var(--nest-accent);
  border-radius: var(--nest-radius-sm);
  background: rgba(0, 0, 0, 0.12);
  font-family: var(--nest-font-body);
  overflow: hidden;
}

.nest-plate-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 5px 10px 5px 10px;
  background: rgba(0, 0, 0, 0.1);
  border-bottom: 1px solid var(--nest-border-subtle);
}
.nest-plate-label {
  display: flex;
  align-items: center;
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
}
.nest-plate-toggle {
  all: unset;
  cursor: pointer;
  padding: 2px 4px;
  color: var(--nest-text-muted);
  border-radius: 3px;
  &:hover { color: var(--nest-text); background: rgba(255,255,255,0.05); }
}

.nest-plate-raw {
  margin: 0;
  padding: 10px 12px;
  font-family: var(--nest-font-mono);
  font-size: 12px;
  line-height: 1.5;
  color: var(--nest-text-secondary);
  overflow-x: auto;
  white-space: pre;
}

.nest-plate-body {
  display: grid;
  grid-template-columns: minmax(90px, auto) 1fr;
  row-gap: 4px;
  column-gap: 10px;
  padding: 8px 12px;
}
.nest-plate-row {
  display: contents;
}
.nest-plate-key {
  font-family: var(--nest-font-mono);
  font-size: 11.5px;
  color: var(--nest-text-muted);
  padding: 3px 0;
  align-self: start;
}
.nest-plate-val {
  font-size: 13px;
  color: var(--nest-text);
  padding: 2px 0;
  min-width: 0;
  word-break: break-word;
}

.nest-plate-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.nest-plate-chip {
  padding: 1px 8px;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius-pill);
  font-size: 11.5px;
  color: var(--nest-text-secondary);
  background: rgba(255,255,255,0.04);
}

.nest-v-null { color: var(--nest-text-muted); font-style: italic; }
.nest-v-true { color: var(--nest-green); }
.nest-v-false { color: var(--nest-text-muted); }
.nest-v-num {
  color: var(--nest-text);
  font-family: var(--nest-font-mono);
  font-feature-settings: 'tnum' 1;
}
.nest-v-str {
  color: var(--nest-text);
}

// Nested plates: smaller + no separator banner
.nest-plate .nest-plate {
  border-left-width: 2px;
  background: rgba(0, 0, 0, 0.06);
  .nest-plate-head { display: none; }
  .nest-plate-body { padding: 6px 10px; }
}
</style>
