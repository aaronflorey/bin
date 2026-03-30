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
 * Returns a Promise<{ statusCode, body: Buffer }>.
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
      res.on('end', () => resolve({ statusCode: res.statusCode, body: Buffer.concat(chunks) }));
      res.on('error', reject);
    }).on('error', reject);
  });
}

/**
 * Streams `url` to `destPath`, following redirects.
 * Asset downloads don't include auth headers after a redirect to S3/CDN.
 */
function downloadFile(url, destPath, headers = {}, maxRedirects = 5) {
  return new Promise((resolve, reject) => {
    function get(u, hdrs, redirectsLeft) {
      https.get(u, { headers: hdrs }, (res) => {
        if ((res.statusCode === 301 || res.statusCode === 302) && res.headers.location) {
          if (redirectsLeft === 0) return reject(new Error(`Too many redirects for ${u}`));
          // Drop auth header on cross-origin redirects (e.g. GitHub → S3)
          const next = res.headers.location;
          const sameOrigin = new URL(next).hostname === new URL(u).hostname;
          return get(next, sameOrigin ? hdrs : {}, redirectsLeft - 1);
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
    get(url, headers, maxRedirects);
  });
}

// ── Platform detection ─────────────────────────────────────────────────────────

function detectPlatform() {
  const platform = os.platform();
  const arch = os.arch();

  const osMap = { linux: 'linux', darwin: 'darwin', win32: 'windows' };
  const archMap = { x64: 'amd64', arm64: 'arm64' };

  if (!osMap[platform]) fail(`Unsupported OS: ${platform}`);
  if (!archMap[arch]) fail(`Unsupported architecture: ${arch}`);

  return { osName: osMap[platform], arch: archMap[arch] };
}

// ── Release fetching ───────────────────────────────────────────────────────────

async function fetchRelease(requestedVersion) {
  let apiUrl;

  if (requestedVersion === 'latest') {
    console.log('Fetching latest bin release...');
    apiUrl = 'https://api.github.com/repos/aaronflorey/bin/releases/latest';
  } else {
    const tag = requestedVersion.startsWith('v') ? requestedVersion : `v${requestedVersion}`;
    console.log(`Fetching bin release ${tag}...`);
    apiUrl = `https://api.github.com/repos/aaronflorey/bin/releases/tags/${tag}`;
  }

  const { statusCode, body } = await httpsGet(apiUrl, authHeaders());

  if (statusCode !== 200) {
    fail(`Failed to fetch release from GitHub API (HTTP ${statusCode}). Verify the version exists and the token has sufficient permissions.`);
  }

  let release;
  try {
    release = JSON.parse(body.toString());
  } catch {
    fail('Failed to parse the GitHub API response.');
  }

  if (!release.tag_name) fail('GitHub API response did not include a tag_name field.');
  return release;
}

// ── Asset selection ────────────────────────────────────────────────────────────

/**
 * Picks the best matching asset for the current platform.
 *
 * Strategy: find assets whose name contains both the OS and arch strings.
 * Excludes checksums and source archives. Prefers shorter names (fewer
 * extra qualifiers) when multiple candidates match.
 */
function selectAsset(assets, osName, arch) {
  const candidates = assets.filter((a) => {
    const n = a.name.toLowerCase();
    return (
      n.includes(osName) &&
      n.includes(arch) &&
      !n.endsWith('.txt') &&
      !n.endsWith('.json') &&
      !n.endsWith('.sbom')
    );
  });

  if (candidates.length === 0) {
    const names = assets.map((a) => a.name).join(', ');
    fail(`No release asset found for ${osName}/${arch}. Available assets: ${names}`);
  }

  // Prefer the shortest name — fewest extra qualifiers (e.g. musl, gnu).
  candidates.sort((a, b) => a.name.length - b.name.length);
  return candidates[0];
}

// ── Installation ───────────────────────────────────────────────────────────────

/**
 * Installs the asset at `assetPath` to `installDir`.
 * Handles plain binaries, .exe, .tar.gz, and .zip archives.
 */
function installAsset(assetPath, assetName, osName, installDir) {
  const binName = osName === 'windows' ? 'bin.exe' : 'bin';
  const destPath = path.join(installDir, binName);

  if (assetName.endsWith('.tar.gz') || assetName.endsWith('.tgz')) {
    const tmpDir = path.dirname(assetPath);
    execFileSync('tar', ['-xzf', assetPath, '-C', tmpDir], { stdio: 'inherit' });
    const extracted = path.join(tmpDir, binName);
    if (!fs.existsSync(extracted)) fail(`'${binName}' not found in archive.`);
    fs.copyFileSync(extracted, destPath);
  } else if (assetName.endsWith('.zip')) {
    const tmpDir = path.dirname(assetPath);
    if (osName === 'windows') {
      execFileSync('tar', ['-xf', assetPath, '-C', tmpDir], { stdio: 'inherit' });
    } else {
      execFileSync('unzip', ['-q', assetPath, '-d', tmpDir], { stdio: 'inherit' });
    }
    const extracted = path.join(tmpDir, binName);
    if (!fs.existsSync(extracted)) fail(`'${binName}' not found in archive.`);
    fs.copyFileSync(extracted, destPath);
  } else {
    // Plain binary or .exe — use directly.
    fs.copyFileSync(assetPath, destPath);
  }

  fs.chmodSync(destPath, 0o755);
}

// ── Main ───────────────────────────────────────────────────────────────────────

async function main() {
  const { osName, arch } = detectPlatform();
  const release = await fetchRelease(version);
  const resolvedVersion = release.tag_name;

  const asset = selectAsset(release.assets, osName, arch);
  console.log(`Installing bin ${resolvedVersion} (${osName}/${arch}) via ${asset.name}...`);

  const installDir = path.join(os.homedir(), '.local', 'bin');
  fs.mkdirSync(installDir, { recursive: true });

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'bin-setup-'));
  try {
    const assetPath = path.join(tmpDir, asset.name);

    try {
      await downloadFile(asset.browser_download_url, assetPath, authHeaders());
    } catch (err) {
      fail(`Failed to download ${asset.browser_download_url}: ${err.message}`);
    }

    installAsset(assetPath, asset.name, osName, installDir);

    addToPath(installDir);
    setOutput('version', resolvedVersion);

    console.log(`bin ${resolvedVersion} installed to ${installDir}`);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

main().catch((err) => fail(err.message ?? String(err)));
