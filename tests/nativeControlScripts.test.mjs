import { execFileSync, spawn, spawnSync } from 'node:child_process';
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { afterEach, describe, expect, it } from 'vitest';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const unixControlScript = path.join(repoRoot, 'bin/native/cpa-manager-plusctl.sh');
const windowsControlScript = path.join(repoRoot, 'bin/native/cpa-manager-plusctl.ps1');
const managedRuntimeSource = path.join(
  repoRoot,
  'apps/manager-server/internal/managedruntime/runtime.go'
);
const managedProcessSource = path.join(
  repoRoot,
  'apps/manager-server/internal/managedruntime/process.go'
);
const managedProcessWindowsSource = path.join(
  repoRoot,
  'apps/manager-server/internal/managedruntime/process_windows.go'
);
const replacementWindowsSource = path.join(
  repoRoot,
  'apps/manager-server/cmd/cpa-manager-plus/replacement_windows.go'
);
const tempDirs = [];

const findExecutable = (candidates) => candidates.find((candidate) => existsSync(candidate));

const windowsPowerShell = () => {
  if (process.env.SystemRoot) {
    return path.join(
      process.env.SystemRoot,
      'System32',
      'WindowsPowerShell',
      'v1.0',
      'powershell.exe'
    );
  }
  return 'powershell.exe';
};

const psQuote = (value) => `'${value.replace(/'/g, "''")}'`;

const runUnixControl = (script, env, args, options = {}) =>
  execFileSync('bash', [script, ...args], {
    env,
    encoding: 'utf8',
    ...options,
  });

const runControl = (env, args, options = {}) =>
  runUnixControl(unixControlScript, env, args, options);

const readUnixPidRecord = (pidFile) => {
  const raw = readFileSync(pidFile, 'utf8').trim();
  const metadataPid = raw.match(/^pid=([0-9]+)$/m)?.[1];
  return Number.parseInt(metadataPid || raw, 10);
};

const writeExecutablePathFakes = (tempDir) => {
  const fakeBin = path.join(tempDir, 'fake-bin');
  const processPathFile = path.join(tempDir, 'process-path.txt');
  const actualRealpath = findExecutable(['/usr/bin/realpath', '/bin/realpath']);
  mkdirSync(fakeBin, { recursive: true });
  writeFileSync(
    path.join(fakeBin, 'realpath'),
    [
      '#!/usr/bin/env bash',
      'set -euo pipefail',
      'case "${1:-}" in',
      '  /proc/*/exe) cat "${CPA_MANAGER_PLUS_TEST_PROCESS_PATH_FILE}" ;;',
      actualRealpath
        ? `  *) exec ${JSON.stringify(actualRealpath)} "$@" ;;`
        : '  *) printf "%s\\n" "${1:-}" ;;',
      'esac',
      '',
    ].join('\n')
  );
  writeFileSync(
    path.join(fakeBin, 'lsof'),
    [
      '#!/usr/bin/env bash',
      'set -euo pipefail',
      'case "$*" in',
      '  *"-d txt"*) printf "n%s\\n" "$(cat "${CPA_MANAGER_PLUS_TEST_PROCESS_PATH_FILE}")" ;;',
      '  *) exit 1 ;;',
      'esac',
      '',
    ].join('\n')
  );
  chmodSync(path.join(fakeBin, 'realpath'), 0o755);
  chmodSync(path.join(fakeBin, 'lsof'), 0o755);
  return { fakeBin, processPathFile };
};

const runPowerShell = (args, options = {}) =>
  execFileSync(windowsPowerShell(), ['-NoProfile', '-ExecutionPolicy', 'Bypass', ...args], {
    encoding: 'utf8',
    ...options,
  });

const runPowerShellControl = (env, args, options = {}) => {
  try {
    return execFileSync(
      windowsPowerShell(),
      ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', windowsControlScript, ...args],
      {
        env,
        encoding: 'utf8',
        ...options,
      }
    );
  } catch (error) {
    throw new Error(
      [
        `PowerShell control script failed: ${args.join(' ')}`,
        `status: ${error.status ?? 'unknown'}`,
        error.stdout ? `stdout:\n${error.stdout}` : '',
        error.stderr ? `stderr:\n${error.stderr}` : '',
        error.message,
      ]
        .filter(Boolean)
        .join('\n\n')
    );
  }
};

const spawnPowerShellControl = (env, args) => {
  const result = spawnSync(
    windowsPowerShell(),
    ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', windowsControlScript, ...args],
    {
      env,
      encoding: 'utf8',
    }
  );
  if (result.status !== 0) {
    throw new Error(
      [
        `PowerShell control script exited with status ${result.status}`,
        result.error ? `error: ${result.error.message}` : '',
        result.stdout ? `stdout:\n${result.stdout}` : '',
        result.stderr ? `stderr:\n${result.stderr}` : '',
      ]
        .filter(Boolean)
        .join('\n\n')
    );
  }
};

afterEach(() => {
  for (const dir of tempDirs.splice(0)) {
    rmSync(dir, { force: true, recursive: true });
  }
});

describe('native control scripts', () => {
  it('starts Unix processes from the package directory when invoked elsewhere', () => {
    if (process.platform === 'win32') {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-cwd-'));
    tempDirs.push(tempDir);

    const packageDir = path.join(tempDir, 'package');
    const callerDir = path.join(tempDir, 'caller');
    mkdirSync(packageDir, { recursive: true });
    mkdirSync(callerDir, { recursive: true });

    const controlScript = path.join(packageDir, 'cpa-manager-plusctl');
    const fakeBinary = path.join(packageDir, 'cpa-manager-plus');
    const cwdFile = path.join(tempDir, 'cwd.txt');
    const dataEnvFile = path.join(tempDir, 'data-env.txt');
    copyFileSync(unixControlScript, controlScript);
    chmodSync(controlScript, 0o755);
    writeFileSync(
      fakeBinary,
      [
        '#!/usr/bin/env bash',
        'set -euo pipefail',
        'pwd >"${CPA_MANAGER_PLUS_TEST_CWD_FILE}"',
        'printf "%s\\n" "${USAGE_DATA_DIR:-}" >"${CPA_MANAGER_PLUS_TEST_DATA_FILE}"',
        'sleep 30',
        '',
      ].join('\n')
    );
    chmodSync(fakeBinary, 0o755);

    const env = {
      ...process.env,
      CPA_MANAGER_PLUS_TEST_CWD_FILE: cwdFile,
      CPA_MANAGER_PLUS_TEST_DATA_FILE: dataEnvFile,
      USAGE_DATA_DIR: './data',
    };

    try {
      runUnixControl(controlScript, env, ['start'], { cwd: callerDir });

      expect(readFileSync(cwdFile, 'utf8').trim()).toBe(packageDir);
      expect(readFileSync(dataEnvFile, 'utf8').trim()).toBe('./data');
      expect(runUnixControl(controlScript, env, ['status'], { cwd: callerDir })).toContain(
        'is running with PID'
      );
      expect(runUnixControl(controlScript, env, ['stop'], { cwd: callerDir })).toContain('stopped');
    } finally {
      spawnSync('bash', [controlScript, 'stop'], { cwd: callerDir, env, encoding: 'utf8' });
    }
  });

  it('uses the packaged runtime mode only when no explicit Unix binary arguments are supplied', () => {
    if (process.platform === 'win32') {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-runtime-mode-'));
    tempDirs.push(tempDir);

    const packageDir = path.join(tempDir, 'package');
    mkdirSync(packageDir, { recursive: true });
    const controlScript = path.join(packageDir, 'cpa-manager-plusctl');
    const fakeBinary = path.join(packageDir, 'cpa-manager-plus');
    const argsFile = path.join(tempDir, 'args.txt');
    const modeFile = path.join(tempDir, 'mode.txt');
    copyFileSync(unixControlScript, controlScript);
    chmodSync(controlScript, 0o755);
    writeFileSync(path.join(packageDir, '.cpamp-runtime-mode'), 'integrated\n');
    writeFileSync(
      fakeBinary,
      [
        '#!/usr/bin/env bash',
        'set -euo pipefail',
        'printf "%s\\n" "$*" >"${CPA_MANAGER_PLUS_TEST_ARGS_FILE}"',
        'printf "%s\\n" "${CPA_MANAGER_DEPLOYMENT_MODE:-}" >"${CPA_MANAGER_PLUS_TEST_MODE_FILE}"',
        'sleep 30',
        '',
      ].join('\n')
    );
    chmodSync(fakeBinary, 0o755);

    const env = {
      ...process.env,
      CPA_MANAGER_PLUS_TEST_ARGS_FILE: argsFile,
      CPA_MANAGER_PLUS_TEST_MODE_FILE: modeFile,
    };

    try {
      runUnixControl(controlScript, env, ['start']);
      expect(readFileSync(argsFile, 'utf8').trim()).toBe('runtime');
      expect(readFileSync(modeFile, 'utf8').trim()).toBe('integrated');
      runUnixControl(controlScript, env, ['stop']);

      runUnixControl(controlScript, env, ['start', 'serve']);
      expect(readFileSync(argsFile, 'utf8').trim()).toBe('serve');
      expect(readFileSync(modeFile, 'utf8').trim()).toBe('');
      expect(runUnixControl(controlScript, env, ['stop'])).toContain('stopped');
    } finally {
      spawnSync('bash', [controlScript, 'stop'], { env, encoding: 'utf8' });
    }
  });

  it('keeps controlling a Unix runtime after same-PID handoff into the managed CPAMP component root', () => {
    if (process.platform === 'win32') {
      return;
    }

    const sleepBinary = findExecutable(['/bin/sleep', '/usr/bin/sleep']);
    if (!sleepBinary) {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-handoff-'));
    tempDirs.push(tempDir);
    const { fakeBin, processPathFile } = writeExecutablePathFakes(tempDir);
    const componentRoot = path.join(tempDir, 'data/runtime/components');
    const componentBinary = path.join(componentRoot, 'cpamp/v2.0.0/cpa-manager-plus');
    const pidFile = path.join(tempDir, 'run/cpa-manager-plus.pid');
    const logFile = path.join(tempDir, 'logs/cpa-manager-plus.log');
    mkdirSync(path.dirname(componentBinary), { recursive: true });
    writeFileSync(componentBinary, 'managed-component');
    writeFileSync(processPathFile, `${sleepBinary}\n`);
    const env = {
      ...process.env,
      PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
      CPA_MANAGER_PLUS_BIN: sleepBinary,
      CPA_MANAGER_PLUS_PID_FILE: pidFile,
      CPA_MANAGER_PLUS_LOG_FILE: logFile,
      CPA_MANAGER_RUNTIME_COMPONENT_DIR: componentRoot,
      CPA_MANAGER_PLUS_TEST_PROCESS_PATH_FILE: processPathFile,
    };

    try {
      runControl(env, ['start', '30']);
      writeFileSync(processPathFile, `${componentBinary}\n`);
      expect(runControl(env, ['status'])).toContain('is running with PID');
      expect(runControl(env, ['stop'])).toContain('stopped');
    } finally {
      spawnSync('bash', [unixControlScript, 'stop'], { env, encoding: 'utf8' });
    }
  });

  it('rejects same-PID Unix handoff outside the managed CPAMP component root', () => {
    if (process.platform === 'win32') {
      return;
    }

    const sleepBinary = findExecutable(['/bin/sleep', '/usr/bin/sleep']);
    if (!sleepBinary) {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-handoff-reject-'));
    tempDirs.push(tempDir);
    const { fakeBin, processPathFile } = writeExecutablePathFakes(tempDir);
    const componentRoot = path.join(tempDir, 'data/runtime/components');
    const outsideBinary = path.join(componentRoot, 'cpa/v2.0.0/cpa-manager-plus');
    const pidFile = path.join(tempDir, 'run/cpa-manager-plus.pid');
    const logFile = path.join(tempDir, 'logs/cpa-manager-plus.log');
    mkdirSync(path.dirname(outsideBinary), { recursive: true });
    writeFileSync(outsideBinary, 'outside-component');
    writeFileSync(processPathFile, `${sleepBinary}\n`);
    const env = {
      ...process.env,
      PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
      CPA_MANAGER_PLUS_BIN: sleepBinary,
      CPA_MANAGER_PLUS_PID_FILE: pidFile,
      CPA_MANAGER_PLUS_LOG_FILE: logFile,
      CPA_MANAGER_RUNTIME_COMPONENT_DIR: componentRoot,
      CPA_MANAGER_PLUS_TEST_PROCESS_PATH_FILE: processPathFile,
    };
    let pid = 0;

    try {
      runControl(env, ['start', '30']);
      pid = readUnixPidRecord(pidFile);
      writeFileSync(processPathFile, `${outsideBinary}\n`);
      const status = spawnSync('bash', [unixControlScript, 'status'], { env, encoding: 'utf8' });
      expect(status.status).not.toBe(0);
      expect(status.stdout).toContain('status is unknown');
      const stop = spawnSync('bash', [unixControlScript, 'stop'], { env, encoding: 'utf8' });
      expect(stop.status).not.toBe(0);
      expect(stop.stderr).toContain('Refusing to stop');
      expect(spawnSync('kill', ['-0', String(pid)]).status).toBe(0);
    } finally {
      if (pid > 0) {
        spawnSync('kill', ['-TERM', String(pid)]);
      }
    }
  });

  it('creates custom Unix PID/log parent directories with private runtime files', () => {
    if (process.platform === 'win32') {
      return;
    }

    const sleepBinary = findExecutable(['/bin/sleep', '/usr/bin/sleep']);
    if (!sleepBinary) {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-control-'));
    tempDirs.push(tempDir);

    const pidFile = path.join(tempDir, 'custom-run', 'nested', 'manager.pid');
    const logFile = path.join(tempDir, 'custom-logs', 'nested', 'manager.log');
    const env = {
      ...process.env,
      CPA_MANAGER_PLUS_BIN: sleepBinary,
      CPA_MANAGER_PLUS_RUN_DIR: path.join(tempDir, 'default-run'),
      CPA_MANAGER_PLUS_LOG_DIR: path.join(tempDir, 'default-logs'),
      CPA_MANAGER_PLUS_PID_FILE: pidFile,
      CPA_MANAGER_PLUS_LOG_FILE: logFile,
    };

    try {
      runControl(env, ['start', '30']);

      expect(existsSync(path.dirname(pidFile))).toBe(true);
      expect(existsSync(path.dirname(logFile))).toBe(true);
      expect(existsSync(pidFile)).toBe(true);
      expect(existsSync(logFile)).toBe(true);
      expect(statSync(pidFile).mode & 0o777).toBe(0o600);
      expect(statSync(logFile).mode & 0o777).toBe(0o600);

      const pidRecord = readFileSync(pidFile, 'utf8');
      expect(pidRecord).toContain('pid=');
      expect(pidRecord).toContain('start=');
      expect(pidRecord).toContain('binary=');
      expect(pidRecord).toContain('command=');

      expect(runControl(env, ['status'])).toContain('is running with PID');
      expect(runControl(env, ['stop'])).toContain('stopped');
      expect(existsSync(pidFile)).toBe(false);
    } finally {
      spawnSync('bash', [unixControlScript, 'stop'], { env, encoding: 'utf8' });
    }
  });

  it('does not change permissions on existing custom Unix run/log directories', () => {
    if (process.platform === 'win32') {
      return;
    }

    const sleepBinary = findExecutable(['/bin/sleep', '/usr/bin/sleep']);
    if (!sleepBinary) {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-existing-dir-'));
    tempDirs.push(tempDir);

    const runDir = path.join(tempDir, 'existing-run');
    const logDir = path.join(tempDir, 'existing-logs');
    mkdirSync(runDir, { recursive: true });
    mkdirSync(logDir, { recursive: true });
    chmodSync(runDir, 0o755);
    chmodSync(logDir, 0o755);

    const pidFile = path.join(runDir, 'cpa-manager-plus.pid');
    const logFile = path.join(logDir, 'cpa-manager-plus.log');
    const env = {
      ...process.env,
      CPA_MANAGER_PLUS_BIN: sleepBinary,
      CPA_MANAGER_PLUS_RUN_DIR: runDir,
      CPA_MANAGER_PLUS_LOG_DIR: logDir,
    };

    try {
      runControl(env, ['start', '30']);

      expect(statSync(runDir).mode & 0o777).toBe(0o755);
      expect(statSync(logDir).mode & 0o777).toBe(0o755);
      expect(statSync(pidFile).mode & 0o777).toBe(0o600);
      expect(statSync(logFile).mode & 0o777).toBe(0o600);
      expect(runControl(env, ['stop'])).toContain('stopped');
    } finally {
      spawnSync('bash', [unixControlScript, 'stop'], { env, encoding: 'utf8' });
    }
  });

  it('rejects unsafe Unix custom runtime parents and symlinked runtime files', () => {
    if (process.platform === 'win32') {
      return;
    }

    const sleepBinary = findExecutable(['/bin/sleep', '/usr/bin/sleep']);
    if (!sleepBinary) {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-unsafe-'));
    tempDirs.push(tempDir);

    const unsafeDir = path.join(tempDir, 'unsafe-logs');
    mkdirSync(unsafeDir, { recursive: true });
    chmodSync(unsafeDir, 0o777);

    const unsafeParentResult = spawnSync('bash', [unixControlScript, 'start', '30'], {
      env: {
        ...process.env,
        CPA_MANAGER_PLUS_BIN: sleepBinary,
        CPA_MANAGER_PLUS_RUN_DIR: path.join(tempDir, 'run'),
        CPA_MANAGER_PLUS_LOG_FILE: path.join(unsafeDir, 'manager.log'),
      },
      encoding: 'utf8',
    });

    expect(unsafeParentResult.status).not.toBe(0);
    expect(unsafeParentResult.stderr).toContain('unsafe runtime directory');

    const logTarget = path.join(tempDir, 'target.log');
    const symlinkedLog = path.join(tempDir, 'symlink.log');
    writeFileSync(logTarget, '');
    symlinkSync(logTarget, symlinkedLog);

    const symlinkResult = spawnSync('bash', [unixControlScript, 'start', '30'], {
      env: {
        ...process.env,
        CPA_MANAGER_PLUS_BIN: sleepBinary,
        CPA_MANAGER_PLUS_RUN_DIR: path.join(tempDir, 'run-2'),
        CPA_MANAGER_PLUS_LOG_FILE: symlinkedLog,
      },
      encoding: 'utf8',
    });

    expect(symlinkResult.status).not.toBe(0);
    expect(symlinkResult.stderr).toContain('symlinked runtime file');
  });

  it('rejects invalid Unix log line counts', () => {
    if (process.platform === 'win32') {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-logs-'));
    tempDirs.push(tempDir);

    const logFile = path.join(tempDir, 'manager.log');
    writeFileSync(logFile, 'line\n');

    for (const invalidLineCount of ['0', '-1', 'abc']) {
      const result = spawnSync('bash', [unixControlScript, 'logs', invalidLineCount], {
        env: {
          ...process.env,
          CPA_MANAGER_PLUS_LOG_FILE: logFile,
        },
        encoding: 'utf8',
      });

      expect(result.status).not.toBe(0);
      expect(result.stderr).toContain('Invalid log line count');
    }
  });

  it('refuses to stop a running process from an unverifiable legacy Unix PID file', () => {
    if (process.platform === 'win32') {
      return;
    }

    const sleepBinary = findExecutable(['/bin/sleep', '/usr/bin/sleep']);
    if (!sleepBinary) {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-conflict-'));
    tempDirs.push(tempDir);

    const pidFile = path.join(tempDir, 'run', 'manager.pid');
    const logFile = path.join(tempDir, 'logs', 'manager.log');
    const unrelatedProcess = spawn(sleepBinary, ['5'], {
      stdio: 'ignore',
    });
    const unrelatedPid = unrelatedProcess.pid;
    expect(unrelatedPid).toBeGreaterThan(0);

    const env = {
      ...process.env,
      CPA_MANAGER_PLUS_BIN: sleepBinary,
      CPA_MANAGER_PLUS_PID_FILE: pidFile,
      CPA_MANAGER_PLUS_LOG_FILE: logFile,
    };

    try {
      rmSync(path.dirname(pidFile), { force: true, recursive: true });
      mkdirSync(path.dirname(pidFile), { recursive: true });
      writeFileSync(pidFile, `${unrelatedPid}\n`);

      const stopResult = spawnSync('bash', [unixControlScript, 'stop'], {
        env,
        encoding: 'utf8',
      });

      expect(stopResult.status).not.toBe(0);
      expect(stopResult.stderr).toContain('Refusing to stop');
      expect(spawnSync('kill', ['-0', String(unrelatedPid)]).status).toBe(0);
    } finally {
      unrelatedProcess.kill();
    }
  });

  it('rejects symlinked Unix PID files on status and stop', () => {
    if (process.platform === 'win32') {
      return;
    }

    const sleepBinary = findExecutable(['/bin/sleep', '/usr/bin/sleep']);
    if (!sleepBinary) {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-pid-link-'));
    tempDirs.push(tempDir);

    const pidFile = path.join(tempDir, 'run', 'manager.pid');
    const linkPidFile = path.join(tempDir, 'linked.pid');
    const logFile = path.join(tempDir, 'logs', 'manager.log');
    const realEnv = {
      ...process.env,
      CPA_MANAGER_PLUS_BIN: sleepBinary,
      CPA_MANAGER_PLUS_PID_FILE: pidFile,
      CPA_MANAGER_PLUS_LOG_FILE: logFile,
    };
    const linkedEnv = {
      ...realEnv,
      CPA_MANAGER_PLUS_PID_FILE: linkPidFile,
    };

    try {
      runControl(realEnv, ['start', '30']);
      symlinkSync(pidFile, linkPidFile);

      for (const command of ['status', 'stop']) {
        const result = spawnSync('bash', [unixControlScript, command], {
          env: linkedEnv,
          encoding: 'utf8',
        });

        expect(result.status).not.toBe(0);
        expect(result.stderr).toContain('symlinked runtime file');
      }

      expect(runControl(realEnv, ['status'])).toContain('is running with PID');
      expect(runControl(realEnv, ['stop'])).toContain('stopped');
    } finally {
      spawnSync('bash', [unixControlScript, 'stop'], { env: realEnv, encoding: 'utf8' });
    }
  });

  it('rejects unsafe Unix PID parent directories on status and stop', () => {
    if (process.platform === 'win32') {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-pid-parent-'));
    tempDirs.push(tempDir);

    const unsafeDir = path.join(tempDir, 'unsafe-run');
    mkdirSync(unsafeDir, { recursive: true });
    chmodSync(unsafeDir, 0o777);

    const pidFile = path.join(unsafeDir, 'manager.pid');
    writeFileSync(pidFile, `${process.pid}\n`);

    const env = {
      ...process.env,
      CPA_MANAGER_PLUS_PID_FILE: pidFile,
      CPA_MANAGER_PLUS_LOG_FILE: path.join(tempDir, 'logs', 'manager.log'),
    };

    for (const command of ['status', 'stop']) {
      const result = spawnSync('bash', [unixControlScript, command], {
        env,
        encoding: 'utf8',
      });

      expect(result.status).not.toBe(0);
      expect(result.stderr).toContain('unsafe runtime directory');
    }
  });

  it('parses the Windows PowerShell control script', () => {
    if (process.platform !== 'win32') {
      return;
    }

    runPowerShell([
      '-Command',
      [
        '$tokens = $null',
        '$errors = $null',
        `[System.Management.Automation.Language.Parser]::ParseFile(${psQuote(windowsControlScript)}, [ref]$tokens, [ref]$errors) | Out-Null`,
        'if ($errors.Count -gt 0) { $errors | ForEach-Object { Write-Error $_.Message }; exit 1 }',
      ].join('; '),
    ]);
  });

  it('terminates the complete Windows runtime process tree', () => {
    const source = readFileSync(windowsControlScript, 'utf8');

    expect(source).toContain('function Stop-ProcessTree');
    expect(source).toContain("@('/PID', [string]$ProcessId, '/T')");
    expect(source).toContain("$taskkillArgs += '/F'");
    expect(source).toContain('Stop-ProcessTree -ProcessId $state.Snapshot.Pid');
  });

  it('allows native managed runtimes to finish their graceful shutdown budget', () => {
    const runtimeSource = readFileSync(managedRuntimeSource, 'utf8');
    const processSource = readFileSync(managedProcessSource, 'utf8');
    const replacementSource = readFileSync(replacementWindowsSource, 'utf8');
    const unixSource = readFileSync(unixControlScript, 'utf8');
    const windowsSource = readFileSync(windowsControlScript, 'utf8');

    const runtimeTimeout = Number(
      runtimeSource.match(/runtimeShutdownTimeout\s*=\s*(\d+) \* time\.Second/)?.[1]
    );
    const processTimeout = Number(
      processSource.match(/managedProcessGracefulStopTimeout = (\d+) \* time\.Second/)?.[1]
    );
    const replacementTimeout = Number(
      replacementSource.match(/replacementRuntimeGracefulStopTimeout = (\d+) \* time\.Second/)?.[1]
    );
    const unixTimeout = Number(unixSource.match(/graceful_stop_timeout_seconds=(\d+)/)?.[1]);
    const windowsTimeout = Number(windowsSource.match(/\$GracefulStopTimeoutSeconds = (\d+)/)?.[1]);

    expect(processTimeout).toBeGreaterThan(30);
    expect(runtimeTimeout).toBeGreaterThan(processTimeout);
    expect(replacementTimeout).toBeGreaterThan(runtimeTimeout);
    expect(unixTimeout).toBeGreaterThan(runtimeTimeout);
    expect(windowsTimeout).toBeGreaterThan(replacementTimeout);
    expect(unixSource).toContain('while [ "${elapsed}" -lt "${graceful_stop_timeout_seconds}" ]');
    expect(windowsSource).toContain('for ($i = 0; $i -lt $GracefulStopTimeoutSeconds; $i++)');
  });

  it('uses Windows process groups and control-break for managed child shutdown', () => {
    const source = readFileSync(managedProcessWindowsSource, 'utf8');
    const replacementSource = readFileSync(replacementWindowsSource, 'utf8');

    for (const windowsSource of [source, replacementSource]) {
      expect(windowsSource).toContain('windows.CREATE_NEW_PROCESS_GROUP');
      expect(windowsSource).toContain('windows.GenerateConsoleCtrlEvent');
      expect(windowsSource).toContain('windows.CTRL_BREAK_EVENT');
    }
  });

  it('starts Windows processes with custom paths, private files, logs, and stop', () => {
    if (process.platform !== 'win32') {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-win-'));
    tempDirs.push(tempDir);

    const pidFile = path.join(tempDir, 'custom-run', 'nested', 'manager.pid');
    const logFile = path.join(tempDir, 'custom-logs', 'nested', 'manager.log');
    const errLogFile = path.join(tempDir, 'custom-logs', 'nested', 'manager.err.log');
    const childScript = path.join(tempDir, 'child.js');
    writeFileSync(childScript, 'setTimeout(() => {}, 30000);\r\n');

    const env = {
      ...process.env,
      CPA_MANAGER_PLUS_BIN: process.execPath,
      CPA_MANAGER_PLUS_PID_FILE: pidFile,
      CPA_MANAGER_PLUS_LOG_FILE: logFile,
      CPA_MANAGER_PLUS_ERR_LOG_FILE: errLogFile,
    };

    try {
      spawnPowerShellControl(env, ['start', childScript]);

      expect(existsSync(pidFile)).toBe(true);
      expect(existsSync(logFile)).toBe(true);
      expect(existsSync(errLogFile)).toBe(true);
      expect(runPowerShellControl(env, ['status'])).toContain('is running with PID');

      const pidRecord = JSON.parse(readFileSync(pidFile, 'utf8'));
      expect(pidRecord.pid).toBeGreaterThan(0);
      expect(pidRecord.startTimeUtc).toBeTruthy();
      expect(pidRecord.binaryPath || pidRecord.commandLine).toBeTruthy();

      runPowerShell([
        '-Command',
        [
          `foreach ($path in @(${[pidFile, logFile, errLogFile].map(psQuote).join(', ')})) {`,
          '  $item = Get-Item -LiteralPath $path -Force;',
          '  $acl = [System.IO.File]::GetAccessControl($item.FullName);',
          '  if (-not $acl.AreAccessRulesProtected) { throw "ACL is not protected: $path" }',
          '}',
        ].join(' '),
      ]);

      expect(runPowerShellControl(env, ['logs', '20'])).toBeDefined();
      const invalidLogsResult = spawnSync(
        windowsPowerShell(),
        ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', windowsControlScript, 'logs', '0'],
        {
          env,
          encoding: 'utf8',
        }
      );
      expect(invalidLogsResult.status).not.toBe(0);
      expect(invalidLogsResult.stderr).toContain('Invalid log line count');
      expect(runPowerShellControl(env, ['stop'])).toContain('stopped');
      expect(existsSync(pidFile)).toBe(false);
    } finally {
      spawnSync(
        windowsPowerShell(),
        ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', windowsControlScript, 'stop'],
        {
          env,
          encoding: 'utf8',
        }
      );
    }
  }, 30000);

  it('rejects unsafe Windows custom runtime parents', () => {
    if (process.platform !== 'win32') {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-win-unsafe-'));
    tempDirs.push(tempDir);

    const unsafeDir = path.join(tempDir, 'unsafe-run');
    mkdirSync(unsafeDir, { recursive: true });
    const childScript = path.join(tempDir, 'child.js');
    writeFileSync(childScript, 'setTimeout(() => {}, 30000);\r\n');

    runPowerShell([
      '-Command',
      [
        `$path = ${psQuote(unsafeDir)}`,
        '$item = Get-Item -LiteralPath $path -Force',
        '$acl = [System.IO.Directory]::GetAccessControl($item.FullName)',
        '$users = New-Object System.Security.Principal.SecurityIdentifier "S-1-5-32-545"',
        '$rule = New-Object System.Security.AccessControl.FileSystemAccessRule($users, "Modify", "ContainerInherit, ObjectInherit", "None", "Allow")',
        '$acl.AddAccessRule($rule)',
        '[System.IO.Directory]::SetAccessControl($item.FullName, $acl)',
      ].join('; '),
    ]);

    const result = spawnSync(
      windowsPowerShell(),
      [
        '-NoProfile',
        '-ExecutionPolicy',
        'Bypass',
        '-File',
        windowsControlScript,
        'start',
        childScript,
      ],
      {
        env: {
          ...process.env,
          CPA_MANAGER_PLUS_BIN: process.execPath,
          CPA_MANAGER_PLUS_RUN_DIR: path.join(tempDir, 'safe-run'),
          CPA_MANAGER_PLUS_LOG_DIR: path.join(tempDir, 'safe-logs'),
          CPA_MANAGER_PLUS_PID_FILE: path.join(unsafeDir, 'manager.pid'),
        },
        encoding: 'utf8',
      }
    );

    expect(result.status).not.toBe(0);
    expect(result.stderr).toContain('unsafe runtime directory');
  });

  it('rejects reparse-point Windows PID files on status and stop', () => {
    if (process.platform !== 'win32') {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-win-pid-link-'));
    tempDirs.push(tempDir);

    const pidTargetDir = path.join(tempDir, 'pid-target');
    const pidReparsePath = path.join(tempDir, 'manager.pid');
    mkdirSync(pidTargetDir, { recursive: true });
    runPowerShell([
      '-Command',
      `New-Item -ItemType Junction -Path ${psQuote(pidReparsePath)} -Target ${psQuote(pidTargetDir)} | Out-Null`,
    ]);

    const env = {
      ...process.env,
      CPA_MANAGER_PLUS_PID_FILE: pidReparsePath,
    };

    for (const command of ['status', 'stop']) {
      const result = spawnSync(
        windowsPowerShell(),
        ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', windowsControlScript, command],
        {
          env,
          encoding: 'utf8',
        }
      );

      expect(result.status).not.toBe(0);
      expect(result.stderr).toContain('reparse-point runtime file');
    }
  });

  it('rejects unsafe Windows PID parent directories on status and stop', () => {
    if (process.platform !== 'win32') {
      return;
    }

    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-win-pid-parent-'));
    tempDirs.push(tempDir);

    const unsafeDir = path.join(tempDir, 'unsafe-run');
    mkdirSync(unsafeDir, { recursive: true });
    const pidFile = path.join(unsafeDir, 'manager.pid');
    writeFileSync(pidFile, `${process.pid}\r\n`);

    runPowerShell([
      '-Command',
      [
        `$path = ${psQuote(unsafeDir)}`,
        '$item = Get-Item -LiteralPath $path -Force',
        '$acl = [System.IO.Directory]::GetAccessControl($item.FullName)',
        '$users = New-Object System.Security.Principal.SecurityIdentifier "S-1-5-32-545"',
        '$rule = New-Object System.Security.AccessControl.FileSystemAccessRule($users, "Modify", "ContainerInherit, ObjectInherit", "None", "Allow")',
        '$acl.AddAccessRule($rule)',
        '[System.IO.Directory]::SetAccessControl($item.FullName, $acl)',
      ].join('; '),
    ]);

    const env = {
      ...process.env,
      CPA_MANAGER_PLUS_PID_FILE: pidFile,
    };

    for (const command of ['status', 'stop']) {
      const result = spawnSync(
        windowsPowerShell(),
        ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', windowsControlScript, command],
        {
          env,
          encoding: 'utf8',
        }
      );

      expect(result.status).not.toBe(0);
      expect(result.stderr).toContain('unsafe runtime directory');
    }
  });
});
