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
const zlib = require('zlib');

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

function shouldRetainAuthOnRedirect(fromUrl, toUrl) {
  return new URL(toUrl, fromUrl).origin === new URL(fromUrl).origin;
}

function isHttpsUrl(url, base) {
  return new URL(url, base).protocol === 'https:';
}

function isRedirectResponse(statusCode, location) {
  return [301, 302, 303, 307, 308].includes(statusCode) && Boolean(location);
}

/**
 * Makes an HTTPS GET request, following up to `maxRedirects` redirects.
 * Returns a Promise<{ statusCode, body: Buffer }>.
 */
function httpsGet(url, headers = {}, maxRedirects = 5) {
  return new Promise((resolve, reject) => {
    https.get(url, { headers }, (res) => {
      if (isRedirectResponse(res.statusCode, res.headers.location)) {
        if (maxRedirects === 0) return reject(new Error(`Too many redirects for ${url}`));
        const next = new URL(res.headers.location, url).toString();
        if (!shouldRetainAuthOnRedirect(url, next)) {
          return reject(new Error(`Refusing cross-origin redirect for ${url}`));
        }
        return resolve(httpsGet(next, headers, maxRedirects - 1));
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
        if (isRedirectResponse(res.statusCode, res.headers.location)) {
          if (redirectsLeft === 0) return reject(new Error(`Too many redirects for ${u}`));
          // Drop auth header on cross-origin redirects (e.g. GitHub → S3)
          const next = new URL(res.headers.location, u).toString();
          if (!isHttpsUrl(next)) {
            res.resume();
            return reject(new Error(`Refusing insecure redirect for ${u}`));
          }
          const sameOrigin = shouldRetainAuthOnRedirect(u, next);
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

function classifyArchivePath(name, targetName) {
  if (typeof name !== 'string' || name.length === 0 || name.includes('\0')) {
    return { normalized: null, targetsSelectedName: false, reason: 'invalid path' };
  }

  const unified = name.replace(/\\/g, '/');
  const rawParts = unified.split('/');
  const rawLeafName = rawParts.findLast((part) => part !== '') || '';
  const rawTargetsSelectedName =
    rawLeafName === targetName ||
    unified === targetName ||
    unified === `${targetName}/` ||
    unified.startsWith(`${targetName}/`) ||
    unified.endsWith(`/${targetName}`);

  if (unified.startsWith('/') || /^[A-Za-z]:\//.test(unified)) {
    return { normalized: null, targetsSelectedName: rawTargetsSelectedName, reason: 'absolute path' };
  }

  const normalizedParts = [];
  let nonCanonical = unified !== name;

  for (const part of rawParts) {
    if (part === '' || part === '.') {
      nonCanonical = true;
      continue;
    }
    if (part === '..') {
      return { normalized: null, targetsSelectedName: rawTargetsSelectedName, reason: 'parent traversal' };
    }
    normalizedParts.push(part);
  }

  const normalized = normalizedParts.join('/');
  const leafName = normalizedParts[normalizedParts.length - 1] || '';
  const targetsSelectedName =
    leafName === targetName ||
    normalized === targetName ||
    normalized.startsWith(`${targetName}/`) ||
    unified === `${targetName}/` ||
    unified.startsWith(`${targetName}/`);

  if (!normalized) {
    return { normalized, targetsSelectedName, reason: 'empty path' };
  }

  if (targetsSelectedName && (nonCanonical || normalized !== targetName || normalizedParts.length !== 1)) {
    return { normalized, targetsSelectedName, reason: 'nested or non-canonical path' };
  }

  return { normalized, targetsSelectedName, reason: null };
}

function readTarString(buffer, start, length) {
  return buffer.slice(start, start + length).toString('utf8').replace(/\0.*$/, '');
}

function readTarOctal(buffer, start, length) {
  const text = readTarString(buffer, start, length).trim();
  if (!text) return 0;
  if (!/^[0-7]+$/.test(text)) throw new Error('Unsupported tar numeric field encoding.');
  return parseInt(text, 8);
}

function extractTarMember(assetPath, binName) {
  const archive = zlib.gunzipSync(fs.readFileSync(assetPath));
  let offset = 0;
  let selected = null;

  while (offset + 512 <= archive.length) {
    const header = archive.slice(offset, offset + 512);
    if (header.every((byte) => byte === 0)) break;

    const name = readTarString(header, 0, 100);
    const prefix = readTarString(header, 345, 155);
    const fullName = prefix ? `${prefix}/${name}` : name;
    const size = readTarOctal(header, 124, 12);
    const typeflag = String.fromCharCode(header[156] || 0);
    const dataStart = offset + 512;
    const dataEnd = dataStart + size;

    if (dataEnd > archive.length) throw new Error('Tar archive is truncated.');

    const pathInfo = classifyArchivePath(fullName, binName);
    if (pathInfo.targetsSelectedName) {
      if (pathInfo.reason) throw new Error(`Archive member '${fullName}' rejected: ${pathInfo.reason}.`);
      if (typeflag !== '0' && typeflag !== '\0') {
        throw new Error(`Archive member '${fullName}' rejected: not a regular file.`);
      }
      if (selected) throw new Error(`Archive member '${binName}' is duplicated.`);
      selected = Buffer.from(archive.slice(dataStart, dataEnd));
    }

    offset = dataStart + Math.ceil(size / 512) * 512;
  }

  if (!selected) throw new Error(`'${binName}' not found in archive.`);
  return selected;
}

function findZipEndOfCentralDirectory(buffer) {
  const minOffset = Math.max(0, buffer.length - 0xFFFF - 22);
  for (let offset = buffer.length - 22; offset >= minOffset; offset -= 1) {
    if (buffer.readUInt32LE(offset) === 0x06054b50) return offset;
  }
  throw new Error('Zip end of central directory not found.');
}

function isZipRegularFile(entry) {
  if (entry.name.endsWith('/')) return false;

  const unixMode = (entry.externalAttributes >>> 16) & 0xFFFF;
  if (unixMode !== 0) {
    const fileType = unixMode & 0o170000;
    if (fileType === 0o100000) return true;
    if (fileType !== 0) return false;
  }

  if ((entry.externalAttributes & 0x10) !== 0) return false;
  return true;
}

function parseZipEntries(buffer) {
  const eocdOffset = findZipEndOfCentralDirectory(buffer);
  const entryCount = buffer.readUInt16LE(eocdOffset + 10);
  const centralDirectorySize = buffer.readUInt32LE(eocdOffset + 12);
  const centralDirectoryOffset = buffer.readUInt32LE(eocdOffset + 16);

  if (entryCount === 0xFFFF || centralDirectorySize === 0xFFFFFFFF || centralDirectoryOffset === 0xFFFFFFFF) {
    throw new Error('Zip64 archives are not supported.');
  }
  if (centralDirectoryOffset + centralDirectorySize > buffer.length) {
    throw new Error('Zip central directory exceeds archive bounds.');
  }

  let offset = centralDirectoryOffset;
  const entries = [];

  for (let i = 0; i < entryCount; i += 1) {
    if (offset + 46 > buffer.length || buffer.readUInt32LE(offset) !== 0x02014b50) {
      throw new Error('Invalid zip central directory entry.');
    }

    const flags = buffer.readUInt16LE(offset + 8);
    const compressionMethod = buffer.readUInt16LE(offset + 10);
    const compressedSize = buffer.readUInt32LE(offset + 20);
    const uncompressedSize = buffer.readUInt32LE(offset + 24);
    const fileNameLength = buffer.readUInt16LE(offset + 28);
    const extraFieldLength = buffer.readUInt16LE(offset + 30);
    const fileCommentLength = buffer.readUInt16LE(offset + 32);
    const externalAttributes = buffer.readUInt32LE(offset + 38);
    const localHeaderOffset = buffer.readUInt32LE(offset + 42);
    const nameStart = offset + 46;
    const nameEnd = nameStart + fileNameLength;

    if (nameEnd > buffer.length) throw new Error('Zip entry name exceeds archive bounds.');
    if (compressedSize === 0xFFFFFFFF || uncompressedSize === 0xFFFFFFFF || localHeaderOffset === 0xFFFFFFFF) {
      throw new Error('Zip64 entries are not supported.');
    }

    entries.push({
      name: buffer.slice(nameStart, nameEnd).toString('utf8'),
      flags,
      compressionMethod,
      compressedSize,
      uncompressedSize,
      externalAttributes,
      localHeaderOffset,
    });

    offset = nameEnd + extraFieldLength + fileCommentLength;
  }

  return entries;
}

function extractZipMember(assetPath, binName) {
  const archive = fs.readFileSync(assetPath);
  const entries = parseZipEntries(archive);
  let selected = null;

  for (const entry of entries) {
    const pathInfo = classifyArchivePath(entry.name, binName);
    if (!pathInfo.targetsSelectedName) continue;
    if (pathInfo.reason) throw new Error(`Archive member '${entry.name}' rejected: ${pathInfo.reason}.`);
    if (!isZipRegularFile(entry)) {
      throw new Error(`Archive member '${entry.name}' rejected: not a regular file.`);
    }
    if ((entry.flags & 0x1) !== 0) throw new Error(`Archive member '${entry.name}' rejected: encrypted entries are unsupported.`);
    if (selected) throw new Error(`Archive member '${binName}' is duplicated.`);
    selected = entry;
  }

  if (!selected) throw new Error(`'${binName}' not found in archive.`);

  const localOffset = selected.localHeaderOffset;
  if (localOffset + 30 > archive.length || archive.readUInt32LE(localOffset) !== 0x04034b50) {
    throw new Error('Invalid zip local file header.');
  }

  const fileNameLength = archive.readUInt16LE(localOffset + 26);
  const extraFieldLength = archive.readUInt16LE(localOffset + 28);
  const fileNameStart = localOffset + 30;
  const fileNameEnd = fileNameStart + fileNameLength;
  const dataStart = fileNameEnd + extraFieldLength;
  const dataEnd = dataStart + selected.compressedSize;

  if (dataEnd > archive.length) throw new Error('Zip entry exceeds archive bounds.');

  const localName = archive.slice(fileNameStart, fileNameEnd).toString('utf8');
  if (localName !== selected.name) throw new Error('Zip central directory/local header name mismatch.');

  const compressed = archive.slice(dataStart, dataEnd);
  if (selected.compressionMethod === 0) {
    return Buffer.from(compressed);
  }
  if (selected.compressionMethod === 8) {
    const uncompressed = zlib.inflateRawSync(compressed);
    if (uncompressed.length !== selected.uncompressedSize) {
      throw new Error('Zip entry size mismatch after decompression.');
    }
    return uncompressed;
  }

  throw new Error(`Unsupported zip compression method: ${selected.compressionMethod}.`);
}

function copyRegularFile(sourcePath, destPath) {
  const stat = fs.lstatSync(sourcePath);
  if (!stat.isFile()) throw new Error(`'${sourcePath}' is not a regular file.`);
  fs.copyFileSync(sourcePath, destPath);
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
    const extracted = extractTarMember(assetPath, binName);
    fs.writeFileSync(destPath, extracted);
  } else if (assetName.endsWith('.zip')) {
    const extracted = extractZipMember(assetPath, binName);
    fs.writeFileSync(destPath, extracted);
  } else {
    // Plain binary or .exe — use directly.
    copyRegularFile(assetPath, destPath);
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

module.exports = {
  classifyArchivePath,
  fetchRelease,
  extractTarMember,
  extractZipMember,
  downloadFile,
  httpsGet,
  installAsset,
  isZipRegularFile,
  parseZipEntries,
  shouldRetainAuthOnRedirect,
};

if (require.main === module) {
  main().catch((err) => fail(err.message ?? String(err)));
}
