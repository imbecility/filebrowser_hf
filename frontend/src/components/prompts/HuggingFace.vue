<template>
  <div class="card floating" id="hf-upload">
    <template v-if="phase === 'config'">
      <div class="card-title">
        <h2>{{ t("huggingface.title") }}</h2>
      </div>

      <div class="card-content">
        <template v-if="!hasServerToken">
          <p>{{ t("huggingface.tokenLabel") }}</p>
          <input
            class="input input--block"
            type="password"
            v-model.trim="token"
            :placeholder="t('huggingface.tokenPlaceholder')"
            autocomplete="off"
          />
        </template>
        <p>{{ t("huggingface.repoIdLabel") }}</p>
        <input
          id="hf-repo-input"
          class="input input--block"
          type="text"
          v-model.trim="repoId"
          list="hf-repo-datalist"
          :placeholder="t('huggingface.repoIdPlaceholder')"
          @keyup.enter="isValid && submit()"
        />
        <datalist id="hf-repo-datalist">
          <option v-for="r in repoHistory" :key="r" :value="r" />
        </datalist>
        
        <p>{{ t("huggingface.repoTypeLabel") }}</p>
        <select class="input input--block" v-model="repoType">
          <option value="model">Model</option>
          <option value="dataset">Dataset</option>
          <option value="space">Space</option>
        </select>
        
        <p>
          {{ t("huggingface.pathPrefixLabel") }}
          <small class="hf-hint">{{ t("huggingface.pathPrefixHint") }}</small>
        </p>
        <input
          class="input input--block"
          type="text"
          v-model.trim="pathInRepoPrefix"
          :placeholder="t('huggingface.pathPrefixPlaceholder')"
        />
        
        <p>{{ t("huggingface.commitMsgLabel") }}</p>
        <input
          class="input input--block"
          type="text"
          v-model.trim="commitMessage"
          :placeholder="t('huggingface.commitMsgPlaceholder')"
        />
        
        <p>{{ t("huggingface.filesToUpload", { count: selectedPaths.length }) }}</p>
        <ul class="hf-file-list">
          <li
            v-for="p in selectedPaths"
            :key="p"
            class="hf-file-list-item"
          >
            <i class="material-icons">{{ p.endsWith("/") ? "folder" : "insert_drive_file" }}</i>
            <span>{{ p }}</span>
          </li>
        </ul>
      </div>

      <div class="card-action">
        <button
          class="button button--flat button--grey"
          @click="layoutStore.closeHovers()"
        >{{ t("buttons.cancel") }}</button>
        <button
          class="button button--flat button--blue"
          :disabled="!isValid"
          @click="submit"
        >{{ t("huggingface.uploadBtn") }}</button>
      </div>
    </template>
    
    <template v-else-if="phase === 'uploading'">
      <div class="card-title">
        <h2>{{ t("huggingface.uploading") }}</h2>
        <p class="hf-subtitle">{{ repoId }}</p>
      </div>

      <div class="card-content hf-uploading-content">
        
        <div v-if="messages.length" class="hf-messages">
          <p v-for="(msg, i) in messages" :key="i" class="hf-msg">
            <i class="material-icons">info_outline</i>
            {{ msg }}
          </p>
        </div>
        
        <div class="hf-file-progress-list">
          <div
            v-for="entry in fileEntries"
            :key="entry.path"
            class="hf-file-progress-item"
          >
            <div class="hf-file-progress-header">
              <i
                class="material-icons hf-status-icon"
                :class="entry.status === 'done' ? 'hf-done' : 'hf-active'"
              >{{ entry.status === 'done' ? 'check_circle' : 'upload' }}</i>
              <span class="hf-file-progress-path">{{ entry.path }}</span>
              <span class="hf-file-progress-bytes">
                {{ formatBytes(entry.bytesDone) }} / {{ formatBytes(entry.totalBytes) }}
              </span>
            </div>
            <div class="hf-progress-track">
              <div
                class="hf-progress-fill"
                :style="{ width: pct(entry) + '%' }"
              />
            </div>
          </div>
        </div>
      </div>

      <div class="card-action">
        <button
          class="button button--flat button--grey"
          @click="abortUpload"
        >{{ t("huggingface.abort") }}</button>
      </div>
    </template>
    
    <template v-else-if="phase === 'done'">
      <div class="card-title">
        <h2>{{ t("huggingface.complete") }}</h2>
      </div>

      <div class="card-content">
        <p>{{ t("huggingface.completeMsg") }}</p>
        <a
          class="hf-repo-link"
          :href="hfRepoURL"
          target="_blank"
          rel="noopener noreferrer"
        >
          <i class="material-icons">open_in_new</i>
          {{ hfRepoURL }}
        </a>
      </div>

      <div class="card-action">
        <button
          class="button button--flat button--blue"
          @click="layoutStore.closeHovers()"
        >{{ t("buttons.close") }}</button>
      </div>
    </template>
    
    <template v-else-if="phase === 'error'">
      <div class="card-title">
        <h2>{{ t("huggingface.failed") }}</h2>
      </div>

      <div class="card-content">
        <pre class="hf-error-msg">{{ errorMessage }}</pre>
      </div>

      <div class="card-action">
        <button
          class="button button--flat button--grey"
          @click="phase = 'config'"
        >{{ t("huggingface.retry") }}</button>
        <button
          class="button button--flat button--blue"
          @click="layoutStore.closeHovers()"
        >{{ t("buttons.close") }}</button>
      </div>
    </template>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRoute } from "vue-router";
import { useLayoutStore } from "@/stores/layout";
import { useFileStore } from "@/stores/file";
import { useI18n } from "vue-i18n";
import { removePrefix } from "@/api/utils";
import * as hfApi from "@/api/huggingface";
import type { HFProgressEvent, RepoType } from "@/api/huggingface";

const layoutStore = useLayoutStore();
const fileStore = useFileStore();
const route = useRoute();
const { t } = useI18n();

interface FileEntry {
  path: string;
  totalBytes: number;
  bytesDone: number;
  status: "uploading" | "done";
}

type Phase = "config" | "uploading" | "done" | "error";

const phase = ref<Phase>("config");
const hasServerToken = ref(true);

const token = ref("");
const repoId = ref("");
const repoType = ref<RepoType>("model");
const pathInRepoPrefix = ref("");
const commitMessage = ref("");

const repoHistory = ref<string[]>([]);

const fileEntries = ref<FileEntry[]>([]);
const messages = ref<string[]>([]);
const errorMessage = ref("");

const abortController = ref<AbortController | null>(null);


/**
 * VFS paths of selected files/folders.
 * Follows the Share.vue pattern: selectedCount > 0 → items[selected[i]].url,
 * otherwise - the current path of the router (viewing a single file).
 */
const selectedPaths = computed<string[]>(() => {
  if (!fileStore.isListing) {
    return [removePrefix(route.path) || "/"];
  }
  if (fileStore.selectedCount === 0) return [];
  return fileStore.selected.map((i) =>
      removePrefix(fileStore.req!.items[i].url)
  );
});

const isValid = computed(
  () =>
    repoId.value.trim().length > 0 &&
    selectedPaths.value.length > 0 &&
    (hasServerToken.value || token.value.trim().length > 0)
);

const hfRepoURL = computed(() => {
  const base = "https://huggingface.co";
  if (repoType.value === "dataset") return `${base}/datasets/${repoId.value}`;
  if (repoType.value === "space") return `${base}/spaces/${repoId.value}`;
  return `${base}/${repoId.value}`;
});


const formatBytes = (bytes: number): string => {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
};

const pct = (entry: FileEntry): number =>
  entry.totalBytes > 0
    ? Math.min(100, Math.round((entry.bytesDone / entry.totalBytes) * 100))
    : 0;

//  localStorage 
const LS_TOKEN = "hf_token";
const LS_REPOS = "hf_repos";
const MAX_HISTORY = 20;

function loadFromStorage(): void {
  const savedToken = localStorage.getItem(LS_TOKEN);
  if (savedToken) token.value = savedToken;

  try {
    const raw = localStorage.getItem(LS_REPOS);
    if (raw) repoHistory.value = JSON.parse(raw);
  } catch {
    repoHistory.value = [];
  }
}

function saveRepoToHistory(repo: string): void {
  const updated = [
    repo,
    ...repoHistory.value.filter((r) => r !== repo),
  ].slice(0, MAX_HISTORY);
  repoHistory.value = updated;
  localStorage.setItem(LS_REPOS, JSON.stringify(updated));
}

function saveTokenToStorage(): void {
  if (token.value) localStorage.setItem(LS_TOKEN, token.value);
}

function handleProgress(event: HFProgressEvent): void {
  switch (event.action) {
    case "discover":
      if (event.message) messages.value.push(event.message);
      break;

    case "start": {
      if (!event.path) break;
      if (!fileEntries.value.some((e) => e.path === event.path)) {
        fileEntries.value.push({
          path: event.path,
          totalBytes: event.totalBytes ?? 0,
          bytesDone: 0,
          status: "uploading",
        });
      }
      break;
    }

    case "update": {
      const entry = fileEntries.value.find((e) => e.path === event.path);
      if (entry) entry.bytesDone = event.bytesDone ?? 0;
      break;
    }

    case "done": {
      const entry = fileEntries.value.find((e) => e.path === event.path);
      if (entry) {
        entry.status = "done";
        entry.bytesDone = entry.totalBytes;
      }
      break;
    }

    case "complete":
      phase.value = "done";
      break;

    case "error":
      errorMessage.value = event.message ?? t("huggingface.unknownError");
      phase.value = "error";
      break;
  }
}

async function submit(): Promise<void> {
  if (!isValid.value) return;

  if (!hasServerToken.value) saveTokenToStorage();
  saveRepoToHistory(repoId.value);

  fileEntries.value = [];
  messages.value = [];
  errorMessage.value = "";
  phase.value = "uploading";

  abortController.value = new AbortController();

  try {
    await hfApi.upload(
      {
        repoId: repoId.value,
        repoType: repoType.value,
        paths: selectedPaths.value,
        pathInRepoPrefix: pathInRepoPrefix.value || undefined,
        commitMessage: commitMessage.value || undefined,
        token: hasServerToken.value ? undefined : token.value,
      },
      handleProgress,
      abortController.value.signal
    );

    if (phase.value === "uploading") phase.value = "done";
  } catch (e: any) {
    if (e?.name === "AbortError") {
      layoutStore.closeHovers();
      return;
    }

    errorMessage.value = e?.message ?? t("huggingface.unknownError");
    phase.value = "error";
  }
  }


function abortUpload(): void {
  abortController.value?.abort();
}

onMounted(async () => {
  loadFromStorage();

  // Autocomplete pathInRepoPrefix
  // If exactly one folder is selected, substitute its name as a prefix,
  // so that the content does not end up at the root of the repository.
  if (fileStore.isListing && fileStore.selectedCount === 1) {
    const item = fileStore.req!.items[fileStore.selected[0]];
    if (item.isDir) {
      pathInRepoPrefix.value = item.name;
    }
  } else if (!fileStore.isListing && fileStore.req?.isDir) {
    pathInRepoPrefix.value = fileStore.req.name;
  }

  try {
    hasServerToken.value = await hfApi.getTokenStatus();
  } catch {
    hasServerToken.value = false;
  }
});
</script>

<style scoped>
#hf-upload {
  min-width: 460px;
  max-width: 580px;
}

.hf-subtitle {
  font-size: 0.85em;
  color: var(--textSecondary, #666);
  margin: 0.15em 0 0;
  font-weight: normal;
  font-family: monospace;
}

.hf-hint {
  color: var(--textSecondary, #888);
  font-weight: normal;
  margin-left: 0.4em;
}

.hf-file-list {
  list-style: none;
  padding: 0;
  margin: 0.4em 0 0.8em;
  max-height: 120px;
  overflow-y: auto;
  border: 1px solid var(--divider, #e0e0e0);
  border-radius: 4px;
}

.hf-file-list-item {
  display: flex;
  align-items: center;
  gap: 0.4em;
  padding: 0.3em 0.6em;
  font-size: 0.85em;
  font-family: monospace;
  border-bottom: 1px solid var(--divider, #e0e0e0);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.hf-file-list-item:last-child { border-bottom: none; }
.hf-file-list-item .material-icons { font-size: 1rem; flex-shrink: 0; }

.hf-uploading-content {
  max-height: 320px;
  overflow-y: auto;
}

.hf-messages { margin-bottom: 0.75em; }

.hf-msg {
  display: flex;
  align-items: center;
  gap: 0.35em;
  font-size: 0.82em;
  color: var(--textSecondary, #666);
  margin: 0.2em 0;
}
.hf-msg .material-icons { font-size: 0.95rem; }

.hf-file-progress-list {
  display: flex;
  flex-direction: column;
  gap: 0.8em;
}

.hf-file-progress-item { display: flex; flex-direction: column; gap: 0.25em; }

.hf-file-progress-header {
  display: flex;
  align-items: center;
  gap: 0.4em;
  font-size: 0.85em;
}

.hf-status-icon { font-size: 1.1rem; flex-shrink: 0; }
.hf-done  { color: #4caf50; }
.hf-active { color: var(--blue, #2979ff); }

.hf-file-progress-path {
  flex: 1;
  font-family: monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hf-file-progress-bytes {
  color: var(--textSecondary, #666);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.hf-progress-track {
  width: 100%;
  height: 5px;
  background: var(--divider, #e0e0e0);
  border-radius: 3px;
  overflow: hidden;
}
.hf-progress-fill {
  height: 100%;
  background: var(--blue, #2979ff);
  border-radius: 3px;
  transition: width 0.15s ease;
}

.hf-repo-link {
  display: inline-flex;
  align-items: center;
  gap: 0.3em;
  color: var(--blue, #2979ff);
  text-decoration: none;
  font-family: monospace;
  font-size: 0.9em;
  margin-top: 0.5em;
}
.hf-repo-link:hover { text-decoration: underline; }
.hf-repo-link .material-icons { font-size: 1rem; }

.hf-error-msg {
  color: var(--red, #f44336);
  font-family: monospace;
  font-size: 0.82em;
  white-space: pre-wrap;
  word-break: break-all;
  padding: 0.75em;
  background: rgba(244, 67, 54, 0.06);
  border-radius: 4px;
  border-left: 3px solid var(--red, #f44336);
  margin: 0;
}
</style>
