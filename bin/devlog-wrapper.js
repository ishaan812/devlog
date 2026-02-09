#!/usr/bin/env node

const { execFileSync } = require("child_process");
const path = require("path");
const fs = require("fs");

const isWindows = process.platform === "win32";
const binaryName = isWindows ? "devlog.exe" : "devlog";
const binaryPath = path.join(__dirname, binaryName);

if (!fs.existsSync(binaryPath)) {
  console.error(
    "Error: devlog binary not found. Try reinstalling:\n" +
      "  npm install -g @ishaan812/devlog\n" +
      "\nOr install via Go:\n" +
      "  go install github.com/ishaan812/devlog/cmd/devlog@latest"
  );
  process.exit(1);
}

try {
  const result = execFileSync(binaryPath, process.argv.slice(2), {
    stdio: "inherit",
    env: process.env,
  });
} catch (error) {
  if (error.status !== null) {
    process.exit(error.status);
  }
  process.exit(1);
}
