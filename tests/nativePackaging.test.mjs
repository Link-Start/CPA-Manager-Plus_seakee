import { spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const packageScript = readFileSync(path.join(repoRoot, 'bin/release/package-native.sh'), 'utf8');
const dockerfile = readFileSync(path.join(repoRoot, 'Dockerfile.manager-server'), 'utf8');
const releaseWorkflow = readFileSync(path.join(repoRoot, '.github/workflows/release.yml'), 'utf8');
const manifestWorkflow = readFileSync(
  path.join(repoRoot, '.github/workflows/runtime-manifest.yml'),
  'utf8'
);

describe('native release packaging', () => {
  it('bundles the portable CPA asset in Linux Full packages', () => {
    expect(packageScript).toContain('if [ "${goos}" = "linux" ]; then');
    expect(packageScript).toContain('asset_suffix="_no-plugin"');
    expect(packageScript).toContain('${asset_arch}${asset_suffix}.${extension}');
  });

  it('builds compatibility archives with a matching top-level directory', () => {
    expect(packageScript).toContain('compatibility_dir="${work_dir}/${compatibility_name}"');
    expect(packageScript).toContain('copy_package_files "${compatibility_dir}" "${goos}" "slim"');
    expect(packageScript).toContain('archive_package "${compatibility_name}" "${goos}"');
    expect(packageScript).not.toContain('cp "${out_dir}/${slim_name}');
  });

  it('requires verified CPA assets for release Full packages and Docker images', () => {
    expect(packageScript).toContain('Full native packages require ${checksum_file}');
    expect(packageScript).toContain('sha256sum -c checksums.txt');
    expect(releaseWorkflow).toContain('sha256sum -c checksums.txt');
    expect(dockerfile).toContain('CPA checksum is missing for $asset_name');
    expect(dockerfile).toContain('wget -qO "/tmp/${asset_name}"');
    expect(dockerfile).toContain('sha256sum -c -');
    expect(dockerfile).toContain('tar -xOzf "/tmp/${asset_name}"');
    expect(dockerfile).not.toContain('/tmp/cpa.tar.gz');
  });

  it('rejects incompatible CPA releases before downloading release assets', () => {
    const releaseMetadataCheck = releaseWorkflow.indexOf('--json isDraft,isPrerelease');
    const releaseAssetDownload = releaseWorkflow.indexOf('mkdir -p dist/cpa');

    expect(releaseMetadataCheck).toBeGreaterThan(-1);
    expect(releaseAssetDownload).toBeGreaterThan(releaseMetadataCheck);
    expect(releaseWorkflow).toContain('Draft CPA release cannot be bundled');
    expect(releaseWorkflow).toContain('Stable CPAMP release cannot bundle prerelease CPA');
    expect(releaseWorkflow).toContain('[[ "${GITHUB_REF_NAME}" != *-* ]]');
  });

  it('refreshes the signed manifest after independent CPA releases', () => {
    expect(manifestWorkflow).toContain("cron: '23 */6 * * *'");
    expect(manifestWorkflow).toContain('ref: ${{ steps.versions.outputs.cpamp_version }}');
    expect(manifestWorkflow).toContain('Expected 6 CPAMP Slim assets');
    expect(manifestWorkflow).toContain('Missing required CPA asset');
    expect(manifestWorkflow).toContain('--json isPrerelease');
    expect(manifestWorkflow).toContain('Stable runtime manifest cannot use prerelease CPA');
    expect(manifestWorkflow).toContain(
      'gh release upload "${CPAMP_VERSION}" runtime-manifest.json'
    );
    expect(manifestWorkflow).toContain('--clobber');
  });

  it('keeps the signing key away from code checked out from a release tag', () => {
    expect(manifestWorkflow).toContain('path: released');
    expect(manifestWorkflow).toContain(
      'if [ -f released/apps/manager-server/cmd/runtime-manifest/main.go ]'
    );
    expect(manifestWorkflow).toContain('path: signer');
    expect(manifestWorkflow).toContain('ref: ${{ github.sha }}');
    expect(manifestWorkflow).toContain('working-directory: signer/apps/manager-server');
    expect(manifestWorkflow).not.toContain('working-directory: released/apps/manager-server');
  });

  it('publishes the GitHub Release only after Docker images succeed', () => {
    expect(releaseWorkflow).toContain('- name: Create draft Release');
    expect(releaseWorkflow).toContain('draft: true');
    expect(releaseWorkflow).toContain('publish-release:');
    expect(releaseWorkflow).toContain('- build-and-push-docker');
    expect(releaseWorkflow).toContain('gh release edit "${GITHUB_REF_NAME}"');
    expect(releaseWorkflow).toContain('--draft=false');
    expect(releaseWorkflow).toContain('needs: publish-release');
  });

  it('refuses to clear a populated custom output directory it does not own', () => {
    const tempDir = mkdtempSync(path.join(os.tmpdir(), 'cpamp-native-package-'));
    const outputDir = path.join(tempDir, 'output');
    const webHtml = path.join(tempDir, 'management.html');
    const sentinel = path.join(outputDir, 'keep.txt');

    try {
      mkdirSync(outputDir);
      writeFileSync(webHtml, '<!doctype html>');
      writeFileSync(sentinel, 'keep');
      const result = spawnSync('bash', [path.join(repoRoot, 'bin/release/package-native.sh')], {
        cwd: repoRoot,
        env: {
          ...process.env,
          OUT_DIR: outputDir,
          WEB_HTML: webHtml,
          VERSION: 'dev',
        },
        encoding: 'utf8',
      });

      expect(result.status).toBe(1);
      expect(result.stderr).toContain(
        'refusing to clear non-empty OUT_DIR without .cpamp-native-packaging-output'
      );
      expect(readFileSync(sentinel, 'utf8')).toBe('keep');
    } finally {
      rmSync(tempDir, { recursive: true, force: true });
    }
  });
});
