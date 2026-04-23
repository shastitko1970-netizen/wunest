<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { OpenAIBundleData } from '@/api/presets'

/**
 * AdvancedBundlePanel — misc ST preset flags that don't fit in Sampler
 * / Prompts / Regex tabs. Each control is a thin v-model wrapper that
 * writes back to the bundle. Unknown fields (kept in bundle.data by the
 * round-trip spread) remain untouched.
 *
 * Grouped into sections so users can skip the subgroups that don't apply
 * to their provider:
 *   - Prefill / continuation — Claude / Gemini continuation hints
 *   - Provider behavior — squash, sysprompt handling
 *   - Multimodal / reasoning — image/video/function-call toggles + thinking effort
 *   - Format — wi/scenario/personality string formats
 */

const { t } = useI18n()

const props = defineProps<{
  modelValue: OpenAIBundleData
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: OpenAIBundleData): void
}>()

function update<K extends keyof OpenAIBundleData>(key: K, value: OpenAIBundleData[K]) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}
</script>

<template>
  <div class="nest-adv-bundle">
    <v-expansion-panels variant="accordion" multiple>
      <!-- ── Prefill / continuation ─── -->
      <v-expansion-panel>
        <v-expansion-panel-title>
          <v-icon size="16" class="mr-2">mdi-pencil-plus-outline</v-icon>
          {{ t('presets.advanced.sectionPrefill') }}
        </v-expansion-panel-title>
        <v-expansion-panel-text>
          <div class="nest-field">
            <label class="nest-field-label">
              {{ t('presets.advanced.assistantPrefill') }}
              <v-tooltip location="top" :text="t('presets.advanced.assistantPrefillHint')">
                <template #activator="{ props: p }">
                  <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                </template>
              </v-tooltip>
            </label>
            <v-textarea
              :model-value="modelValue.assistant_prefill"
              rows="3" auto-grow density="compact" hide-details
              @update:model-value="v => update('assistant_prefill', v)"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.advanced.continuePrefill') }}</label>
            <v-switch
              :model-value="modelValue.continue_prefill"
              color="primary" hide-details density="compact"
              @update:model-value="v => update('continue_prefill', v)"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.advanced.continueNudge') }}</label>
            <v-textarea
              :model-value="modelValue.continue_nudge_prompt"
              rows="2" auto-grow density="compact" hide-details
              @update:model-value="v => update('continue_nudge_prompt', v)"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.advanced.impersonationPrompt') }}</label>
            <v-textarea
              :model-value="modelValue.impersonation_prompt"
              rows="2" auto-grow density="compact" hide-details
              @update:model-value="v => update('impersonation_prompt', v)"
            />
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>

      <!-- ── Provider behavior ─── -->
      <v-expansion-panel>
        <v-expansion-panel-title>
          <v-icon size="16" class="mr-2">mdi-cog-outline</v-icon>
          {{ t('presets.advanced.sectionProvider') }}
        </v-expansion-panel-title>
        <v-expansion-panel-text>
          <div class="nest-field">
            <v-switch
              :model-value="modelValue.squash_system_messages ?? false"
              color="primary" hide-details density="compact"
              :label="t('presets.advanced.squashSystem')"
              @update:model-value="v => update('squash_system_messages', v)"
            />
            <div class="nest-field-hint">{{ t('presets.advanced.squashSystemHint') }}</div>
          </div>
          <div class="nest-field">
            <v-switch
              :model-value="modelValue.claude_use_sysprompt ?? false"
              color="primary" hide-details density="compact"
              :label="t('presets.advanced.claudeUseSysprompt')"
              @update:model-value="v => update('claude_use_sysprompt', v)"
            />
          </div>
          <div class="nest-field">
            <v-switch
              :model-value="modelValue.use_makersuite_sysprompt ?? false"
              color="primary" hide-details density="compact"
              :label="t('presets.advanced.useMakersuiteSysprompt')"
              @update:model-value="v => update('use_makersuite_sysprompt', v)"
            />
          </div>
          <div class="nest-field">
            <v-switch
              :model-value="modelValue.stream_openai ?? true"
              color="primary" hide-details density="compact"
              :label="t('presets.advanced.streamOpenAI')"
              @update:model-value="v => update('stream_openai', v)"
            />
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>

      <!-- ── Multimodal / reasoning ─── -->
      <v-expansion-panel>
        <v-expansion-panel-title>
          <v-icon size="16" class="mr-2">mdi-image-multiple-outline</v-icon>
          {{ t('presets.advanced.sectionMultimodal') }}
        </v-expansion-panel-title>
        <v-expansion-panel-text>
          <div class="nest-field">
            <v-switch
              :model-value="modelValue.image_inlining ?? false"
              color="primary" hide-details density="compact"
              :label="t('presets.advanced.imageInlining')"
              @update:model-value="v => update('image_inlining', v)"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.advanced.inlineImageQuality') }}</label>
            <v-select
              :model-value="modelValue.inline_image_quality || 'auto'"
              :items="[
                { value: 'auto',   title: 'auto' },
                { value: 'low',    title: 'low' },
                { value: 'high',   title: 'high' },
              ]"
              density="compact" hide-details
              @update:model-value="v => update('inline_image_quality', v)"
            />
          </div>
          <div class="nest-field">
            <v-switch
              :model-value="modelValue.video_inlining ?? false"
              color="primary" hide-details density="compact"
              :label="t('presets.advanced.videoInlining')"
              @update:model-value="v => update('video_inlining', v)"
            />
          </div>
          <div class="nest-field">
            <v-switch
              :model-value="modelValue.request_images ?? false"
              color="primary" hide-details density="compact"
              :label="t('presets.advanced.requestImages')"
              @update:model-value="v => update('request_images', v)"
            />
          </div>
          <div class="nest-field">
            <v-switch
              :model-value="modelValue.function_calling ?? false"
              color="primary" hide-details density="compact"
              :label="t('presets.advanced.functionCalling')"
              @update:model-value="v => update('function_calling', v)"
            />
          </div>
          <v-divider class="my-3" />
          <div class="nest-field">
            <v-switch
              :model-value="modelValue.show_thoughts ?? false"
              color="primary" hide-details density="compact"
              :label="t('presets.advanced.showThoughts')"
              @update:model-value="v => update('show_thoughts', v)"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">
              {{ t('presets.advanced.reasoningEffort') }}
              <v-tooltip location="top" :text="t('presets.advanced.reasoningEffortHint')">
                <template #activator="{ props: p }">
                  <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                </template>
              </v-tooltip>
            </label>
            <v-select
              :model-value="modelValue.reasoning_effort || ''"
              :items="[
                { value: '',       title: t('presets.advanced.reasoningDefault') },
                { value: 'low',    title: 'low' },
                { value: 'medium', title: 'medium' },
                { value: 'high',   title: 'high' },
              ]"
              density="compact" hide-details
              @update:model-value="v => update('reasoning_effort', v)"
            />
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>

      <!-- ── Format / output shaping ─── -->
      <v-expansion-panel>
        <v-expansion-panel-title>
          <v-icon size="16" class="mr-2">mdi-format-align-left</v-icon>
          {{ t('presets.advanced.sectionFormat') }}
        </v-expansion-panel-title>
        <v-expansion-panel-text>
          <div class="nest-field">
            <v-switch
              :model-value="modelValue.wrap_in_quotes ?? false"
              color="primary" hide-details density="compact"
              :label="t('presets.advanced.wrapInQuotes')"
              @update:model-value="v => update('wrap_in_quotes', v)"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.advanced.namesBehavior') }}</label>
            <v-select
              :model-value="modelValue.names_behavior ?? 0"
              :items="[
                { value: 0, title: t('presets.advanced.namesNone') },
                { value: 1, title: t('presets.advanced.namesOnly') },
                { value: 2, title: t('presets.advanced.namesAll') },
              ]"
              density="compact" hide-details
              @update:model-value="v => update('names_behavior', v)"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.advanced.wiFormat') }}</label>
            <v-text-field
              :model-value="modelValue.wi_format"
              density="compact" hide-details
              @update:model-value="v => update('wi_format', v)"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.advanced.scenarioFormat') }}</label>
            <v-text-field
              :model-value="modelValue.scenario_format"
              density="compact" hide-details
              @update:model-value="v => update('scenario_format', v)"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.advanced.personalityFormat') }}</label>
            <v-text-field
              :model-value="modelValue.personality_format"
              density="compact" hide-details
              @update:model-value="v => update('personality_format', v)"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.advanced.sendIfEmpty') }}</label>
            <v-text-field
              :model-value="modelValue.send_if_empty"
              density="compact" hide-details
              @update:model-value="v => update('send_if_empty', v)"
            />
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>
    </v-expansion-panels>
  </div>
</template>

<style lang="scss" scoped>
.nest-adv-bundle {
  :deep(.v-expansion-panel-title) {
    min-height: 42px !important;
    padding: 8px 14px !important;
    font-size: 13px;
  }
  :deep(.v-expansion-panel-text__wrapper) {
    padding: 12px 14px 16px !important;
  }
  :deep(.v-expansion-panel) {
    background: var(--nest-surface) !important;
    border: 1px solid var(--nest-border-subtle) !important;
    border-radius: var(--nest-radius-sm) !important;
    margin-top: 6px;
  }
}

.nest-field { margin-bottom: 10px; min-width: 0; }
.nest-field-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--nest-text-secondary);
  margin-bottom: 4px;
}
.nest-field-hint {
  font-size: 11px;
  color: var(--nest-text-muted);
  margin-top: 2px;
}
.nest-hint-icon {
  color: var(--nest-text-muted);
  cursor: help;
  opacity: 0.7;
  &:hover { opacity: 1; color: var(--nest-accent); }
}
</style>
