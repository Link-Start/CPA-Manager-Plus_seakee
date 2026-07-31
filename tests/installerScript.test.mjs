import { spawn, spawnSync } from 'node:child_process';
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const installerPath = path.join(repoRoot, 'bin/install-cpamp.sh');
const managedRuntimePath = path.join(
  repoRoot,
  'apps/manager-server/internal/managedruntime/runtime.go'
);
const dockerComposePath = path.join(repoRoot, 'docker-compose.manager.yml');

const combinedOutput = (result) => `${result.stdout}\n${result.stderr}`;

const readInstallerPid = (installDir) => {
  const raw = readFileSync(path.join(installDir, 'cpa-manager-plus.pid'), 'utf8').trim();
  const metadataPid = raw.match(/^pid=([0-9]+)$/m)?.[1];
  return Number.parseInt(metadataPid || raw, 10);
};

const processStartMarker = (pid) => {
  const procStat = `/proc/${pid}/stat`;
  if (existsSync(procStat)) {
    return readFileSync(procStat, 'utf8').trim().split(/\s+/)[21] || '';
  }
  const result = spawnSync('ps', ['-ww', '-p', String(pid), '-o', 'lstart='], {
    encoding: 'utf8',
  });
  return result.status === 0 ? result.stdout.trim().replace(/\s+/g, ' ') : '';
};

const startDetachedSleep = (binary) => {
  const result = spawnSync(
    'bash',
    ['-c', 'nohup "$1" 30 >/dev/null 2>&1 & printf "%s\\n" "$!"', 'cpamp-installer-test', binary],
    { encoding: 'utf8' }
  );
  if (result.status !== 0) {
    throw new Error(combinedOutput(result));
  }
  return Number.parseInt(result.stdout.trim(), 10);
};

const writeInstallerProcessPathFakes = (fakeBin, processPathFile) => {
  const actualRealpath = ['/usr/bin/realpath', '/bin/realpath'].find((candidate) =>
    existsSync(candidate)
  );
  writeFileSync(
    path.join(fakeBin, 'realpath'),
    [
      '#!/usr/bin/env bash',
      'set -euo pipefail',
      'case "${1:-}" in',
      '  /proc/*/exe) cat "${CPAMP_TEST_PROCESS_PATH_FILE}" ;;',
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
      '  *"-d txt"*) printf "n%s\\n" "$(cat "${CPAMP_TEST_PROCESS_PATH_FILE}")" ;;',
      '  *) exit 1 ;;',
      'esac',
      '',
    ].join('\n')
  );
  chmodSync(path.join(fakeBin, 'realpath'), 0o755);
  chmodSync(path.join(fakeBin, 'lsof'), 0o755);
};

const runInstaller = (env) =>
  spawnSync('bash', [installerPath], {
    cwd: repoRoot,
    env: {
      ...process.env,
      CPAMP_DRY_RUN: '1',
      CPAMP_NON_INTERACTIVE: '1',
      CPAMP_LANG: 'en-US',
      CPAMP_INSTALL_DIR: '/tmp/cpamp-installer-test',
      ...env,
    },
    encoding: 'utf8',
  });

const runInstallerFromStdin = (env) =>
  spawnSync('bash', ['-c', `bash < ${JSON.stringify(installerPath)}`], {
    cwd: repoRoot,
    env: {
      ...process.env,
      CPAMP_DRY_RUN: '1',
      CPAMP_INSTALL_DIR: '/tmp/cpamp-installer-test',
      ...env,
    },
    encoding: 'utf8',
  });

const writeFakeDocker = (dir) => {
  const fakeDocker = path.join(dir, 'docker');
  const fakeCurl = path.join(dir, 'curl');
  writeFileSync(
    fakeDocker,
    `#!/usr/bin/env bash
set -eu
if [ -n "\${FAKE_DOCKER_LOG:-}" ]; then
  printf '%s|%s\n' "\${COMPOSE_PROJECT_NAME:-}" "$*" >> "$FAKE_DOCKER_LOG"
fi
if [ "$1" = "volume" ] && [ "\${2:-}" = "inspect" ]; then
  if [ "\${FAKE_DOCKER_VOLUME_EXISTS:-0}" = "1" ]; then
    exit 0
  fi
  exit 1
fi
if [ "$1" = "info" ] && [ "\${FAKE_DOCKER_DAEMON_OK:-1}" != "1" ]; then
  exit 1
fi
if [ "$1" = "compose" ] && [ "\${2:-}" = "images" ] && [ "\${3:-}" = "-q" ]; then
  case "\${4:-}" in
    cpa-manager-plus) printf '%s\n' "\${FAKE_CPAMP_PREVIOUS_IMAGE_ID-sha256:previous-cpamp}" ;;
    cli-proxy-api) printf '%s\n' "\${FAKE_CPA_PREVIOUS_IMAGE_ID-sha256:previous-cpa}" ;;
  esac
  exit 0
fi
if [ "$1" = "compose" ] && [ "\${2:-}" = "config" ]; then
  printf 'services:\n'
  printf '  cpa-manager-plus:\n'
  printf '    image: %s\n' "\${FAKE_CPAMP_COMPOSE_IMAGE:-example/cpamp:latest}"
  if [ "\${FAKE_DOCKER_STACK:-0}" = "1" ] || grep -q '^[[:space:]]*cli-proxy-api:' compose.yaml 2>/dev/null; then
    printf '  cli-proxy-api:\n'
    printf '    image: %s\n' "\${FAKE_CPA_COMPOSE_IMAGE:-example/cpa:latest}"
  fi
  exit 0
fi
if [ "$1" = "image" ] && [ "\${2:-}" = "tag" ]; then
  if [ -n "\${FAKE_DOCKER_ROLLBACK_MARKER:-}" ]; then
    touch "$FAKE_DOCKER_ROLLBACK_MARKER"
  fi
  exit 0
fi
if [ "$1" = "compose" ] && [ "\${2:-}" = "logs" ]; then
  printf '%s\n' "\${FAKE_CPAMP_LOGS:-}"
  exit 0
fi
if [ "$1" = "compose" ] && [ "\${2:-}" = "exec" ]; then
  case "$*" in
    *'cat /data/runtime/state.json'*)
      printf '{"deployment":{"panelBasePath":"%s"}}' "\${FAKE_CPAMP_PANEL_BASE_PATH:-/management.html}"
      exit 0
      ;;
  esac
  if [ "\${FAKE_DOCKER_HEALTH_FAIL_UNTIL_ROLLBACK:-0}" = "1" ] &&
     { [ -z "\${FAKE_DOCKER_ROLLBACK_MARKER:-}" ] || [ ! -f "$FAKE_DOCKER_ROLLBACK_MARKER" ]; }; then
    exit 1
  fi
  if [ -n "\${FAKE_CPAMP_INTERNAL_PORT:-}" ]; then
    case "$*" in
      *":\${FAKE_CPAMP_INTERNAL_PORT}/health"*|*":\${FAKE_CPAMP_INTERNAL_PORT}/status"*) ;;
      *) exit 1 ;;
    esac
  fi
  case "$*" in
    *'/status'*)
      if [ "\${FAKE_DOCKER_AUTH_OK:-1}" = "1" ]; then
        exit 0
      fi
      exit 1
      ;;
    *) exit 0 ;;
  esac
fi
exit 0
`
  );
  writeFileSync(
    fakeCurl,
    `#!/usr/bin/env bash
set -eu
case "$*" in
  *'/health'*)
    if [ -n "\${FAKE_CPAMP_PUBLIC_PORT:-}" ]; then
      case "$*" in
        *":\${FAKE_CPAMP_PUBLIC_PORT}/health"*) ;;
        *) exit 7 ;;
      esac
    fi
    ;;
  *'/usage-service/info'*)
    if [ -n "\${FAKE_CPAMP_PUBLIC_PORT:-}" ]; then
      case "$*" in
        *":\${FAKE_CPAMP_PUBLIC_PORT}/usage-service/info"*) ;;
        *) exit 7 ;;
      esac
    fi
    printf '{"service":"cpa-manager-plus","bootstrapRequired":%s}' "\${FAKE_CPAMP_BOOTSTRAP_REQUIRED:-false}"
    ;;
esac
`
  );
  chmodSync(fakeDocker, 0o755);
  chmodSync(fakeCurl, 0o755);
  return fakeDocker;
};

const writeFakeCPACurl = (dir) => {
  const fakeCurl = path.join(dir, 'curl');
  writeFileSync(
    fakeCurl,
    `#!/usr/bin/env bash
set -eu
if [ "\${FAKE_CPA_CURL_EXIT:-0}" = "1" ]; then
  exit 7
fi
printf '%s' "\${FAKE_CPA_STATUS:-200}"
`
  );
  chmodSync(fakeCurl, 0o755);
  return fakeCurl;
};

describe('installer script', () => {
  it('passes shell syntax validation', () => {
    const result = spawnSync('bash', ['-n', installerPath], {
      cwd: repoRoot,
      encoding: 'utf8',
    });

    expect(result.status).toBe(0);
    expect(result.stderr).toBe('');
  });

  it('waits longer than the managed runtime shutdown budget during native upgrades', () => {
    const installer = readFileSync(installerPath, 'utf8');
    const runtime = readFileSync(managedRuntimePath, 'utf8');
    const timeout = Number.parseInt(
      installer.match(/^native_graceful_stop_timeout_seconds=([0-9]+)$/m)?.[1] || '',
      10
    );
    const runtimeTimeout = Number.parseInt(
      runtime.match(/^\s*runtimeShutdownTimeout\s*=\s*([0-9]+) \* time\.Second$/m)?.[1] || '',
      10
    );

    expect(runtimeTimeout).toBeGreaterThan(30);
    expect(timeout).toBeGreaterThan(runtimeTimeout);
    expect(installer).toContain('local attempts=$((native_graceful_stop_timeout_seconds * 4))');
    expect(installer).toMatch(
      /while \[ "\$i" -le "\$attempts" \]; do[\s\S]*sleep 0\.25[\s\S]*done\n  ! kill -0 "\$pid"/
    );
  });

  it('preserves enough Docker stop grace for managed runtime shutdown', () => {
    const installer = readFileSync(installerPath, 'utf8');
    const compose = readFileSync(dockerComposePath, 'utf8');

    expect(compose).toContain('stop_grace_period: 45s');
    expect(installer.match(/stop_grace_period: 45s/g)).toHaveLength(3);
    expect(installer.match(/stop_grace_period: 35s/g)).toHaveLength(1);
  });

  it('discovers the panel path from local runtime state instead of the public info endpoint', () => {
    const source = readFileSync(installerPath, 'utf8');
    const start = source.indexOf('discover_runtime_panel_path() {');
    const end = source.indexOf('\npost_install_message() {', start);
    const discovery = source.slice(start, end);

    expect(start).toBeGreaterThanOrEqual(0);
    expect(end).toBeGreaterThan(start);
    expect(discovery).toContain('/data/runtime/state.json');
    expect(discovery).toContain('$install_dir/data/runtime/state.json');
    expect(discovery).toContain('/health');
    expect(discovery).not.toContain('/usage-service/info');
  });

  it('cleans the temporary CPA bearer header on exits and signals', () => {
    const source = readFileSync(installerPath, 'utf8');

    expect(source).toContain('validate_cpa_management_access() (');
    expect(source).toContain('trap cleanup_cpa_header EXIT');
    expect(source).toContain("trap 'cleanup_cpa_header; exit 129' HUP");
    expect(source).toContain("trap 'cleanup_cpa_header; exit 130' INT");
    expect(source).toContain("trap 'cleanup_cpa_header; exit 143' TERM");
  });

  it('refuses interactive execution when stdin is not a terminal', () => {
    const result = runInstallerFromStdin({});

    expect(result.status).toBe(1);
    expect(combinedOutput(result)).toContain('Interactive install requires a terminal on stdin');
  });

  it('keeps explicit non-interactive stdin execution available', () => {
    const result = runInstallerFromStdin({
      CPAMP_NON_INTERACTIVE: '1',
      CPAMP_LANG: 'en-US',
      CPAMP_INSTALL_MODE: 'stack',
      CPAMP_DEPLOY_METHOD: 'docker',
    });

    expect(result.status).toBe(0);
    expect(result.stdout).toContain('Install scope: CPA + CPAMP stack');
    expect(result.stdout).toContain('docker compose pull');
  });

  it('resets interactive core choices when the summary returns to modify', () => {
    const fixtureDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-source-'));
    const sourcePath = path.join(fixtureDir, 'install-cpamp-source.sh');

    try {
      const source = readFileSync(installerPath, 'utf8').replace(/\nmain "\$@"\s*$/, '\n');
      writeFileSync(sourcePath, source);

      const result = spawnSync(
        'bash',
        [
          '-c',
          [
            'source "$1"',
            'non_interactive=0',
            'existing_install_state=fresh',
            'install_mode=stack',
            'deploy_method=docker',
            'cpa_connection_mode=env',
            'deployment_mode=installer-managed',
            'cpa_validation_url=http://cpa.example:8317',
            'cpa_url=http://host.docker.internal:8317',
            'cpa_management_key=previous-key',
            'prompt_choice() { printf "modify\\n"; }',
            'if confirm_choices; then exit 9; fi',
            'printf "%s|%s|%s|%s|%s|%s|%s\\n" "$install_mode" "$deploy_method" "$cpa_connection_mode" "$deployment_mode" "$cpa_validation_url" "$cpa_url" "$cpa_management_key"',
          ].join('\n'),
          'cpamp-installer-test',
          sourcePath,
        ],
        {
          cwd: repoRoot,
          env: {
            ...process.env,
            CPAMP_INSTALL_MODE: '',
            CPAMP_DEPLOY_METHOD: '',
            CPAMP_CPA_CONNECTION_MODE: '',
          },
          encoding: 'utf8',
        }
      );

      expect(result.status).toBe(0);
      expect(result.stdout.trim()).toBe('||||||');
    } finally {
      rmSync(fixtureDir, { recursive: true, force: true });
    }
  });

  it('keeps native deployment locked while modifying an existing native install', () => {
    const fixtureDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-source-'));
    const sourcePath = path.join(fixtureDir, 'install-cpamp-source.sh');

    try {
      const source = readFileSync(installerPath, 'utf8').replace(/\nmain "\$@"\s*$/, '\n');
      writeFileSync(sourcePath, source);

      const result = spawnSync(
        'bash',
        [
          '-c',
          [
            'source "$1"',
            'non_interactive=0',
            'existing_install_state=native-managed',
            'install_mode=cpamp',
            'deploy_method=native',
            'cpa_connection_mode=preserve',
            'deployment_mode=slim',
            'prompt_choice() { printf "modify\\n"; }',
            'if confirm_choices; then exit 9; fi',
            'printf "%s|%s|%s|%s\\n" "$install_mode" "$deploy_method" "$cpa_connection_mode" "$deployment_mode"',
          ].join('\n'),
          'cpamp-installer-test',
          sourcePath,
        ],
        {
          cwd: repoRoot,
          env: {
            ...process.env,
            CPAMP_INSTALL_MODE: '',
            CPAMP_DEPLOY_METHOD: '',
            CPAMP_CPA_CONNECTION_MODE: '',
          },
          encoding: 'utf8',
        }
      );

      expect(result.status).toBe(0);
      expect(result.stdout.trim()).toBe('cpamp|native|preserve|');
    } finally {
      rmSync(fixtureDir, { recursive: true, force: true });
    }
  });

  it.each([
    ['en-US', 'must keep the deployment method, install scope, and CPA source'],
    ['zh-CN', '必须保持原部署方式、安装范围和 CPA 来源'],
  ])(
    'rejects topology conversion inside an existing native install directory in %s',
    (lang, message) => {
      const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-'));
      const oldRuntime = path.join(installDir, 'runtime/cpa-manager-plus_v1.11.0_legacy');

      try {
        mkdirSync(oldRuntime, { recursive: true });
        writeFileSync(path.join(oldRuntime, 'cpa-manager-plus'), 'old-binary');
        writeFileSync(
          path.join(installDir, 'run.sh'),
          `#!/usr/bin/env bash\nexport CPA_MANAGER_DEPLOYMENT_MODE=slim\ncd ${JSON.stringify(oldRuntime)}\nexec ./cpa-manager-plus runtime\n`
        );

        const result = spawnSync('bash', [installerPath], {
          cwd: repoRoot,
          env: {
            ...process.env,
            CPAMP_DRY_RUN: '1',
            CPAMP_OPERATION: 'upgrade',
            CPAMP_NON_INTERACTIVE: '1',
            CPAMP_LANG: lang,
            CPAMP_INSTALL_DIR: installDir,
            CPAMP_INSTALL_MODE: 'stack',
            CPAMP_DEPLOY_METHOD: 'native',
          },
          encoding: 'utf8',
        });

        expect(result.status).toBe(1);
        expect(combinedOutput(result)).toContain(message);
      } finally {
        rmSync(installDir, { recursive: true, force: true });
      }
    }
  );

  it('prints a full Docker stack dry-run plan', () => {
    const result = runInstaller({
      CPAMP_INSTALL_MODE: 'stack',
      CPAMP_DEPLOY_METHOD: 'docker',
    });

    expect(result.status).toBe(0);
    expect(result.stdout).toContain('Install scope: CPA + CPAMP stack');
    expect(result.stdout).toContain('CPA URL for CPAMP: http://cli-proxy-api:8317');
    expect(result.stdout).toContain('docker compose pull');
    expect(result.stdout).toContain('Dry-run plan completed');
  });

  it('keeps CPAMP-only non-interactive installs in first-setup mode by default', () => {
    const result = runInstaller({
      CPAMP_INSTALL_MODE: 'cpamp',
      CPAMP_DEPLOY_METHOD: 'docker',
    });

    expect(result.status).toBe(0);
    expect(result.stdout).toContain('CPA connection: choose in the Slim setup wizard');
    expect(result.stdout).toContain('Dry-run plan completed');
  });

  it('selects the native Full package for full stack installs', () => {
    const platform = process.platform === 'darwin' ? 'darwin' : 'linux';
    const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
    const result = runInstaller({
      CPAMP_INSTALL_MODE: 'stack',
      CPAMP_DEPLOY_METHOD: 'native',
      CPAMP_VERSION: 'v1.12.0',
    });

    expect(result.status).toBe(0);
    expect(result.stdout).toContain(`cpa-manager-plus_v1.12.0_${platform}_${arch}_full.tar.gz`);
    expect(result.stdout).toContain('CPA connection: bundled CPA');
  });

  it('rejects CPA URLs that would inject extra env lines', () => {
    const result = runInstaller({
      CPAMP_INSTALL_MODE: 'cpamp',
      CPAMP_DEPLOY_METHOD: 'docker',
      CPAMP_CPA_CONNECTION_MODE: 'env',
      CPAMP_CPA_URL: 'http://host.docker.internal:8317\nCPA_MANAGER_ADMIN_KEY=bad',
      CPAMP_CPA_MANAGEMENT_KEY: 'cpa_existing_management_key',
    });

    expect(result.status).toBe(1);
    expect(combinedOutput(result)).toContain('CPA URL must be a single line');
  });

  it('rejects CPA URLs with URL fragments', () => {
    const result = runInstaller({
      CPAMP_INSTALL_MODE: 'cpamp',
      CPAMP_DEPLOY_METHOD: 'docker',
      CPAMP_CPA_CONNECTION_MODE: 'env',
      CPAMP_CPA_URL: 'http://host.docker.internal:8317#fragment',
      CPAMP_CPA_MANAGEMENT_KEY: 'cpa_existing_management_key',
    });

    expect(result.status).toBe(1);
    expect(combinedOutput(result)).toContain('CPA URL contains unsupported characters');
  });

  it('rejects CPA URLs with query strings', () => {
    const result = runInstaller({
      CPAMP_INSTALL_MODE: 'cpamp',
      CPAMP_DEPLOY_METHOD: 'docker',
      CPAMP_CPA_CONNECTION_MODE: 'env',
      CPAMP_CPA_URL: 'http://host.docker.internal:8317?x=y',
      CPAMP_CPA_MANAGEMENT_KEY: 'cpa_existing_management_key',
    });

    expect(result.status).toBe(1);
    expect(combinedOutput(result)).toContain('CPA URL contains unsupported characters');
  });

  it('rejects Docker image references with compose interpolation syntax', () => {
    const result = runInstaller({
      CPAMP_INSTALL_MODE: 'stack',
      CPAMP_DEPLOY_METHOD: 'docker',
      CPAMP_IMAGE: 'seakee/cpa-manager-plus:${BAD}',
    });

    expect(result.status).toBe(1);
    expect(combinedOutput(result)).toContain('CPAMP Docker image contains unsupported characters');
  });

  it('rejects native release versions that can escape the runtime directory', () => {
    const result = runInstaller({
      CPAMP_INSTALL_MODE: 'cpamp',
      CPAMP_DEPLOY_METHOD: 'native',
      CPAMP_CPA_CONNECTION_MODE: 'setup',
      CPAMP_VERSION: '../v1.12.0',
    });

    expect(result.status).toBe(1);
    expect(combinedOutput(result)).toContain('CPAMP version contains unsupported characters');
  });

  it('rejects Docker Compose project names that Docker would normalize differently', () => {
    const result = runInstaller({
      CPAMP_PROJECT_NAME: 'CPAMP.Bad',
      CPAMP_INSTALL_MODE: 'stack',
      CPAMP_DEPLOY_METHOD: 'docker',
    });

    expect(result.status).toBe(1);
    expect(combinedOutput(result)).toContain(
      'Docker Compose project name contains unsupported characters'
    );
  });

  it('rejects an empty persisted Docker Compose project name', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));

    try {
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(path.join(installDir, '.env'), 'COMPOSE_PROJECT_NAME=\nCPAMP_PORT=18317\n');
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    image: example/cpamp:v1\n'
      );
      writeFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), 'cpamp_existing_admin_key\n');

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_DRY_RUN: '1',
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('Docker Compose project name must not be empty');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('fails instead of looping when the random source yields no alphanumeric characters', () => {
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      const fakeOpenSSL = path.join(fakeBin, 'openssl');
      writeFileSync(fakeOpenSSL, '#!/usr/bin/env bash\nprintf -- "----"\n');
      chmodSync(fakeOpenSSL, 0o755);

      const result = runInstaller({
        CPAMP_INSTALL_MODE: 'stack',
        CPAMP_DEPLOY_METHOD: 'docker',
        PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain(
        'Random source produced no usable alphanumeric characters'
      );
    } finally {
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('generates full Docker config with CPA image paths', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));

    try {
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'stack',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);

      const compose = readFileSync(path.join(installDir, 'compose.yaml'), 'utf8');
      const cpaConfig = readFileSync(path.join(installDir, 'cliproxyapi/config.yaml'), 'utf8');
      const cpaManagementKey = readFileSync(
        path.join(installDir, 'secrets/cpa-management-key'),
        'utf8'
      ).trim();
      const demoClientKey = readFileSync(
        path.join(installDir, 'secrets/cpa-demo-client-key'),
        'utf8'
      ).trim();

      expect(compose).toContain('./cliproxyapi/config.yaml:/CLIProxyAPI/config.yaml');
      expect(compose).toContain('./cliproxyapi/auths:/root/.cli-proxy-api');
      expect(compose).toContain('./cliproxyapi/logs:/CLIProxyAPI/logs');
      expect(compose).toContain('CPA_MANAGER_DEPLOYMENT_MODE: "installer-managed"');
      expect(compose).toContain('"${CPAMP_API_PORT}:8137"');
      expect(compose).toContain('"${CPAMP_PANEL_PORT}:18137"');
      expect(compose).toContain('"${CPAMP_PORT}:18317"');
      expect(compose).not.toContain('CPA_MANAGER_ADMIN_KEY_FILE');
      expect(existsSync(path.join(installDir, 'secrets/cpamp-admin-key'))).toBe(false);
      expect(cpaManagementKey).toMatch(/^cpa_[A-Za-z0-9]{32}$/);
      expect(demoClientKey).toMatch(/^sk-[A-Za-z0-9]{64}$/);
      expect(cpaConfig).toContain('auth-dir: "/root/.cli-proxy-api"');
      expect(cpaConfig).toContain(`secret-key: "${cpaManagementKey}"`);
      expect(cpaConfig).toContain(`api-keys:\n  - "${demoClientKey}"`);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it.each([
    ['gateway', { CPAMP_CPA_PORT: '8137' }],
    ['management', { CPAMP_CPA_PORT: '18137' }],
    ['compatibility', { CPAMP_CPA_PORT: '18317' }],
  ])('rejects a CPA host port that conflicts with the CPAMP %s port', (_label, ports) => {
    const result = runInstaller({
      CPAMP_INSTALL_MODE: 'stack',
      CPAMP_DEPLOY_METHOD: 'docker',
      ...ports,
    });

    expect(result.status).toBe(1);
    expect(combinedOutput(result)).toContain('CPA and CPAMP host ports must be different.');
  });

  it('generates CPAMP-only Docker config for a host CPA URL', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFakeCPACurl(fakeBin);
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_CPA_CONNECTION_MODE: 'env',
          CPAMP_CPA_URL: 'http://127.0.0.1:8317',
          CPAMP_CPA_MANAGEMENT_KEY: 'cpa_existing_management_key',
          CPAMP_INSTALL_DIR: installDir,
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);

      const envFile = readFileSync(path.join(installDir, '.env'), 'utf8');
      const compose = readFileSync(path.join(installDir, 'compose.yaml'), 'utf8');

      expect(envFile).toContain('CPA_UPSTREAM_URL=http://host.docker.internal:8317');
      expect(envFile).toContain('CPA_MANAGER_DEPLOYMENT_MODE=installer-managed');
      expect(compose).not.toContain('CPA_MANAGER_ADMIN_KEY_FILE');
      expect(result.stdout).toContain('CPA Management Key validation passed');
      expect(result.stdout).toContain('Deployment config generated');
      if (process.platform === 'linux') {
        expect(compose).toContain('host.docker.internal:host-gateway');
      }
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('uses a detected CPA only after validating its management key', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFakeCPACurl(fakeBin);
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_DETECTED_CPA_URL: 'http://127.0.0.1:8317',
          CPAMP_USE_DETECTED_CPA: '1',
          CPAMP_CPA_MANAGEMENT_KEY: 'detected-management-key',
          CPAMP_INSTALL_DIR: installDir,
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      expect(result.stdout).toContain('Detected an available CPA service');
      expect(result.stdout).toContain('CPA Management Key validation passed');
      expect(readFileSync(path.join(installDir, '.env'), 'utf8')).toContain(
        'CPA_UPSTREAM_URL=http://host.docker.internal:8317'
      );
      expect(existsSync(path.join(installDir, 'secrets/cpamp-admin-key'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('rejects a detected CPA when the management key returns 401', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFakeCPACurl(fakeBin);
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_DETECTED_CPA_URL: 'http://127.0.0.1:8317',
          CPAMP_USE_DETECTED_CPA: '1',
          CPAMP_CPA_MANAGEMENT_KEY: 'wrong-management-key',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_CPA_STATUS: '401',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('validation failed (401/403)');
      expect(existsSync(path.join(installDir, '.env'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('fails when a forced detected CPA is unreachable', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFakeCPACurl(fakeBin);
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_DETECTED_CPA_URL: 'http://127.0.0.1:8317',
          CPAMP_USE_DETECTED_CPA: '1',
          CPAMP_CPA_MANAGEMENT_KEY: 'management-key',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_CPA_CURL_EXIT: '1',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('CPA Management API is unreachable');
      expect(existsSync(path.join(installDir, '.env'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('reuses an existing CPA Management Key secret for CPAMP-only Docker env installs', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFakeCPACurl(fakeBin);
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(
        path.join(installDir, 'secrets/cpa-management-key'),
        'cpa_reused_management_key\n'
      );

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_CPA_CONNECTION_MODE: 'env',
          CPAMP_CPA_URL: 'http://127.0.0.1:8317',
          CPAMP_INSTALL_DIR: installDir,
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      expect(readFileSync(path.join(installDir, 'secrets/cpa-management-key'), 'utf8').trim()).toBe(
        'cpa_reused_management_key'
      );
      expect(readFileSync(path.join(installDir, 'compose.yaml'), 'utf8')).toContain(
        'CPA_MANAGEMENT_KEY_FILE: "/run/secrets/cpa_management_key"'
      );
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('reuses an existing CPA Management Key secret during dry runs', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));

    try {
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(
        path.join(installDir, 'secrets/cpa-management-key'),
        'cpa_reused_management_key\n'
      );

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_DRY_RUN: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_CPA_CONNECTION_MODE: 'env',
          CPAMP_CPA_URL: 'http://host.docker.internal:8317',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      expect(combinedOutput(result)).not.toContain('secrets/cpa-management-key must not be empty');
      expect(result.stdout).toContain('installer-validated existing CPA');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('blocks non-interactive installs when an orphaned Docker data volume exists', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFakeDocker(fakeBin);
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_VOLUME_EXISTS: '1',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('old Docker data volume exists');
      expect(existsSync(path.join(installDir, 'compose.yaml'))).toBe(false);
      expect(existsSync(path.join(installDir, 'secrets/cpamp-admin-key'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('requires the original install scope for non-interactive orphan-volume repair', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFakeDocker(fakeBin);
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'repair',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_VOLUME_EXISTS: '1',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('requires CPAMP_INSTALL_MODE=stack or cpamp');
      expect(existsSync(path.join(installDir, 'compose.yaml'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('rejects skipped execution for orphan-volume repair before writing a new secret', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFakeDocker(fakeBin);
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'repair',
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'stack',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_VOLUME_EXISTS: '1',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('cannot use CPAMP_SKIP_EXECUTE=1');
      expect(existsSync(path.join(installDir, 'secrets/cpamp-admin-key'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('fails before writing Docker config when the Docker daemon is unavailable', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFakeDocker(fakeBin);
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'stack',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_DAEMON_OK: '0',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('Docker daemon is not available');
      expect(existsSync(path.join(installDir, '.env'))).toBe(false);
      expect(existsSync(path.join(installDir, 'compose.yaml'))).toBe(false);
      expect(existsSync(path.join(installDir, 'secrets/cpamp-admin-key'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('does not create a replacement admin secret before repair preflight succeeds', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFileSync(
        path.join(installDir, '.env'),
        'COMPOSE_PROJECT_NAME=cpamp\nCPAMP_IMAGE=example/cpamp:v1\nCPAMP_PORT=18317\n'
      );
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    image: ${CPAMP_IMAGE}\n'
      );
      writeFakeDocker(fakeBin);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'repair',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_DAEMON_OK: '0',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('Docker daemon is not available');
      expect(existsSync(path.join(installDir, 'secrets/cpamp-admin-key'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('does not create a half-repaired admin secret when repair execution is skipped', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));

    try {
      writeFileSync(
        path.join(installDir, '.env'),
        'COMPOSE_PROJECT_NAME=cpamp\nCPAMP_IMAGE=example/cpamp:v1\nCPAMP_PORT=18317\n'
      );
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    image: ${CPAMP_IMAGE}\n'
      );

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'repair',
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      expect(existsSync(path.join(installDir, 'secrets/cpamp-admin-key'))).toBe(false);
      expect(result.stdout).toContain('upgrade or repair commands were skipped');
      expect(result.stdout).not.toContain('Admin key saved');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('repairs an orphaned Docker deployment and verifies the generated admin key', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const dockerLog = path.join(
      os.tmpdir(),
      `cpamp-installer-docker-${process.pid}-${Date.now()}.log`
    );

    try {
      writeFakeDocker(fakeBin);
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'repair',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'stack',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_LOG: dockerLog,
          FAKE_DOCKER_VOLUME_EXISTS: '1',
          FAKE_DOCKER_AUTH_OK: '1',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      expect(readFileSync(dockerLog, 'utf8')).toContain('compose run --rm -v ');
      expect(readFileSync(dockerLog, 'utf8')).toContain(
        'cpa-manager-plus reset-admin-key --admin-key-file /run/secrets/cpamp_admin_key'
      );
      expect(readFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), 'utf8').trim()).toMatch(
        /^cpamp_[A-Za-z0-9]{32}$/
      );
      expect(result.stdout).toContain('Admin key verification passed');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
      rmSync(dockerLog, { force: true });
    }
  });

  it('upgrades a managed Docker install without rewriting config or secrets', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const dockerLog = path.join(
      os.tmpdir(),
      `cpamp-installer-docker-${process.pid}-${Date.now()}.log`
    );
    const envContent =
      'COMPOSE_PROJECT_NAME=oldproject\nCOMPOSE_PROJECT_NAME=cpamp\nCPAMP_IMAGE=example/cpamp:v1\nCPAMP_PORT=19999\nCPAMP_PORT=18317\n';
    const composeContent =
      'services:\n  cpa-manager-plus:\n    image: ${CPAMP_IMAGE}\n  unrelated-service:\n    image: example/unrelated:latest\n';
    const secretContent = 'cpamp_existing_admin_key\n';
    const cpaSentinels = new Map([
      ['cliproxyapi/config.yaml', 'existing-cpa-config'],
      ['cliproxyapi/auths/account.json', 'existing-cpa-auth'],
      ['cliproxyapi/logs/service.log', 'existing-cpa-log'],
      ['secrets/cpa-management-key', 'existing-cpa-management-key'],
    ]);

    try {
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(path.join(installDir, '.env'), envContent);
      writeFileSync(path.join(installDir, 'compose.yaml'), composeContent);
      writeFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), secretContent);
      for (const [relative, value] of cpaSentinels) {
        const target = path.join(installDir, relative);
        mkdirSync(path.dirname(target), { recursive: true });
        writeFileSync(target, value);
      }
      writeFakeDocker(fakeBin);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'upgrade',
          COMPOSE_PROJECT_NAME: 'wrong-project',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_LOG: dockerLog,
          FAKE_DOCKER_AUTH_OK: '1',
          FAKE_CPAMP_INTERNAL_PORT: '18137',
          FAKE_CPAMP_PANEL_BASE_PATH: '/admin',
          FAKE_CPAMP_PUBLIC_PORT: '18317',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      expect(readFileSync(path.join(installDir, '.env'), 'utf8')).toBe(envContent);
      expect(readFileSync(path.join(installDir, 'compose.yaml'), 'utf8')).toBe(composeContent);
      expect(readFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), 'utf8')).toBe(
        secretContent
      );
      for (const [relative, value] of cpaSentinels) {
        expect(readFileSync(path.join(installDir, relative), 'utf8')).toBe(value);
      }
      const dockerCalls = readFileSync(dockerLog, 'utf8');
      expect(dockerCalls).toContain('compose pull cpa-manager-plus');
      expect(dockerCalls).toContain('compose stop -t 45 cpa-manager-plus');
      expect(dockerCalls).toContain('compose up -d cpa-manager-plus');
      expect(dockerCalls).not.toContain('compose pull unrelated-service');
      expect(dockerCalls).not.toContain('compose up -d unrelated-service');
      expect(dockerCalls).not.toContain('reset-admin-key');
      expect(dockerCalls).toContain('cpamp|compose pull cpa-manager-plus');
      expect(dockerCalls).toContain(
        'compose exec -T cpa-manager-plus cat /data/runtime/state.json'
      );
      expect(result.stdout).toContain('CPAMP compatibility port: 18317');
      expect(result.stdout).toContain('Open panel: http://127.0.0.1:18317/admin');
      expect(result.stdout).toContain(
        'The preserved Compose file does not declare CPAMP_PANEL_PORT'
      );
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
      rmSync(dockerLog, { force: true });
    }
  });

  it('does not replay a consumed bootstrap token from retained Docker logs', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFileSync(
        path.join(installDir, '.env'),
        'CPAMP_IMAGE=example/cpamp:v1\nCPAMP_PORT=18317\nCPAMP_PANEL_PORT=18137\n'
      );
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    image: ${CPAMP_IMAGE}\n'
      );
      writeFakeDocker(fakeBin);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_CPAMP_BOOTSTRAP_REQUIRED: 'false',
          FAKE_CPAMP_LOGS:
            '2026-07-29 CPA Manager Plus one-time bootstrap token: bootstrap_stale_token',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status, combinedOutput(result)).toBe(0);
      expect(result.stdout).not.toContain('bootstrap_stale_token');
      expect(result.stdout).not.toContain('If no token is shown');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('shows only the latest bootstrap token when the running service still requires it', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      writeFileSync(
        path.join(installDir, '.env'),
        'CPAMP_IMAGE=example/cpamp:v1\nCPAMP_PORT=18317\nCPAMP_PANEL_PORT=18137\n'
      );
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    image: ${CPAMP_IMAGE}\n'
      );
      writeFakeDocker(fakeBin);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_CPAMP_BOOTSTRAP_REQUIRED: 'true',
          FAKE_CPAMP_LOGS: [
            '2026-07-29 CPA Manager Plus one-time bootstrap token: bootstrap_old_token',
            '2026-07-30 CPA Manager Plus one-time bootstrap token: bootstrap_current_token',
          ].join('\n'),
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status, combinedOutput(result)).toBe(0);
      expect(result.stdout).toContain('One-time bootstrap token: bootstrap_current_token');
      expect(result.stdout).not.toContain('bootstrap_old_token');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('restores the previous Docker image when an upgraded container stays unhealthy', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const dockerLog = path.join(
      os.tmpdir(),
      `cpamp-installer-docker-${process.pid}-${Date.now()}.log`
    );
    const rollbackMarker = path.join(
      os.tmpdir(),
      `cpamp-installer-rollback-${process.pid}-${Date.now()}`
    );
    const envContent =
      'COMPOSE_PROJECT_NAME=cpamp\nCPAMP_IMAGE=example/cpamp:latest\nCPAMP_PORT=18317\n';
    const composeContent = 'services:\n  cpa-manager-plus:\n    image: ${CPAMP_IMAGE}\n';

    try {
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(path.join(installDir, '.env'), envContent);
      writeFileSync(path.join(installDir, 'compose.yaml'), composeContent);
      writeFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), 'cpamp_existing_admin_key\n');
      writeFakeDocker(fakeBin);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          CPAMP_DOCKER_HEALTH_ATTEMPTS: '1',
          FAKE_DOCKER_LOG: dockerLog,
          FAKE_CPAMP_PREVIOUS_IMAGE_ID: 'sha256:previous-cpamp',
          FAKE_DOCKER_HEALTH_FAIL_UNTIL_ROLLBACK: '1',
          FAKE_DOCKER_ROLLBACK_MARKER: rollbackMarker,
          FAKE_CPAMP_INTERNAL_PORT: '18317',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain(
        'The previous image was restored and passed the health check.'
      );
      expect(readFileSync(path.join(installDir, '.env'), 'utf8')).toBe(envContent);
      expect(readFileSync(path.join(installDir, 'compose.yaml'), 'utf8')).toBe(composeContent);
      const calls = readFileSync(dockerLog, 'utf8');
      expect(calls).toContain('compose images -q cpa-manager-plus');
      expect(calls).toContain('compose stop -t 45 cpa-manager-plus');
      expect(calls).toContain('image tag sha256:previous-cpamp example/cpamp:latest');
      expect(calls).toContain('compose up -d --force-recreate --pull never cpa-manager-plus');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
      rmSync(dockerLog, { force: true });
      rmSync(rollbackMarker, { force: true });
    }
  });

  it('stops before pulling when no previous Docker image is available for rollback', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const dockerLog = path.join(
      os.tmpdir(),
      `cpamp-installer-docker-${process.pid}-${Date.now()}.log`
    );

    try {
      writeFileSync(
        path.join(installDir, '.env'),
        'COMPOSE_PROJECT_NAME=cpamp\nCPAMP_IMAGE=example/cpamp:latest\nCPAMP_PORT=18317\n'
      );
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    image: ${CPAMP_IMAGE}\n'
      );
      writeFakeDocker(fakeBin);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_LOG: dockerLog,
          FAKE_CPAMP_PREVIOUS_IMAGE_ID: '',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain(
        'The installer stopped before pulling to preserve rollback safety.'
      );
      const calls = readFileSync(dockerLog, 'utf8');
      expect(calls).toContain('compose images -q cpa-manager-plus');
      expect(calls).not.toContain('compose pull');
      expect(calls).not.toContain('compose stop');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
      rmSync(dockerLog, { force: true });
    }
  });

  it('stops before pulling when the preserved Compose image uses a digest reference', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const dockerLog = path.join(
      os.tmpdir(),
      `cpamp-installer-docker-${process.pid}-${Date.now()}.log`
    );

    try {
      writeFileSync(
        path.join(installDir, '.env'),
        'COMPOSE_PROJECT_NAME=cpamp\nCPAMP_IMAGE=example/cpamp:latest\nCPAMP_PORT=18317\n'
      );
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    image: example/cpamp@sha256:1234\n'
      );
      writeFakeDocker(fakeBin);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_LOG: dockerLog,
          FAKE_CPAMP_COMPOSE_IMAGE: 'example/cpamp@sha256:1234',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('digest reference example/cpamp@sha256:1234');
      const calls = readFileSync(dockerLog, 'utf8');
      expect(calls).not.toContain('compose pull');
      expect(calls).not.toContain('compose stop');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
      rmSync(dockerLog, { force: true });
    }
  });

  it('repairs a managed Docker login without pulling unrelated service images', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const dockerLog = path.join(
      os.tmpdir(),
      `cpamp-installer-docker-${process.pid}-${Date.now()}.log`
    );

    try {
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(
        path.join(installDir, '.env'),
        'COMPOSE_PROJECT_NAME=cpamp\nCPAMP_IMAGE=example/cpamp:v1\nCPAMP_PORT=18317\n'
      );
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    image: ${CPAMP_IMAGE}\n'
      );
      writeFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), 'cpamp_existing_admin_key\n');
      writeFakeDocker(fakeBin);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'repair',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_LOG: dockerLog,
          FAKE_DOCKER_AUTH_OK: '1',
          FAKE_CPAMP_INTERNAL_PORT: '18317',
          FAKE_CPAMP_PUBLIC_PORT: '18317',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      const calls = readFileSync(dockerLog, 'utf8');
      expect(calls).toContain('compose run --rm -v ');
      expect(calls).toContain(
        'cpa-manager-plus reset-admin-key --admin-key-file /run/secrets/cpamp_admin_key'
      );
      expect(calls).not.toContain('compose pull');
      expect(result.stdout).toContain('Open panel: http://127.0.0.1:18317/management.html');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
      rmSync(dockerLog, { force: true });
    }
  });

  it('does not report success when post-start admin key verification fails', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(
        path.join(installDir, '.env'),
        'COMPOSE_PROJECT_NAME=cpamp\nCPAMP_IMAGE=example/cpamp:v1\nCPAMP_PORT=18317\n'
      );
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    image: ${CPAMP_IMAGE}\n'
      );
      writeFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), 'cpamp_wrong_admin_key\n');
      writeFakeDocker(fakeBin);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          FAKE_DOCKER_AUTH_OK: '0',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('admin key verification failed');
      expect(result.stdout).not.toContain('Install steps completed');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('backs up generated config before regenerating a managed Docker install', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const oldEnv = 'COMPOSE_PROJECT_NAME=cpamp\nCPAMP_IMAGE=example/old:v1\nCPAMP_PORT=18317\n';
    const oldCompose = 'services:\n  cpa-manager-plus:\n    image: old\n';

    try {
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(path.join(installDir, '.env'), oldEnv);
      writeFileSync(path.join(installDir, 'compose.yaml'), oldCompose);
      writeFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), 'cpamp_existing_admin_key\n');

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'regenerate',
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_CPA_CONNECTION_MODE: 'setup',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      const backupNames = readdirSync(path.join(installDir, 'backups'));
      expect(backupNames).toHaveLength(1);
      const backupDir = path.join(installDir, 'backups', backupNames[0]);
      expect(readFileSync(path.join(backupDir, '.env'), 'utf8')).toBe(oldEnv);
      expect(readFileSync(path.join(backupDir, 'compose.yaml'), 'utf8')).toBe(oldCompose);
      expect(readFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), 'utf8')).toBe(
        'cpamp_existing_admin_key\n'
      );
      expect(readFileSync(path.join(installDir, '.env'), 'utf8')).toContain(
        'CPAMP_IMAGE=example/old:v1'
      );
      expect(readFileSync(path.join(installDir, '.env'), 'utf8')).toContain('CPAMP_PORT=18317');
      expect(result.stdout).toContain('Previous config backed up to');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('backs up and replaces an explicitly changed CPA key during Docker regeneration', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const oldKey = 'cpa_old_management_key';
    const newKey = 'cpa_new_management_key';

    try {
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(
        path.join(installDir, '.env'),
        [
          'COMPOSE_PROJECT_NAME=cpamp',
          'CPAMP_IMAGE=example/cpamp:v1',
          'CPAMP_API_PORT=8137',
          'CPAMP_PANEL_PORT=18137',
          'CPAMP_PORT=18317',
          'CPA_MANAGER_DEPLOYMENT_MODE=installer-managed',
          'CPA_UPSTREAM_URL=http://old-cpa:8317',
          '',
        ].join('\n')
      );
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    environment:\n      CPA_MANAGEMENT_KEY_FILE: /run/secrets/cpa_management_key\n'
      );
      writeFileSync(path.join(installDir, 'secrets/cpa-management-key'), `${oldKey}\n`);
      writeFakeCPACurl(fakeBin);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'regenerate',
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_CPA_CONNECTION_MODE: 'env',
          CPAMP_CPA_URL: 'http://new-cpa:8317',
          CPAMP_CPA_MANAGEMENT_KEY: newKey,
          CPAMP_INSTALL_DIR: installDir,
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status, combinedOutput(result)).toBe(0);
      expect(readFileSync(path.join(installDir, 'secrets/cpa-management-key'), 'utf8')).toBe(
        `${newKey}\n`
      );
      const backupNames = readdirSync(path.join(installDir, 'backups'));
      expect(backupNames).toHaveLength(1);
      expect(
        readFileSync(
          path.join(installDir, 'backups', backupNames[0], 'secrets/cpa-management-key'),
          'utf8'
        )
      ).toBe(`${oldKey}\n`);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('blocks a partial Docker install before writing additional generated files', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));

    try {
      writeFileSync(path.join(installDir, 'compose.yaml'), 'existing compose\n');

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_CPA_CONNECTION_MODE: 'setup',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('Non-interactive mode requires CPAMP_OPERATION');
      expect(existsSync(path.join(installDir, '.env'))).toBe(false);
      expect(existsSync(path.join(installDir, 'secrets/cpamp-admin-key'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('does not change existing admin-secret permissions during dry runs', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const secretFile = path.join(installDir, 'secrets/cpamp-admin-key');

    try {
      mkdirSync(path.dirname(secretFile), { recursive: true });
      writeFileSync(
        path.join(installDir, '.env'),
        'COMPOSE_PROJECT_NAME=cpamp\nCPAMP_IMAGE=example/cpamp:v1\nCPAMP_PORT=18317\n'
      );
      writeFileSync(
        path.join(installDir, 'compose.yaml'),
        'services:\n  cpa-manager-plus:\n    image: ${CPAMP_IMAGE}\n'
      );
      writeFileSync(secretFile, 'cpamp_existing_admin_key\n');
      chmodSync(secretFile, 0o644);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_DRY_RUN: '1',
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      expect(statSync(secretFile).mode & 0o777).toBe(0o644);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('rejects empty existing secret files before generating config', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));

    try {
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), '');

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'stack',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('secrets/cpamp-admin-key must not be empty');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('fails when existing secret file permissions cannot be restricted', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));

    try {
      mkdirSync(path.join(installDir, 'secrets'), { recursive: true });
      writeFileSync(path.join(installDir, 'secrets/cpamp-admin-key'), 'cpamp_existing_admin_key\n');
      const fakeChmod = path.join(fakeBin, 'chmod');
      writeFileSync(fakeChmod, '#!/usr/bin/env bash\nexit 1\n');
      chmodSync(fakeChmod, 0o755);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'stack',
          CPAMP_DEPLOY_METHOD: 'docker',
          CPAMP_INSTALL_DIR: installDir,
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('Unable to restrict secret file permissions');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('requires an explicit operation for an existing native deployment in non-interactive mode', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-'));

    try {
      writeFileSync(
        path.join(installDir, 'run.sh'),
        '#!/usr/bin/env bash\nexec ./cpa-manager-plus\n'
      );

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_DRY_RUN: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain(
        'An existing native deployment was detected. Non-interactive mode requires CPAMP_OPERATION=upgrade or regenerate.'
      );
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('refuses to stop an unrelated native process from a recycled PID file', async () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const oldRuntime = path.join(installDir, 'runtime/cpa-manager-plus_v1.8.1_legacy');
    const basePort = 30000 + (process.pid % 10000);
    const oldRunScript = [
      '#!/usr/bin/env bash',
      `export CPA_MANAGER_GATEWAY_ADDRS=0.0.0.0:${basePort},0.0.0.0:${basePort + 1},0.0.0.0:${basePort + 2}`,
      `cd ${JSON.stringify(oldRuntime)}`,
      'exec ./cpa-manager-plus runtime',
      '',
    ].join('\n');
    const unrelated = spawn('bash', ['-c', 'exec -a cpa-manager-plus sleep 30'], {
      cwd: os.tmpdir(),
      stdio: 'ignore',
    });

    try {
      await new Promise((resolve) => setTimeout(resolve, 50));
      expect(unrelated.pid).toBeGreaterThan(0);
      mkdirSync(oldRuntime, { recursive: true });
      writeFileSync(path.join(oldRuntime, 'cpa-manager-plus'), 'old-binary');
      writeFileSync(path.join(installDir, 'run.sh'), oldRunScript);
      writeFileSync(path.join(installDir, 'cpa-manager-plus.pid'), `${unrelated.pid}\n`);
      writeFileSync(path.join(fakeBin, 'curl'), '#!/usr/bin/env bash\nexit 22\n');
      chmodSync(path.join(fakeBin, 'curl'), 0o755);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_VERSION: 'v1.12.0',
          CPAMP_INSTALL_DIR: installDir,
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain(
        'A process outside the installer is using the previous port.'
      );
      expect(spawnSync('kill', ['-0', String(unrelated.pid)]).status).toBe(0);
      expect(readFileSync(path.join(installDir, 'run.sh'), 'utf8')).toBe(oldRunScript);
    } finally {
      unrelated.kill('SIGTERM');
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('recognizes a metadata-recorded native process after handoff into the managed CPAMP component root', () => {
    if (process.platform === 'win32') {
      return;
    }
    const sleepBinary = ['/bin/sleep', '/usr/bin/sleep'].find((candidate) => existsSync(candidate));
    if (!sleepBinary) {
      return;
    }

    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-handoff-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const oldRuntime = path.join(installDir, 'runtime/cpa-manager-plus_v1.11.0_legacy');
    const managedBinary = path.join(
      installDir,
      'data/runtime/components/cpamp/v1.12.0/cpa-manager-plus'
    );
    const processPathFile = path.join(installDir, 'reported-process-path.txt');
    const basePort = 34000 + (process.pid % 10000);
    let managedPid = 0;
    let restartedPid = 0;

    try {
      mkdirSync(oldRuntime, { recursive: true });
      mkdirSync(path.dirname(managedBinary), { recursive: true });
      writeFileSync(
        path.join(oldRuntime, 'cpa-manager-plus'),
        '#!/usr/bin/env bash\nexec sleep 30\n'
      );
      chmodSync(path.join(oldRuntime, 'cpa-manager-plus'), 0o755);
      writeFileSync(
        path.join(installDir, 'run.sh'),
        [
          '#!/usr/bin/env bash',
          'set -euo pipefail',
          'export CPA_MANAGER_DEPLOYMENT_MODE=slim',
          `export CPA_MANAGER_GATEWAY_ADDRS=0.0.0.0:${basePort},0.0.0.0:${basePort + 1},0.0.0.0:${basePort + 2}`,
          `cd ${JSON.stringify(oldRuntime)}`,
          'exec ./cpa-manager-plus runtime',
          '',
        ].join('\n')
      );
      chmodSync(path.join(installDir, 'run.sh'), 0o755);
      writeFileSync(managedBinary, 'managed-component');
      writeFileSync(processPathFile, `${managedBinary}\n`);
      managedPid = startDetachedSleep(sleepBinary);
      const start = processStartMarker(managedPid);
      expect(start).not.toBe('');
      writeFileSync(
        path.join(installDir, 'cpa-manager-plus.pid'),
        `pid=${managedPid}\nstart=${start}\n`
      );
      writeFileSync(path.join(fakeBin, 'curl'), '#!/usr/bin/env bash\nexit 0\n');
      chmodSync(path.join(fakeBin, 'curl'), 0o755);
      writeInstallerProcessPathFakes(fakeBin, processPathFile);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'regenerate',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          CPAMP_DEPLOY_METHOD: 'native',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_NATIVE_HEALTH_ATTEMPTS: '1',
          CPAMP_TEST_PROCESS_PATH_FILE: processPathFile,
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status, combinedOutput(result)).toBe(0);
      expect(combinedOutput(result)).not.toContain(
        'A process outside the installer is using the previous port.'
      );
      expect(spawnSync('kill', ['-0', String(managedPid)]).status).not.toBe(0);
      restartedPid = readInstallerPid(installDir);
      expect(restartedPid).toBeGreaterThan(0);
      expect(restartedPid).not.toBe(managedPid);
    } finally {
      if (restartedPid > 0) {
        spawnSync('kill', ['-TERM', String(restartedPid)]);
      }
      if (managedPid > 0) {
        spawnSync('kill', ['-TERM', String(managedPid)]);
      }
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('rejects a metadata-recorded native process outside the managed CPAMP component root', () => {
    if (process.platform === 'win32') {
      return;
    }
    const sleepBinary = ['/bin/sleep', '/usr/bin/sleep'].find((candidate) => existsSync(candidate));
    if (!sleepBinary) {
      return;
    }

    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-reject-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const oldRuntime = path.join(installDir, 'runtime/cpa-manager-plus_v1.11.0_legacy');
    const outsideBinary = path.join(
      installDir,
      'data/runtime/components/cpa/v1.12.0/cpa-manager-plus'
    );
    const processPathFile = path.join(installDir, 'reported-process-path.txt');
    const basePort = 35000 + (process.pid % 10000);
    let outsidePid = 0;

    try {
      mkdirSync(oldRuntime, { recursive: true });
      mkdirSync(path.dirname(outsideBinary), { recursive: true });
      writeFileSync(path.join(oldRuntime, 'cpa-manager-plus'), 'old-binary');
      writeFileSync(
        path.join(installDir, 'run.sh'),
        [
          '#!/usr/bin/env bash',
          'export CPA_MANAGER_DEPLOYMENT_MODE=slim',
          `export CPA_MANAGER_GATEWAY_ADDRS=0.0.0.0:${basePort},0.0.0.0:${basePort + 1},0.0.0.0:${basePort + 2}`,
          `cd ${JSON.stringify(oldRuntime)}`,
          'exec ./cpa-manager-plus runtime',
          '',
        ].join('\n')
      );
      writeFileSync(outsideBinary, 'outside-component');
      writeFileSync(processPathFile, `${outsideBinary}\n`);
      outsidePid = startDetachedSleep(sleepBinary);
      const start = processStartMarker(outsidePid);
      expect(start).not.toBe('');
      writeFileSync(
        path.join(installDir, 'cpa-manager-plus.pid'),
        `pid=${outsidePid}\nstart=${start}\n`
      );
      writeFileSync(path.join(fakeBin, 'curl'), '#!/usr/bin/env bash\nexit 22\n');
      chmodSync(path.join(fakeBin, 'curl'), 0o755);
      writeInstallerProcessPathFakes(fakeBin, processPathFile);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'regenerate',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
          CPAMP_DEPLOY_METHOD: 'native',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_TEST_PROCESS_PATH_FILE: processPathFile,
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain(
        'A process outside the installer is using the previous port.'
      );
      expect(spawnSync('kill', ['-0', String(outsidePid)]).status).toBe(0);
    } finally {
      if (outsidePid > 0) {
        spawnSync('kill', ['-TERM', String(outsidePid)]);
      }
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
    }
  });

  it('restores and restarts the previous native runtime when the upgraded process stays unhealthy', async () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const fixtureDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-fixture-'));
    const platform = process.platform === 'darwin' ? 'darwin' : 'linux';
    const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
    const oldRuntime = path.join(installDir, 'runtime/cpa-manager-plus_v1.11.0_legacy');
    const packageName = `cpa-manager-plus_vtest_${platform}_${arch}_slim`;
    const packageDir = path.join(fixtureDir, packageName);
    const archivePath = path.join(fixtureDir, `${packageName}.tar.gz`);
    const basePort = 32000 + (process.pid % 10000);
    const oldRunScript = [
      '#!/usr/bin/env bash',
      'set -euo pipefail',
      'export CPA_MANAGER_DEPLOYMENT_MODE=slim',
      `export CPA_MANAGER_GATEWAY_ADDRS=0.0.0.0:${basePort},0.0.0.0:${basePort + 1},0.0.0.0:${basePort + 2}`,
      `cd ${JSON.stringify(oldRuntime)}`,
      'exec ./cpa-manager-plus 60',
      '',
    ].join('\n');
    let oldPid = 0;
    let restoredPid = 0;
    let supervisorPid = 0;

    try {
      mkdirSync(oldRuntime, { recursive: true });
      symlinkSync('/bin/sleep', path.join(oldRuntime, 'cpa-manager-plus'));
      writeFileSync(path.join(installDir, 'run.sh'), oldRunScript);
      chmodSync(path.join(installDir, 'run.sh'), 0o755);

      const supervisor = spawn(
        'bash',
        [
          '-c',
          '"$1" >/dev/null 2>&1 & child=$!; printf "%s\\n" "$child" > "$2"; wait "$child"',
          'cpamp-native-test',
          path.join(installDir, 'run.sh'),
          path.join(installDir, 'cpa-manager-plus.pid'),
        ],
        { cwd: installDir, detached: true, stdio: 'ignore' }
      );
      supervisorPid = supervisor.pid || 0;
      supervisor.unref();
      for (let attempt = 0; attempt < 20; attempt += 1) {
        if (existsSync(path.join(installDir, 'cpa-manager-plus.pid'))) {
          break;
        }
        await new Promise((resolve) => setTimeout(resolve, 25));
      }
      oldPid = readInstallerPid(installDir);
      expect(oldPid).toBeGreaterThan(0);
      expect(spawnSync('kill', ['-0', String(oldPid)]).status).toBe(0);

      mkdirSync(packageDir, { recursive: true });
      symlinkSync('/bin/cat', path.join(packageDir, 'cpa-manager-plus'));
      const fifoResult = spawnSync('mkfifo', [path.join(packageDir, 'runtime')], {
        cwd: repoRoot,
        encoding: 'utf8',
      });
      expect(fifoResult.status, combinedOutput(fifoResult)).toBe(0);
      const tarResult = spawnSync('tar', ['-czf', archivePath, '-C', fixtureDir, packageName], {
        cwd: repoRoot,
        encoding: 'utf8',
      });
      expect(tarResult.status, combinedOutput(tarResult)).toBe(0);

      writeFileSync(
        path.join(fakeBin, 'curl'),
        `#!/usr/bin/env bash
set -euo pipefail
url=""
out=""
previous=""
for arg in "$@"; do
  if [ "$previous" = "-o" ]; then
    out="$arg"
  fi
  case "$arg" in
    http://*|https://*) url="$arg" ;;
  esac
  previous="$arg"
done
if [ -n "$out" ]; then
  if [ "\${url##*/}" = "checksums.txt" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      checksum="$(sha256sum "$CPAMP_FAKE_NATIVE_ARCHIVE" | awk '{print $1}')"
    else
      checksum="$(shasum -a 256 "$CPAMP_FAKE_NATIVE_ARCHIVE" | awk '{print $1}')"
    fi
    printf '%s  %s\n' "$checksum" "$CPAMP_FAKE_NATIVE_ASSET_NAME" > "$out"
  else
    cp "$CPAMP_FAKE_NATIVE_ARCHIVE" "$out"
  fi
  exit 0
fi
case "$url" in
  http://127.0.0.1:*/health)
    ! grep -q 'cpa-manager-plus_vtest_' "$CPAMP_FAKE_NATIVE_RUN_SCRIPT"
    ;;
  *) exit 22 ;;
esac
`
      );
      chmodSync(path.join(fakeBin, 'curl'), 0o755);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_VERSION: 'vtest',
          CPAMP_INSTALL_DIR: installDir,
          CPAMP_FAKE_NATIVE_ARCHIVE: archivePath,
          CPAMP_FAKE_NATIVE_ASSET_NAME: `${packageName}.tar.gz`,
          CPAMP_FAKE_NATIVE_RUN_SCRIPT: path.join(installDir, 'run.sh'),
          CPAMP_NATIVE_HEALTH_ATTEMPTS: '1',
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status, combinedOutput(result)).toBe(1);
      expect(combinedOutput(result)).toContain(
        'The new version did not become healthy. The previous run script was restored and the old version was restarted.'
      );
      expect(readFileSync(path.join(installDir, 'run.sh'), 'utf8')).toBe(oldRunScript);
      restoredPid = readInstallerPid(installDir);
      expect(restoredPid).toBeGreaterThan(0);
      expect(restoredPid).not.toBe(oldPid);
      expect(spawnSync('kill', ['-0', String(restoredPid)]).status).toBe(0);
    } finally {
      if (restoredPid > 0) {
        spawnSync('kill', ['-TERM', String(restoredPid)]);
      }
      if (oldPid > 0) {
        spawnSync('kill', ['-TERM', String(oldPid)]);
      }
      if (supervisorPid > 0) {
        spawnSync('kill', ['-TERM', String(supervisorPid)]);
      }
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
      rmSync(fixtureDir, { recursive: true, force: true });
    }
  });

  it('plans an in-place Slim upgrade for a legacy native deployment', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-'));
    const platform = process.platform === 'darwin' ? 'darwin' : 'linux';
    const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';

    try {
      mkdirSync(path.join(installDir, 'data'), { recursive: true });
      writeFileSync(
        path.join(installDir, 'run.sh'),
        '#!/usr/bin/env bash\ncd /opt/cpamp/runtime/legacy\nexec ./cpa-manager-plus\n'
      );
      writeFileSync(path.join(installDir, 'data/usage.sqlite'), 'legacy-db');

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_DRY_RUN: '1',
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_VERSION: 'v1.12.0',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      expect(result.stdout).toContain('Operation: Upgrade existing deployment');
      expect(result.stdout).toContain(`cpa-manager-plus_v1.12.0_${platform}_${arch}_slim.tar.gz`);
      expect(result.stdout).toContain(
        'CPA connection: preserve the existing connection for automatic migration'
      );
      expect(result.stdout).toContain('stop the installer-managed native process if it is running');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('regenerates native startup files with the active package and plans a managed restart', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-'));
    const oldRuntime = path.join(installDir, 'runtime/cpa-manager-plus_v1.8.1_legacy');

    try {
      mkdirSync(oldRuntime, { recursive: true });
      writeFileSync(path.join(oldRuntime, 'cpa-manager-plus'), 'old-binary');
      writeFileSync(
        path.join(installDir, 'run.sh'),
        `#!/usr/bin/env bash\nexport CPA_MANAGER_DEPLOYMENT_MODE=slim\ncd ${JSON.stringify(oldRuntime)}\nexec ./cpa-manager-plus\n`
      );

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_DRY_RUN: '1',
          CPAMP_OPERATION: 'regenerate',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status, combinedOutput(result)).toBe(0);
      expect(result.stdout).toContain('Operation: Regenerate deployment config');
      expect(result.stdout).toContain('stop the installer-managed native process if it is running');
      expect(result.stdout).not.toContain('/releases/download/');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('preserves native databases, keys, CPA state, and the previous runtime during upgrade', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-'));
    const oldRuntime = path.join(installDir, 'runtime/cpa-manager-plus_v1.8.1_legacy');
    const sentinels = new Map([
      ['data/usage.sqlite', 'db'],
      ['data/usage.sqlite-wal', 'wal'],
      ['data/usage.sqlite-shm', 'shm'],
      ['data/data.key', 'data-key'],
      ['data/cpa/config.yaml', 'cpa-config'],
      ['data/cpa/auths/account.json', 'cpa-auth'],
      ['data/cpa/logs/service.log', 'cpa-log'],
      ['secrets/cpamp-admin-key', 'cpamp-existing-admin-key'],
    ]);

    try {
      mkdirSync(oldRuntime, { recursive: true });
      writeFileSync(path.join(oldRuntime, 'cpa-manager-plus'), 'old-binary');
      writeFileSync(
        path.join(installDir, 'run.sh'),
        `#!/usr/bin/env bash\ncd ${JSON.stringify(oldRuntime)}\nexec ./cpa-manager-plus\n`
      );
      for (const [relative, value] of sentinels) {
        const target = path.join(installDir, relative);
        mkdirSync(path.dirname(target), { recursive: true });
        writeFileSync(target, value);
      }

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_VERSION: 'v1.12.0',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);
      for (const [relative, value] of sentinels) {
        expect(readFileSync(path.join(installDir, relative), 'utf8')).toBe(value);
      }
      expect(readFileSync(path.join(oldRuntime, 'cpa-manager-plus'), 'utf8')).toBe('old-binary');
      expect(readFileSync(path.join(installDir, 'run.sh'), 'utf8')).toContain(
        'exec ./cpa-manager-plus runtime'
      );
      expect(readFileSync(path.join(installDir, '.cpamp-native.env'), 'utf8')).toContain(
        'CPA_MANAGER_DEPLOYMENT_MODE=slim'
      );
      const backupNames = readdirSync(path.join(installDir, 'backups'));
      expect(backupNames).toHaveLength(1);
      expect(
        readFileSync(path.join(installDir, 'backups', backupNames[0], 'run.sh'), 'utf8')
      ).toContain('exec ./cpa-manager-plus');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('allows retrying a native upgrade when the same-version target is not active', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-'));
    const platform = process.platform === 'darwin' ? 'darwin' : 'linux';
    const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
    const oldRuntime = path.join(installDir, 'runtime/cpa-manager-plus_v1.11.0_legacy');
    const retryRuntime = path.join(
      installDir,
      `runtime/cpa-manager-plus_v1.12.0_${platform}_${arch}_slim`
    );

    try {
      mkdirSync(oldRuntime, { recursive: true });
      mkdirSync(retryRuntime, { recursive: true });
      writeFileSync(path.join(oldRuntime, 'cpa-manager-plus'), 'old-binary');
      writeFileSync(path.join(retryRuntime, 'partial-download'), 'failed-attempt');
      writeFileSync(
        path.join(installDir, 'run.sh'),
        `#!/usr/bin/env bash\ncd ${JSON.stringify(oldRuntime)}\nexec ./cpa-manager-plus\n`
      );

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_OPERATION: 'upgrade',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_VERSION: 'v1.12.0',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status, combinedOutput(result)).toBe(0);
      expect(readFileSync(path.join(retryRuntime, 'partial-download'), 'utf8')).toBe(
        'failed-attempt'
      );
      expect(readFileSync(path.join(installDir, 'run.sh'), 'utf8')).toContain(retryRuntime);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('preserves custom native database paths using metadata, run script, then legacy config priority', () => {
    const variants = [
      { source: 'metadata', expectedName: 'metadata db/usage.sqlite' },
      { source: 'run', expectedName: 'run-db/usage.sqlite' },
      { source: 'config', expectedName: 'config-db/usage.sqlite' },
    ];

    for (const variant of variants) {
      const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-native-db-'));
      const oldRuntime = path.join(installDir, 'runtime/cpa-manager-plus_v1.8.1_legacy');
      const metadataDB = path.join(installDir, 'metadata db/usage.sqlite');
      const runDB = path.join(installDir, 'run-db/usage.sqlite');
      const configDB = path.join(installDir, 'config-db/usage.sqlite');
      const expectedDB = path.join(installDir, variant.expectedName);

      try {
        mkdirSync(oldRuntime, { recursive: true });
        for (const dbPath of [metadataDB, runDB, configDB]) {
          mkdirSync(path.dirname(dbPath), { recursive: true });
          writeFileSync(dbPath, path.basename(path.dirname(dbPath)));
        }
        const runLines = ['#!/usr/bin/env bash', 'export CPA_MANAGER_DEPLOYMENT_MODE=slim'];
        if (variant.source !== 'config') {
          runLines.push(`export USAGE_DB_PATH=${runDB}`);
        }
        runLines.push(`cd ${oldRuntime}`, 'exec ./cpa-manager-plus', '');
        writeFileSync(path.join(installDir, 'run.sh'), runLines.join('\n'));
        writeFileSync(
          path.join(oldRuntime, 'config.json'),
          `${JSON.stringify({ dbPath: configDB }, null, 2)}\n`
        );
        if (variant.source === 'metadata') {
          writeFileSync(
            path.join(installDir, '.cpamp-native.env'),
            `USAGE_DB_PATH=${metadataDB}\n`
          );
        }

        const result = spawnSync('bash', [installerPath], {
          cwd: repoRoot,
          env: {
            ...process.env,
            CPAMP_SKIP_EXECUTE: '1',
            CPAMP_OPERATION: 'upgrade',
            CPAMP_NON_INTERACTIVE: '1',
            CPAMP_CONFIRM: '1',
            CPAMP_LANG: 'en-US',
            CPAMP_VERSION: 'v1.12.0',
            CPAMP_INSTALL_DIR: installDir,
          },
          encoding: 'utf8',
        });

        expect(result.status, combinedOutput(result)).toBe(0);
        const runScript = readFileSync(path.join(installDir, 'run.sh'), 'utf8');
        const metadata = readFileSync(path.join(installDir, '.cpamp-native.env'), 'utf8');
        expect(runScript).toContain(`export USAGE_DB_PATH=${expectedDB.replaceAll(' ', '\\ ')}`);
        expect(metadata).toContain(`USAGE_DB_PATH=${expectedDB}`);
        expect(readFileSync(expectedDB, 'utf8')).not.toBe('');
      } finally {
        rmSync(installDir, { recursive: true, force: true });
      }
    }
  });

  it('does not leave partial native files when the runtime directory already exists', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const platform = process.platform === 'darwin' ? 'darwin' : 'linux';
    const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
    const packageName = `cpa-manager-plus_v1.8.1_${platform}_${arch}_slim`;

    try {
      mkdirSync(path.join(installDir, 'runtime', packageName), { recursive: true });

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_DEPLOY_METHOD: 'native',
          CPAMP_CPA_CONNECTION_MODE: 'setup',
          CPAMP_VERSION: 'v1.8.1',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('Directory already exists');
      expect(existsSync(path.join(installDir, 'secrets/cpamp-admin-key'))).toBe(false);
      expect(existsSync(path.join(installDir, 'run.sh'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });

  it('fails native installs when the started process exits before health is ready', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const fixtureDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-fixture-'));
    const platform = process.platform === 'darwin' ? 'darwin' : 'linux';
    const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
    const packageName = `cpa-manager-plus_vtest_${platform}_${arch}_slim`;
    const packageDir = path.join(fixtureDir, packageName);
    const archivePath = path.join(fixtureDir, `${packageName}.tar.gz`);

    try {
      mkdirSync(packageDir, { recursive: true });
      const fakeBinary = path.join(packageDir, 'cpa-manager-plus');
      writeFileSync(
        fakeBinary,
        '#!/usr/bin/env bash\necho "fake native process exited" >&2\nexit 42\n'
      );
      chmodSync(fakeBinary, 0o755);
      const tarResult = spawnSync('tar', ['-czf', archivePath, '-C', fixtureDir, packageName], {
        cwd: repoRoot,
        encoding: 'utf8',
      });
      expect(tarResult.status).toBe(0);

      const fakeCurl = path.join(fakeBin, 'curl');
      writeFileSync(
        fakeCurl,
        `#!/usr/bin/env bash
set -euo pipefail
url=""
for arg in "$@"; do
  if [ "$arg" = "https://github.com/seakee/CPA-Manager-Plus/releases/latest" ]; then
    printf 'https://github.com/seakee/CPA-Manager-Plus/releases/tag/vtest'
    exit 0
  fi
  case "$arg" in
    https://*) url="$arg" ;;
  esac
done
out=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then
    out="$arg"
    break
  fi
  prev="$arg"
done
if [ -n "$out" ]; then
	  if [ "\${url##*/}" = "checksums.txt" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      checksum="$(sha256sum "$CPAMP_FAKE_NATIVE_ARCHIVE" | awk '{print $1}')"
    else
      checksum="$(shasum -a 256 "$CPAMP_FAKE_NATIVE_ARCHIVE" | awk '{print $1}')"
    fi
    printf '%s  %s\n' "$checksum" "$CPAMP_FAKE_NATIVE_ASSET_NAME" > "$out"
    exit 0
  fi
  cp "$CPAMP_FAKE_NATIVE_ARCHIVE" "$out"
  exit 0
fi
exit 22
`
      );
      chmodSync(fakeCurl, 0o755);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '0',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_DEPLOY_METHOD: 'native',
          CPAMP_CPA_CONNECTION_MODE: 'setup',
          CPAMP_INSTALL_DIR: installDir,
          CPAMP_FAKE_NATIVE_ARCHIVE: archivePath,
          CPAMP_FAKE_NATIVE_ASSET_NAME: `${packageName}.tar.gz`,
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain(
        'Native CPAMP process exited before becoming healthy'
      );
      expect(combinedOutput(result)).toContain('fake native process exited');
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
      rmSync(fixtureDir, { recursive: true, force: true });
    }
  });

  it('rejects native release archives that do not match Release checksums', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));
    const fakeBin = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-bin-'));
    const fixtureDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-fixture-'));
    const platform = process.platform === 'darwin' ? 'darwin' : 'linux';
    const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
    const packageName = `cpa-manager-plus_vtest_${platform}_${arch}_slim`;
    const packageDir = path.join(fixtureDir, packageName);
    const archivePath = path.join(fixtureDir, `${packageName}.tar.gz`);

    try {
      mkdirSync(packageDir, { recursive: true });
      writeFileSync(path.join(packageDir, 'cpa-manager-plus'), 'archive-content');
      const tarResult = spawnSync('tar', ['-czf', archivePath, '-C', fixtureDir, packageName], {
        cwd: repoRoot,
        encoding: 'utf8',
      });
      expect(tarResult.status).toBe(0);

      const fakeCurl = path.join(fakeBin, 'curl');
      writeFileSync(
        fakeCurl,
        `#!/usr/bin/env bash
set -euo pipefail
url=""
out=""
prev=""
for arg in "$@"; do
  case "$arg" in
    https://*) url="$arg" ;;
  esac
  if [ "$prev" = "-o" ]; then
    out="$arg"
  fi
  prev="$arg"
done
if [ "\${url##*/}" = "checksums.txt" ]; then
  printf '%064d  %s\n' 0 "$CPAMP_FAKE_NATIVE_ASSET_NAME" > "$out"
else
  cp "$CPAMP_FAKE_NATIVE_ARCHIVE" "$out"
fi
`
      );
      chmodSync(fakeCurl, 0o755);

      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '0',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_DEPLOY_METHOD: 'native',
          CPAMP_CPA_CONNECTION_MODE: 'setup',
          CPAMP_VERSION: 'vtest',
          CPAMP_INSTALL_DIR: installDir,
          CPAMP_FAKE_NATIVE_ARCHIVE: archivePath,
          CPAMP_FAKE_NATIVE_ASSET_NAME: `${packageName}.tar.gz`,
          PATH: `${fakeBin}${path.delimiter}${process.env.PATH || ''}`,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(combinedOutput(result)).toContain('Native release checksum verification failed');
      expect(existsSync(path.join(installDir, 'run.sh'))).toBe(false);
    } finally {
      rmSync(installDir, { recursive: true, force: true });
      rmSync(fakeBin, { recursive: true, force: true });
      rmSync(fixtureDir, { recursive: true, force: true });
    }
  });

  it('generates a Linux systemd unit for native installs', () => {
    const installDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-installer-'));

    try {
      const result = spawnSync('bash', [installerPath], {
        cwd: repoRoot,
        env: {
          ...process.env,
          CPAMP_SKIP_EXECUTE: '1',
          CPAMP_NON_INTERACTIVE: '1',
          CPAMP_CONFIRM: '1',
          CPAMP_LANG: 'en-US',
          CPAMP_INSTALL_MODE: 'cpamp',
          CPAMP_DEPLOY_METHOD: 'native',
          CPAMP_CPA_CONNECTION_MODE: 'setup',
          CPAMP_VERSION: 'v1.8.1',
          CPAMP_INSTALL_DIR: installDir,
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(0);

      if (process.platform === 'linux') {
        const service = readFileSync(path.join(installDir, 'cpa-manager-plus.service'), 'utf8');

        expect(service).toContain('[Unit]');
        expect(service).toContain('ExecStart=');
        expect(service).toContain('/cpa-manager-plus');
      }
    } finally {
      rmSync(installDir, { recursive: true, force: true });
    }
  });
});
