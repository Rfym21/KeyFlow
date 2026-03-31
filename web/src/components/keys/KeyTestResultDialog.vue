<script setup lang="ts">
import { keysApi } from "@/api/keys";
import { maskKey } from "@/utils/display";
import {
  CheckmarkCircle,
  Close,
  AlertCircleOutline,
  ChevronDown,
  ChevronUp,
} from "@vicons/ionicons5";
import {
  NButton,
  NCard,
  NCheckbox,
  NIcon,
  NModal,
  NTooltip,
} from "naive-ui";
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";

interface TestResultItem {
  key_value: string;
  is_valid: boolean;
  error: string;
  status_code: number;
}

interface Props {
  show: boolean;
  results: TestResultItem[];
  totalDuration: number;
  groupId: number;
}

interface Emits {
  (e: "update:show", value: boolean): void;
  (e: "refresh"): void;
}

const props = defineProps<Props>();
const emit = defineEmits<Emits>();
const { t } = useI18n();

const selectedKeys = ref<Set<string>>(new Set());
const expandedGroups = ref<Set<string>>(new Set());
const operationLoading = ref(false);

watch(
  () => props.show,
  (show) => {
    if (show) {
      selectedKeys.value = new Set();
      // 默认展开所有分组
      expandedGroups.value = new Set(groupedResults.value.map((g) => g.groupKey));
    }
  }
);

/**
 * 按状态码分组测试结果
 */
const groupedResults = computed(() => {
  const groups = new Map<
    string,
    { groupKey: string; label: string; type: "success" | "error"; statusCode: number; items: TestResultItem[] }
  >();

  for (const item of props.results) {
    if (item.is_valid) {
      const key = "success";
      if (!groups.has(key)) {
        groups.set(key, {
          groupKey: key,
          label: t("keys.testResultSuccess"),
          type: "success",
          statusCode: 200,
          items: [],
        });
      }
      groups.get(key)!.items.push(item);
    } else {
      const code = item.status_code || 0;
      const key = `error_${code}`;
      if (!groups.has(key)) {
        groups.set(key, {
          groupKey: key,
          label: code > 0 ? t("keys.testResultStatusCode", { code }) : t("keys.testResultUnknownError"),
          type: "error",
          statusCode: code,
          items: [],
        });
      }
      groups.get(key)!.items.push(item);
    }
  }

  // 成功组在前，错误组按状态码排序
  const result = Array.from(groups.values());
  result.sort((a, b) => {
    if (a.type === "success" && b.type !== "success") return -1;
    if (a.type !== "success" && b.type === "success") return 1;
    return a.statusCode - b.statusCode;
  });

  return result;
});

function handleClose() {
  emit("update:show", false);
}

function toggleGroup(groupKey: string) {
  const newExpanded = new Set(expandedGroups.value);
  if (newExpanded.has(groupKey)) {
    newExpanded.delete(groupKey);
  } else {
    newExpanded.add(groupKey);
  }
  expandedGroups.value = newExpanded;
}

function isGroupExpanded(groupKey: string): boolean {
  return expandedGroups.value.has(groupKey);
}

function toggleSelectAll(items: TestResultItem[]) {
  const newSelected = new Set(selectedKeys.value);
  const allSelected = items.every((item) => newSelected.has(item.key_value));

  for (const item of items) {
    if (allSelected) {
      newSelected.delete(item.key_value);
    } else {
      newSelected.add(item.key_value);
    }
  }
  selectedKeys.value = newSelected;
}

function isGroupAllSelected(items: TestResultItem[]): boolean {
  return items.length > 0 && items.every((item) => selectedKeys.value.has(item.key_value));
}

function isGroupIndeterminate(items: TestResultItem[]): boolean {
  const selectedCount = items.filter((item) => selectedKeys.value.has(item.key_value)).length;
  return selectedCount > 0 && selectedCount < items.length;
}

function toggleKeySelection(keyValue: string) {
  const newSelected = new Set(selectedKeys.value);
  if (newSelected.has(keyValue)) {
    newSelected.delete(keyValue);
  } else {
    newSelected.add(keyValue);
  }
  selectedKeys.value = newSelected;
}

const selectedCount = computed(() => selectedKeys.value.size);

/**
 * 禁用选中的 Key
 */
async function disableSelected() {
  if (selectedKeys.value.size === 0 || operationLoading.value) return;

  operationLoading.value = true;
  try {
    const keysText = Array.from(selectedKeys.value).join("\n");
    await keysApi.deleteKeys(props.groupId, keysText);
    window.$message.success(t("keys.testResultOperationSuccess", { count: selectedKeys.value.size }));
    selectedKeys.value = new Set();
    emit("refresh");
    handleClose();
  } catch (error) {
    console.error("Disable selected keys failed", error);
  } finally {
    operationLoading.value = false;
  }
}

/**
 * 格式化耗时
 */
function formatDuration(ms: number): string {
  if (ms < 0) return "0ms";
  const minutes = Math.floor(ms / 60000);
  const seconds = Math.floor((ms % 60000) / 1000);
  const milliseconds = ms % 1000;
  let result = "";
  if (minutes > 0) result += `${minutes}m`;
  if (seconds > 0) result += `${seconds}s`;
  if (milliseconds > 0 || result === "") result += `${milliseconds}ms`;
  return result;
}
</script>

<template>
  <n-modal :show="show" @update:show="handleClose" class="form-modal">
    <n-card
      style="width: 700px; max-height: 80vh"
      :title="t('keys.testResultTitle')"
      :bordered="false"
      size="huge"
      role="dialog"
      aria-modal="true"
    >
      <template #header-extra>
        <span class="duration-badge">{{ t("keys.testResultDuration", { duration: formatDuration(totalDuration) }) }}</span>
        <n-button quaternary circle @click="handleClose" style="margin-left: 8px">
          <template #icon>
            <n-icon :component="Close" />
          </template>
        </n-button>
      </template>

      <div class="result-groups" style="max-height: calc(80vh - 160px); overflow-y: auto">
        <div v-for="group in groupedResults" :key="group.groupKey" class="result-group">
          <!-- 分组标题行 -->
          <div class="group-header" @click="toggleGroup(group.groupKey)">
            <div class="group-header-left">
              <n-checkbox
                :checked="isGroupAllSelected(group.items)"
                :indeterminate="isGroupIndeterminate(group.items)"
                @update:checked="toggleSelectAll(group.items)"
                @click.stop
              />
              <n-icon
                :component="group.type === 'success' ? CheckmarkCircle : AlertCircleOutline"
                :style="{ color: group.type === 'success' ? '#18a058' : '#d03050' }"
                size="18"
              />
              <span class="group-label">{{ group.label }}</span>
              <span class="group-count">{{ t("keys.testResultCount", { count: group.items.length }) }}</span>
            </div>
            <n-icon
              :component="isGroupExpanded(group.groupKey) ? ChevronUp : ChevronDown"
              size="18"
              style="color: var(--text-secondary)"
            />
          </div>

          <!-- 分组内 Key 列表 -->
          <div v-if="isGroupExpanded(group.groupKey)" class="group-items">
            <div
              v-for="item in group.items"
              :key="item.key_value"
              class="key-item"
              @click="toggleKeySelection(item.key_value)"
            >
              <n-checkbox
                :checked="selectedKeys.has(item.key_value)"
                @update:checked="toggleKeySelection(item.key_value)"
                @click.stop
              />
              <span class="key-value">{{ maskKey(item.key_value) }}</span>
              <n-tooltip v-if="item.error" trigger="hover" :style="{ maxWidth: '400px' }">
                <template #trigger>
                  <span class="error-hint">
                    <n-icon :component="AlertCircleOutline" size="14" />
                  </span>
                </template>
                {{ item.error }}
              </n-tooltip>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="dialog-footer">
          <span v-if="selectedCount > 0" class="selected-info">
            {{ t("common.selected") }} {{ selectedCount }}
          </span>
          <div class="footer-actions">
            <n-button @click="handleClose">{{ t("common.close") }}</n-button>
            <n-button
              type="error"
              :disabled="selectedCount === 0"
              :loading="operationLoading"
              @click="disableSelected"
            >
              {{ t("keys.testResultDeleteSelected") }}
            </n-button>
          </div>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.form-modal {
  --n-color: rgba(255, 255, 255, 0.95);
}

:deep(.n-card-header) {
  border-bottom: 1px solid var(--border-color);
  padding: 10px 20px;
}

:deep(.n-card__content) {
  padding: 12px 20px;
}

:deep(.n-card__footer) {
  border-top: 1px solid var(--border-color);
  padding: 10px 15px;
}

.duration-badge {
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  padding: 2px 8px;
  border-radius: 4px;
}

.result-group {
  margin-bottom: 8px;
  border: 1px solid var(--border-color);
  border-radius: 6px;
  overflow: hidden;
}

.group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  background: var(--bg-secondary);
  cursor: pointer;
  user-select: none;
}

.group-header:hover {
  background: var(--bg-hover);
}

.group-header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.group-label {
  font-weight: 600;
  font-size: 14px;
}

.group-count {
  font-size: 12px;
  color: var(--text-secondary);
}

.group-items {
  padding: 4px 0;
}

.key-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px 6px 20px;
  cursor: pointer;
  font-size: 13px;
}

.key-item:hover {
  background: var(--bg-hover);
}

.key-value {
  font-family: "SF Mono", "Monaco", "Inconsolata", "Fira Code", monospace;
  color: var(--text-primary);
}

.error-hint {
  color: #d03050;
  display: flex;
  align-items: center;
  cursor: help;
}

.dialog-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.selected-info {
  font-size: 13px;
  color: var(--text-secondary);
}

.footer-actions {
  display: flex;
  gap: 8px;
}
</style>
