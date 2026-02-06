#!/usr/bin/env node

const https = require('https');
const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const VERSION = require('./package.json').version;

// Map Node.js platform/arch to Go build names
const PLATFORM_MAP = {
  'darwin-x64': 'darwin-amd64',
  'darwin-arm64': 'darwin-arm64',
  'linux-x64': 'linux-amd64',
  'linux-arm64': 'linux-arm64',
  'win32-x64': 'windows-amd64'
};

function getPlatform() {
  const platform = process.platform;
  const arch = process.arch;
  const key = `${platform}-${arch}`;
  
  const goPlatform = PLATFORM_MAP[key];
  if (!goPlatform) {
    throw new Error(`Unsupported platform: ${platform}-${arch}`);
  }
  
  return goPlatform;
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    console.log(`Downloading devlog binary from ${url}...`);
    
    const file = fs.createWriteStream(dest);
    https.get(url, (response) => {
      if (response.statusCode === 302 || response.statusCode === 301) {
        // Follow redirect
        return download(response.headers.location, dest).then(resolve).catch(reject);
      }
      
      if (response.statusCode !== 200) {
        reject(new Error(`Failed to download: ${response.statusCode}`));
        return;
      }
      
      response.pipe(file);
      file.on('finish', () => {
        file.close();
        resolve();
      });
    }).on('error', (err) => {
      fs.unlink(dest, () => {});
      reject(err);
    });
  });
}

async function install() {
  try {
    const platform = getPlatform();
    const isWindows = platform.includes('windows');
    const binaryName = isWindows ? 'devlog.exe' : 'devlog';
    const binDir = path.join(__dirname, 'bin');
    const binaryPath = path.join(binDir, binaryName);
    
    // Create bin directory if it doesn't exist
    if (!fs.existsSync(binDir)) {
      fs.mkdirSync(binDir, { recursive: true });
    }
    
    // Download URL - adjust this based on where you host binaries
    const downloadUrl = `https://github.com/ishaan812/devlog/releases/download/v${VERSION}/devlog-${platform}${isWindows ? '.exe' : ''}`;
    
    await download(downloadUrl, binaryPath);
    
    // Make executable on Unix systems
    if (!isWindows) {
      fs.chmodSync(binaryPath, 0o755);
    }
    
    console.log('✓ devlog installed successfully!');
  } catch (error) {
    console.error('Failed to install devlog:', error.message);
    console.error('\nYou can install devlog using Go instead:');
    console.error('  go install github.com/ishaan812/devlog/cmd/devlog@latest');
    process.exit(1);
  }
}

install();
