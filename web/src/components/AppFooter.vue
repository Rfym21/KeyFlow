<script setup lang="ts">
import {
  BugOutline,
  ChatbubbleOutline,
  DocumentTextOutline,
  LogoGithub,
  PeopleOutline,
} from "@vicons/ionicons5"
import { NIcon, NTooltip } from "naive-ui"
import { useI18n } from "vue-i18n"

const { t } = useI18n()
</script>

<template>
  <footer class="app-footer">
    <div class="footer-container">
      <div class="footer-main">
        <span class="project-info">
          <a href="https://github.com/tbphp/gpt-load" target="_blank" rel="noopener noreferrer">
            <b>GPT-Load</b>
          </a>
        </span>

        <n-divider vertical />

        <div class="links-container">
          <n-tooltip trigger="hover" placement="top">
            <template #trigger>
              <a
                href="https://www.gpt-load.com/docs"
                target="_blank"
                rel="noopener noreferrer"
                class="footer-link"
              >
                <n-icon :component="DocumentTextOutline" :size="14" class="link-icon" />
                <span>{{ t("footer.docs") }}</span>
              </a>
            </template>
            {{ t("footer.officialDocs") }}
          </n-tooltip>

          <n-tooltip trigger="hover" placement="top">
            <template #trigger>
              <a
                href="https://github.com/tbphp/gpt-load"
                target="_blank"
                rel="noopener noreferrer"
                class="footer-link"
              >
                <n-icon :component="LogoGithub" :size="14" class="link-icon" />
                <span>GitHub</span>
              </a>
            </template>
            {{ t("footer.viewSource") }}
          </n-tooltip>

          <n-tooltip trigger="hover" placement="top">
            <template #trigger>
              <a
                href="https://github.com/tbphp/gpt-load/issues"
                target="_blank"
                rel="noopener noreferrer"
                class="footer-link"
              >
                <n-icon :component="BugOutline" :size="14" class="link-icon" />
                <span>{{ t("footer.feedback") }}</span>
              </a>
            </template>
            {{ t("footer.reportIssue") }}
          </n-tooltip>

          <n-tooltip trigger="hover" placement="top">
            <template #trigger>
              <a
                href="https://github.com/tbphp/gpt-load/graphs/contributors"
                target="_blank"
                rel="noopener noreferrer"
                class="footer-link"
              >
                <n-icon :component="PeopleOutline" :size="14" class="link-icon" />
                <span>{{ t("footer.contributors") }}</span>
              </a>
            </template>
            {{ t("footer.viewContributors") }}
          </n-tooltip>

          <n-tooltip trigger="hover" placement="top">
            <template #trigger>
              <a
                href="https://t.me/+GHpy5SwEllg3MTUx"
                target="_blank"
                rel="noopener noreferrer"
                class="footer-link"
              >
                <n-icon :component="ChatbubbleOutline" :size="14" class="link-icon" />
                <span>Telegram</span>
              </a>
            </template>
            {{ t("footer.joinGroup") }}
          </n-tooltip>
        </div>

        <n-divider vertical />

        <div class="copyright-container">
          <span class="copyright-text">
            &copy; 2025 by
            <a
              href="https://github.com/tbphp"
              target="_blank"
              rel="noopener noreferrer"
              class="author-link"
            >
              tbphp
            </a>
          </span>
          <span class="license-text">MIT License</span>
        </div>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.app-footer {
  background: var(--footer-bg);
  backdrop-filter: blur(20px);
  border-top: 1px solid var(--border-color-light);
  padding: 12px 24px;
  font-size: 14px;
  min-height: 52px;
}

.footer-container {
  max-width: 1200px;
  margin: 0 auto;
}

.footer-main {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  line-height: 1.4;
}

.project-info {
  color: var(--text-secondary);
  font-weight: 500;
}

.project-info a {
  color: var(--primary-color);
  text-decoration: none;
  font-weight: 600;
}

.project-info a:hover {
  text-decoration: underline;
}

.links-container {
  display: flex;
  align-items: center;
  gap: 12px;
}

.footer-link {
  display: flex;
  align-items: center;
  gap: 4px;
  color: var(--text-secondary);
  text-decoration: none;
  padding: 4px 6px;
  border-radius: 4px;
  transition: all 0.2s ease;
  font-size: 13px;
  white-space: nowrap;
}

.footer-link:hover {
  color: var(--primary-color, #18a058);
  background: rgba(24, 160, 88, 0.1);
  transform: translateY(-1px);
}

.link-icon {
  display: flex;
  align-items: center;
}

.copyright-container {
  display: flex;
  align-items: center;
  gap: 8px;
}

.copyright-text {
  color: var(--text-tertiary);
  font-size: 12px;
}

.license-text {
  color: var(--text-tertiary);
  font-size: 12px;
}

.author-link {
  font-weight: 600;
  color: var(--primary-color);
  text-decoration: none;
}

.author-link:hover {
  text-decoration: underline !important;
}

@media (max-width: 768px) {
  .app-footer {
    padding: 10px 16px;
    height: auto;
  }

  .footer-main {
    flex-direction: column;
    gap: 8px;
    text-align: center;
  }

  .footer-main :deep(.n-divider) {
    display: none;
  }

  .links-container {
    gap: 16px;
  }
}

@media (max-width: 480px) {
  .footer-main {
    gap: 6px;
  }

  .links-container {
    flex-wrap: wrap;
    justify-content: center;
    gap: 12px;
  }

  .project-info {
    font-size: 12px;
  }

  .footer-link {
    font-size: 12px;
  }
}
</style>
