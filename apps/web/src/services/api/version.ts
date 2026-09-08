/**
 * 版本相关 API
 */

import { apiClient } from './client';

export interface ManagerLatestRelease {
  tag_name?: string;
  name?: string;
  html_url?: string;
  published_at?: string;
  [key: string]: unknown;
}

export const versionApi = {
  checkLatest: () => apiClient.get<Record<string, unknown>>('/latest-version'),
};
