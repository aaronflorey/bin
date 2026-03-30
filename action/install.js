'use strict';

/**
 * Installs binaries listed in INPUT_INSTALL using the bin binary manager.
 *
 * Each line of INPUT_INSTALL should be one of:
 *   org/repo              - installs the latest release
 *   org/repo@version      - installs a specific version
 *
 * Lines that are blank or start with '#' are ignored.
 *
 * Inputs (via environment variables):
 *   INPUT_INSTALL        - newline-separated list of repos to install
 *   INPUT_GITHUB_TOKEN   - passed through as GITHUB_TOKEN for bin to use
 */

const { spawnSync } = require('child_process');
const os = require('os');
const path = require('path');

const installList = (process.env.INPUT_INSTALL || '').trim();
const token = (process.env.INPUT_GITHUB_TOKEN || '').trim();

// Installed binaries go to the same directory bin itself lives in.
const BIN_EXE_DIR = path.join(os.homedir(), '.local', 'bin');

// ── Helpers ────────────────────────────────────────────────────────────────────

function error(msg) {
  console.error(`::error::${msg}`);
}

/**
 * Parses a single entry from the install list.
 * Returns { url, label, versioned } or null if the entry is invalid.
 */
function parseEntry(raw) {
  const entry = raw.trim();
  if (!entry || entry.startsWith('#')) return null;

  if (!entry.includes('/')) {
    error(`Invalid entry '${entry}': expected format 'org/repo' or 'org/repo@version'.`);
    return null;
  }

  if (entry.includes('@')) {
    const atIdx = entry.lastIndexOf('@');
    const repo = entry.slice(0, atIdx);
    let ver = entry.slice(atIdx + 1);

    if (!repo.includes('/')) {
      error(`Invalid entry '${entry}': expected format 'org/repo@version'.`);
      return null;
    }

    if (!ver.startsWith('v')) ver = `v${ver}`;
    return {
      url: `github.com/${repo}/releases/tag/${ver}`,
      label: `${repo}@${ver}`,
      versioned: true,
    };
  }

  return {
    url: `github.com/${entry}`,
    label: entry,
    versioned: false,
  };
}

/**
 * Runs `bin install --force <url>` and returns true on success.
 *
 * For versioned installs, pipes "n\n" to stdin so that the interactive
 * version-pin confirmation prompt is gracefully declined in CI.
 */
function installBinary({ url, label, versioned }) {
  console.log(`Installing ${label}...`);

  const result = spawnSync('bin', ['install', '--force', url], {
    input: versioned ? 'n\n' : undefined,
    env: { ...process.env, BIN_EXE_DIR, GITHUB_TOKEN: token },
    encoding: 'utf8',
    stdio: ['pipe', 'inherit', 'inherit'],
  });

  if (result.error) {
    error(`Failed to run bin: ${result.error.message}`);
    return false;
  }

  if (result.status !== 0) {
    error(`Failed to install '${label}'.`);
    return false;
  }

  return true;
}

// ── Main ───────────────────────────────────────────────────────────────────────

function main() {
  if (!installList) return;

  const entries = installList.split('\n').map(parseEntry).filter(Boolean);

  if (entries.length === 0) return;

  let failures = 0;
  for (const entry of entries) {
    if (!installBinary(entry)) failures++;
  }

  if (failures > 0) {
    error(`${failures} binary install(s) failed. See the errors above for details.`);
    process.exit(1);
  }
}

main();
