'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const fs = require('fs');
const https = require('https');
const os = require('os');
const path = require('path');
const zlib = require('zlib');

const {
  downloadFile,
  extractTarMember,
  extractZipMember,
  installAsset,
  shouldRetainAuthOnRedirect,
} = require('./setup');

const REDIRECT_STATUS_CODES = [301, 302, 303, 307, 308];

function loadSetupWithEnv(env) {
  const modulePath = require.resolve('./setup');
  const previous = {};
  for (const [key, value] of Object.entries(env)) {
    previous[key] = process.env[key];
    process.env[key] = value;
  }

  delete require.cache[modulePath];

  try {
    return require('./setup');
  } finally {
    delete require.cache[modulePath];
    for (const [key, value] of Object.entries(previous)) {
      if (value === undefined) {
        delete process.env[key];
      } else {
        process.env[key] = value;
      }
    }
  }
}

function createResponse(statusCode, headers = {}, body = '') {
  const res = new EventEmitter();
  res.statusCode = statusCode;
  res.headers = headers;
  res.resume = () => {};

  process.nextTick(() => {
    if (body.length > 0) res.emit('data', Buffer.from(body));
    res.emit('end');
  });

  return res;
}

function createDownloadResponse(statusCode, headers = {}, body = '') {
  const res = new EventEmitter();
  res.statusCode = statusCode;
  res.headers = headers;
  res.resume = () => {};
  res.pipe = (dest) => {
    process.nextTick(() => dest.end(body));
    return dest;
  };
  return res;
}

function withTempDir(fn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'setup-test-'));
  const cleanup = () => {
    if (!fs.existsSync(dir)) return;
    fs.rmSync(dir, { recursive: true, force: true });
  };

  try {
    const result = fn(dir);
    if (result && typeof result.then === 'function') {
      return result.finally(cleanup);
    }
    cleanup();
    return result;
  } catch (err) {
    cleanup();
    throw err;
  }
}

function writeOctal(buffer, offset, length, value) {
  const digits = value.toString(8);
  const field = digits.padStart(length - 1, '0');
  buffer.write(field, offset, length - 1, 'ascii');
  buffer[offset + length - 1] = 0;
}

function createTarHeader(entry) {
  const header = Buffer.alloc(512, 0);
  const nameBuffer = Buffer.from(entry.name, 'utf8');
  assert.ok(nameBuffer.length <= 100, 'test tar entry names must fit in header');
  nameBuffer.copy(header, 0);
  writeOctal(header, 100, 8, entry.mode ?? 0o755);
  writeOctal(header, 108, 8, 0);
  writeOctal(header, 116, 8, 0);
  writeOctal(header, 124, 12, entry.data ? entry.data.length : 0);
  writeOctal(header, 136, 12, 0);
  header.fill(0x20, 148, 156);
  header[156] = (entry.type ?? '0').charCodeAt(0);
  if (entry.linkname) Buffer.from(entry.linkname, 'utf8').copy(header, 157);
  Buffer.from('ustar\0', 'ascii').copy(header, 257);
  Buffer.from('00', 'ascii').copy(header, 263);
  let sum = 0;
  for (const byte of header) sum += byte;
  const checksum = sum.toString(8).padStart(6, '0');
  header.write(checksum, 148, 6, 'ascii');
  header[154] = 0;
  header[155] = 0x20;
  return header;
}

function createTarGz(entries) {
  const parts = [];
  for (const entry of entries) {
    const data = entry.data ?? Buffer.alloc(0);
    parts.push(createTarHeader({ ...entry, data }));
    parts.push(data);
    const remainder = data.length % 512;
    if (remainder !== 0) parts.push(Buffer.alloc(512 - remainder, 0));
  }
  parts.push(Buffer.alloc(1024, 0));
  return zlib.gzipSync(Buffer.concat(parts));
}

function createZip(entries) {
  const localParts = [];
  const centralParts = [];
  let offset = 0;

  for (const entry of entries) {
    const name = Buffer.from(entry.name, 'utf8');
    const uncompressed = entry.data ?? Buffer.alloc(0);
    const method = entry.method ?? 0;
    const compressed = method === 8 ? zlib.deflateRawSync(uncompressed) : uncompressed;
    const externalAttributes = entry.externalAttributes ?? (0o100755 << 16);

    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(0, 6);
    local.writeUInt16LE(method, 8);
    local.writeUInt16LE(0, 10);
    local.writeUInt16LE(0, 12);
    local.writeUInt32LE(0, 14);
    local.writeUInt32LE(compressed.length, 18);
    local.writeUInt32LE(uncompressed.length, 22);
    local.writeUInt16LE(name.length, 26);
    local.writeUInt16LE(0, 28);
    localParts.push(local, name, compressed);

    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt16LE(0x0314, 4);
    central.writeUInt16LE(20, 6);
    central.writeUInt16LE(0, 8);
    central.writeUInt16LE(method, 10);
    central.writeUInt16LE(0, 12);
    central.writeUInt16LE(0, 14);
    central.writeUInt32LE(0, 16);
    central.writeUInt32LE(compressed.length, 20);
    central.writeUInt32LE(uncompressed.length, 24);
    central.writeUInt16LE(name.length, 28);
    central.writeUInt16LE(0, 30);
    central.writeUInt16LE(0, 32);
    central.writeUInt16LE(0, 34);
    central.writeUInt16LE(0, 36);
    central.writeUInt32LE(externalAttributes >>> 0, 38);
    central.writeUInt32LE(offset, 42);
    centralParts.push(central, name);

    offset += local.length + name.length + compressed.length;
  }

  const centralDirectory = Buffer.concat(centralParts);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(0, 4);
  end.writeUInt16LE(0, 6);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(centralDirectory.length, 12);
  end.writeUInt32LE(offset, 16);
  end.writeUInt16LE(0, 20);

  return Buffer.concat([...localParts, centralDirectory, end]);
}

function writeArchive(dir, name, contents) {
  const archivePath = path.join(dir, name);
  fs.writeFileSync(archivePath, contents);
  return archivePath;
}

test('installAsset extracts a root-level regular tar.gz binary and chmods it', () => {
  withTempDir((dir) => {
    const installDir = path.join(dir, 'install');
    fs.mkdirSync(installDir);
    const archivePath = writeArchive(dir, 'bin.tar.gz', createTarGz([
      { name: 'bin', data: Buffer.from('tar-binary') },
    ]));

    installAsset(archivePath, 'bin.tar.gz', 'linux', installDir);

    const destPath = path.join(installDir, 'bin');
    assert.equal(fs.readFileSync(destPath, 'utf8'), 'tar-binary');
    assert.equal(fs.statSync(destPath).mode & 0o777, 0o755);
  });
});

test('installAsset extracts a root-level regular zip binary and chmods it', () => {
  withTempDir((dir) => {
    const installDir = path.join(dir, 'install');
    fs.mkdirSync(installDir);
    const archivePath = writeArchive(dir, 'bin.zip', createZip([
      { name: 'bin', data: Buffer.from('zip-binary') },
    ]));

    installAsset(archivePath, 'bin.zip', 'linux', installDir);

    const destPath = path.join(installDir, 'bin');
    assert.equal(fs.readFileSync(destPath, 'utf8'), 'zip-binary');
    assert.equal(fs.statSync(destPath).mode & 0o777, 0o755);
  });
});

test('tar extraction rejects nested bin members', () => {
  withTempDir((dir) => {
    const archivePath = writeArchive(dir, 'nested.tar.gz', createTarGz([
      { name: 'nested/bin', data: Buffer.from('bad') },
    ]));

    assert.throws(() => extractTarMember(archivePath, 'bin'), /nested or non-canonical path/);
  });
});

test('zip extraction rejects nested bin members', () => {
  withTempDir((dir) => {
    const archivePath = writeArchive(dir, 'nested.zip', createZip([
      { name: 'nested/bin', data: Buffer.from('bad') },
    ]));

    assert.throws(() => extractZipMember(archivePath, 'bin'), /nested or non-canonical path/);
  });
});

test('archive extraction rejects path traversal and absolute paths', () => {
  withTempDir((dir) => {
    const tarPath = writeArchive(dir, 'traversal.tar.gz', createTarGz([
      { name: '../bin', data: Buffer.from('bad') },
    ]));
    const zipPath = writeArchive(dir, 'absolute.zip', createZip([
      { name: '/bin', data: Buffer.from('bad') },
    ]));

    assert.throws(() => extractTarMember(tarPath, 'bin'), /parent traversal/);
    assert.throws(() => extractZipMember(zipPath, 'bin'), /absolute path/);
  });
});

test('archive extraction rejects duplicate selected members', () => {
  withTempDir((dir) => {
    const tarPath = writeArchive(dir, 'duplicate.tar.gz', createTarGz([
      { name: 'bin', data: Buffer.from('one') },
      { name: 'bin', data: Buffer.from('two') },
    ]));
    const zipPath = writeArchive(dir, 'duplicate.zip', createZip([
      { name: 'bin', data: Buffer.from('one') },
      { name: 'bin', data: Buffer.from('two') },
    ]));

    assert.throws(() => extractTarMember(tarPath, 'bin'), /duplicated/);
    assert.throws(() => extractZipMember(zipPath, 'bin'), /duplicated/);
  });
});

for (const { label, type, linkname } of [
  { label: 'directory', type: '5' },
  { label: 'symlink', type: '2', linkname: 'elsewhere' },
  { label: 'hardlink', type: '1', linkname: 'elsewhere' },
  { label: 'fifo', type: '6' },
]) {
  test(`tar extraction rejects ${label} selected members`, () => {
    withTempDir((dir) => {
      const archivePath = writeArchive(dir, `${label}.tar.gz`, createTarGz([
        { name: 'bin', type, linkname },
      ]));

      assert.throws(() => extractTarMember(archivePath, 'bin'), /not a regular file/);
    });
  });
}

test('zip extraction rejects symlink selected members', () => {
  withTempDir((dir) => {
    const archivePath = writeArchive(dir, 'symlink.zip', createZip([
      { name: 'bin', data: Buffer.from('bad'), externalAttributes: 0o120777 << 16 },
    ]));

    assert.throws(() => extractZipMember(archivePath, 'bin'), /not a regular file/);
  });
});

test('shouldRetainAuthOnRedirect uses exact URL.origin matching', () => {
  assert.equal(
    shouldRetainAuthOnRedirect('https://example.com/releases/1', 'https://example.com/assets/bin'),
    true,
  );
  assert.equal(
    shouldRetainAuthOnRedirect('https://example.com/releases/1', 'https://example.com:8443/assets/bin'),
    false,
  );
  assert.equal(
    shouldRetainAuthOnRedirect('https://example.com/releases/1', 'http://example.com/assets/bin'),
    false,
  );
  assert.equal(
    shouldRetainAuthOnRedirect('https://example.com/releases/1', 'https://downloads.example.com/assets/bin'),
    false,
  );
  assert.equal(
    shouldRetainAuthOnRedirect('https://example.com/releases/1', '/assets/bin'),
    true,
  );
});

for (const statusCode of REDIRECT_STATUS_CODES) {
  test(`fetchRelease fails closed on cross-origin redirect ${statusCode}`, async (t) => {
    const calls = [];
    const originalGet = https.get;
    const { fetchRelease: fetchReleaseWithToken } = loadSetupWithEnv({
      INPUT_GITHUB_TOKEN: 'test-token',
    });

    https.get = (url, options, callback) => {
      calls.push({ url, headers: options.headers });

      if (calls.length === 1) {
        callback(createResponse(statusCode, { location: 'https://redirect.example/releases/latest' }));
      } else {
        callback(createResponse(200, {}, JSON.stringify({ tag_name: 'v1.2.3', assets: [] })));
      }

      return { on() { return this; } };
    };

    t.after(() => {
      https.get = originalGet;
    });

    await assert.rejects(fetchReleaseWithToken('latest'), /Refusing cross-origin redirect/);

    assert.equal(calls.length, 1);
    assert.equal(calls[0].headers.Authorization, 'Bearer test-token');
  });

  test(`fetchRelease retains auth headers on same-origin redirect ${statusCode}`, async (t) => {
    const calls = [];
    const originalGet = https.get;
    const { fetchRelease: fetchReleaseWithToken } = loadSetupWithEnv({
      INPUT_GITHUB_TOKEN: 'test-token',
    });

    https.get = (url, options, callback) => {
      calls.push({ url, headers: options.headers });

      if (calls.length === 1) {
        callback(createResponse(statusCode, { location: '/repos/aaronflorey/bin/releases/latest?page=2' }));
      } else {
        callback(createResponse(200, {}, JSON.stringify({ tag_name: 'v1.2.3', assets: [] })));
      }

      return { on() { return this; } };
    };

    t.after(() => {
      https.get = originalGet;
    });

    const release = await fetchReleaseWithToken('latest');

    assert.equal(release.tag_name, 'v1.2.3');
    assert.equal(calls.length, 2);
    assert.equal(calls[0].headers.Authorization, 'Bearer test-token');
    assert.equal(calls[1].url, 'https://api.github.com/repos/aaronflorey/bin/releases/latest?page=2');
    assert.equal(calls[1].headers.Authorization, 'Bearer test-token');
  });

  test(`downloadFile retains auth headers on same-origin redirect ${statusCode}`, async (t) => {
    await withTempDir(async (dir) => {
      const calls = [];
      const originalGet = https.get;
      const destPath = path.join(dir, `same-origin-${statusCode}.bin`);

      https.get = (url, options, callback) => {
        calls.push({ url, headers: options.headers });

        if (calls.length === 1) {
          callback(createDownloadResponse(statusCode, { location: '/assets/bin' }));
        } else {
          callback(createDownloadResponse(200, {}, 'same-origin-body'));
        }

        return { on() { return this; } };
      };

      t.after(() => {
        https.get = originalGet;
      });

      await downloadFile(
        'https://api.github.com/repos/aaronflorey/bin/releases/assets/1',
        destPath,
        { Authorization: 'Bearer test-token' },
      );

      assert.equal(fs.readFileSync(destPath, 'utf8'), 'same-origin-body');
      assert.equal(calls.length, 2);
      assert.equal(calls[0].headers.Authorization, 'Bearer test-token');
      assert.equal(calls[1].url, 'https://api.github.com/assets/bin');
      assert.equal(calls[1].headers.Authorization, 'Bearer test-token');
    });
  });

  test(`downloadFile strips auth headers on cross-origin redirect ${statusCode}`, async (t) => {
    await withTempDir(async (dir) => {
      const calls = [];
      const originalGet = https.get;
      const destPath = path.join(dir, `cross-origin-${statusCode}.bin`);

      https.get = (url, options, callback) => {
        calls.push({ url, headers: options.headers });

        if (calls.length === 1) {
          callback(createDownloadResponse(statusCode, { location: 'https://objects.example.com/assets/bin' }));
        } else {
          callback(createDownloadResponse(200, {}, 'cross-origin-body'));
        }

        return { on() { return this; } };
      };

      t.after(() => {
        https.get = originalGet;
      });

      await downloadFile(
        'https://api.github.com/repos/aaronflorey/bin/releases/assets/1',
        destPath,
        { Authorization: 'Bearer test-token' },
      );

      assert.equal(fs.readFileSync(destPath, 'utf8'), 'cross-origin-body');
      assert.equal(calls.length, 2);
      assert.equal(calls[0].headers.Authorization, 'Bearer test-token');
      assert.equal(calls[1].url, 'https://objects.example.com/assets/bin');
      assert.equal(calls[1].headers.Authorization, undefined);
    });
  });

  test(`downloadFile rejects insecure redirect target ${statusCode}`, async (t) => {
    await withTempDir(async (dir) => {
      const calls = [];
      const originalGet = https.get;
      const destPath = path.join(dir, `insecure-${statusCode}.bin`);

      https.get = (url, options, callback) => {
        calls.push({ url, headers: options.headers });
        callback(createDownloadResponse(statusCode, { location: 'http://objects.example.com/assets/bin' }));
        return { on() { return this; } };
      };

      t.after(() => {
        https.get = originalGet;
      });

      await assert.rejects(
        downloadFile(
          'https://api.github.com/repos/aaronflorey/bin/releases/assets/1',
          destPath,
          { Authorization: 'Bearer test-token' },
        ),
        /Refusing insecure redirect/,
      );

      assert.equal(calls.length, 1);
      assert.equal(calls[0].headers.Authorization, 'Bearer test-token');
      assert.equal(fs.existsSync(destPath), false);
    });
  });
}
