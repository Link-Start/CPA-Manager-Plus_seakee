import { describe, expect, it } from 'vitest';
import {
  getCodexInspectionMockScenario,
  getCodexInspectionMockSearch,
  isCodexInspectionMockEnabled,
  shouldAllowCodexInspectionMock,
} from './codexInspectionMockData';

describe('codexInspectionMockData', () => {
  it('parses mock params from hash-style URLs on localhost', () => {
    const href =
      'http://localhost:5173/#/monitoring/codex-inspection-tasks?mockCodexInspection=1&mockCodexInspectionScenario=stress';

    expect(
      isCodexInspectionMockEnabled('', {
        dev: false,
        mode: 'production',
        hostname: 'localhost',
        href,
      })
    ).toBe(true);
    expect(
      getCodexInspectionMockScenario('', {
        href,
      })
    ).toBe('stress');
    expect(
      getCodexInspectionMockSearch('', {
        href,
      })
    ).toBe('?mockCodexInspection=1&mockCodexInspectionScenario=stress');
  });

  it('prefers router search when both router and hash queries exist', () => {
    const href =
      'http://localhost:5173/#/monitoring/codex-inspection-tasks?mockCodexInspection=1&mockCodexInspectionScenario=default';
    const routerSearch = '?mockCodexInspection=1&mockCodexInspectionScenario=empty';

    expect(
      getCodexInspectionMockScenario(routerSearch, {
        href,
      })
    ).toBe('empty');
    expect(
      getCodexInspectionMockSearch(routerSearch, {
        href,
      })
    ).toBe(routerSearch);
  });

  it('allows localhost preview while keeping non-localhost production disabled', () => {
    expect(
      shouldAllowCodexInspectionMock({
        dev: false,
        mode: 'production',
        hostname: 'localhost',
      })
    ).toBe(true);
    expect(
      shouldAllowCodexInspectionMock({
        dev: false,
        mode: 'production',
        hostname: 'example.com',
      })
    ).toBe(false);
  });
});
