const CHINESE_TROUBLESHOOTING_PATH = '/docs/troubleshooting/';
const ENGLISH_TROUBLESHOOTING_PATH = '/docs/en/troubleshooting/';

export const localizeSetupDocsUrl = (docsUrl: string, language: string) => {
  const normalized = docsUrl.trim();
  if (!normalized) return '';
  if (language.toLowerCase().startsWith('zh')) {
    return normalized.replace(ENGLISH_TROUBLESHOOTING_PATH, CHINESE_TROUBLESHOOTING_PATH);
  }
  if (normalized.includes(ENGLISH_TROUBLESHOOTING_PATH)) return normalized;
  return normalized.replace(CHINESE_TROUBLESHOOTING_PATH, ENGLISH_TROUBLESHOOTING_PATH);
};
