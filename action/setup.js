'use strict';

/**
 * Downloads and installs the bin binary manager.
 *
 * Inputs (via environment variables):
 *   INPUT_VERSION       - version to install, e.g. "v1.1.0" or "latest"
 *   INPUT_GITHUB_TOKEN  - GitHub token for API / asset download requests
 *
 * GitHub Actions context:
 *   GITHUB_PATH   - append install directory to make bin available on PATH
 *   GITHUB_OUTPUT - write "version=<v>" output
 */

const https = require('https');
const fs = require('fs');
const path = require('path');
const os = require('os');
const { execFileSync } = require('child_process');

const version = (process.env.INPUT_VERSION || 'latest').trim();
const token = (process.env.INPUT_GITHUB_TOKEN || '').trim();

// ── Helpers ────────────────────────────────────────────────────────────────────

function fail(msg) {
  console.error(`::error::${msg}`);
  process.exit(1);
}

function setOutput(name, value) {
  fs.appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}\n`);
}

function addToPath(dir) {
  fs.appendFileSync(process.env.GITHUB_PATH, `${dir}\n`);
}

function authHeaders() {
  const h = { 'User-Agent': 'aaronflorey-bin-action', 'Accept': 'application/vnd.github.v3+json' };
  if (token) h['Authorization'] = `Bearer ${token}`;
  return h;
}

/**
 * Makes an HTTPS GET request, following up to `maxRedirects` redirects.
 * Returns a Promise<{ statusCode, headers, body: Buffer }>.
 */
function httpsGet(url, headers = {}, maxRedirects = 5) {
  return new Promise((resolve, reject) => {
    https.get(url, { headers }, (res) => {
      if ((res.statusCode === 301 || res.statusCode === 302) && res.headers.location) {
        if (maxRedirects === 0) return reject(new Error(`Too many redirects for ${url}`));
        return resolve(httpsGet(res.headers.location, headers, maxRedirects - 1));
      }

      const chunks = [];
      res.on('data', (chunk) => chunks.push(chunk));
      res.on('end', () => resolve({ statusCode: res.statusCode, headers: res.headers, body: Buffer.concat(chunks) }));
      res.on('error', reject);
    }).on('error', reject);
  });
}

/**
 * Downloads `url` to `destPath`, following redirects.
 */
function downloadFile(url, destPath, headers = {}) {
  return new Promise((resolve, reject) => {
    function get(u, redirectsLeft) {
      https.get(u, { headers }, (res) => {
        if ((res.statusCode === 301 || res.statusCode === 302) && res.headers.location) {
          if (redirectsLeft === 0) return reject(new Error(`Too many redirects for ${u}`));
          return get(res.headers.location, redirectsLeft - 1);
        }
        if (res.statusCode !== 200) {
          res.resume();
          return reject(new Error(`HTTP ${res.statusCode} downloading ${u}`));
        }
        const file = fs.createWriteStream(destPath);
        res.pipe(file);
        file.on('finish', () => file.close(resolve));
        file.on('error', reject);
      }).on('error', reject);
    }
    get(url, 5);
  });
}

// ── Platform detection ─────────────────────────────────────────────────────────

function detectPlatform() {
  const platform = os.platform();
  const arch = os.arch();

  const osMap = { linux: ['linux', 'tar.gz'], darwin: ['darwin', 'tar.gz'], win32: ['windows', 'zip'] };
  const archMap = { x64: 'amd64', arm64: 'arm64' };

  if (!osMap[platform]) fail(`Unsupported OS: ${platform}`);
  if (!archMap[arch]) fail(`Unsupported architecture: ${arch}`);

  const [osName, ext] = osMap[platform];
  return { osName, arch: archMap[arch], ext };
}

// ── Version resolution ─────────────────────────────────────────────────────────

async function resolveVersion(requested) {
  if (requested !== 'latest') {
    return requested.startsWith('v') ? requested : `v${requested}`;
  }

  console.log('Fetching latest bin release...');
  const { statusCode, body } = await httpsGet(
    'https://api.github.com/repos/aaronflorey/bin/releases/latest',
    authHeaders(),
  );

  if (statusCode !== 200) {
    fail(`Failed to fetch latest version from GitHub API (HTTP ${statusCode}). Ensure the token has sufficient permissions.`);
  }

  let release;
  try {
    release = JSON.parse(body.toString());
  } catch {
    fail('Failed to parse the GitHub API response while resolving the latest version.');
  }

  if (!release.tag_name) fail('GitHub API response did not include a tag_name field.');
  return release.tag_name;
}

// ── Main ───────────────────────────────────────────────────────────────────────

async function main() {
  const { osName, arch, ext } = detectPlatform();
  const resolvedVersion = await resolveVersion(version);
  const versionNum = resolvedVersion.replace(/^v/, '');

  const asset = `bin_${versionNum}_${osName}_${arch}.${ext}`;
  const downloadUrl = `https://github.com/aaronflorey/bin/releases/download/${resolvedVersion}/${asset}`;

  console.log(`Installing bin ${resolvedVersion} (${osName}/${arch})...`);

  const installDir = path.join(os.homedir(), '.local', 'bin');
  fs.mkdirSync(installDir, { recursive: true });

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'bin-setup-'));
  try {
    const assetPath = path.join(tmpDir, asset);

    console.log(`Downloading ${downloadUrl}...`);
    try {
      await downloadFile(downloadUrl, assetPath, authHeaders());
    } catch (err) {
      fail(`Failed to download ${downloadUrl}: ${err.message}. Verify the version '${resolvedVersion}' exists and the token has sufficient permissions.`);
    }

    // Extract archive
    if (ext === 'tar.gz') {
      execFileSync('tar', ['-xzf', assetPath, '-C', tmpDir], { stdio: 'inherit' });
    } else {
      // .zip – use tar on Windows (bsdtar), unzip elsewhere
      if (osName === 'windows') {
        execFileSync('tar', ['-xf', assetPath, '-C', tmpDir], { stdio: 'inherit' });
      } else {
        execFileSync('unzip', ['-q', assetPath, '-d', tmpDir], { stdio: 'inherit' });
      }
    }

    const binName = osName === 'windows' ? 'bin.exe' : 'bin';
    const extractedBin = path.join(tmpDir, binName);
    if (!fs.existsSync(extractedBin)) {
      fail(`Binary '${binName}' was not found in the downloaded archive.`);
    }

    fs.copyFileSync(extractedBin, path.join(installDir, binName));
    fs.chmodSync(path.join(installDir, binName), 0o755);

    addToPath(installDir);
    setOutput('version', resolvedVersion);

    console.log(`bin ${resolvedVersion} installed to ${installDir}`);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

main().catch((err) => fail(err.message ?? String(err)));
