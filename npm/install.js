"use strict";

const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const https = require("https");

const VERSION = require("./package.json").version;
const REPO = "miyamiyaz/mcp-supervisor";

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

function getArchiveName() {
  const os = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];
  if (!os || !arch) {
    throw new Error(
      `Unsupported platform: ${process.platform}-${process.arch}`
    );
  }
  const ext = os === "windows" ? "zip" : "tar.gz";
  return `mcp-supervisor_${VERSION}_${os}_${arch}.${ext}`;
}

function fetch(url) {
  return new Promise((resolve, reject) => {
    https
      .get(url, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return fetch(res.headers.location).then(resolve, reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        }
        const chunks = [];
        res.on("data", (c) => chunks.push(c));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

function extractTarGz(buf, destDir) {
  const tmp = path.join(destDir, "_archive.tar.gz");
  fs.writeFileSync(tmp, buf);
  execSync(`tar xzf ${JSON.stringify(tmp)} -C ${JSON.stringify(destDir)}`);
  fs.unlinkSync(tmp);
}

function extractZip(buf, destDir) {
  const tmp = path.join(destDir, "_archive.zip");
  fs.writeFileSync(tmp, buf);
  execSync(
    `powershell -NoProfile -Command "Expand-Archive -Force '${tmp}' '${destDir}'"`
  );
  fs.unlinkSync(tmp);
}

async function main() {
  const archive = getArchiveName();
  const url = `https://github.com/${REPO}/releases/download/v${VERSION}/${archive}`;

  console.log(`Downloading ${archive}...`);
  const buf = await fetch(url);

  const binDir = path.join(__dirname, "bin");
  fs.mkdirSync(binDir, { recursive: true });

  if (archive.endsWith(".zip")) {
    extractZip(buf, binDir);
  } else {
    extractTarGz(buf, binDir);
  }

  const binaryName =
    process.platform === "win32" ? "supervisor-mcp.exe" : "supervisor-mcp";
  const binaryPath = path.join(binDir, binaryName);

  if (!fs.existsSync(binaryPath)) {
    throw new Error(`Binary not found at ${binaryPath} after extraction`);
  }

  if (process.platform !== "win32") {
    fs.chmodSync(binaryPath, 0o755);
  }

  console.log("Installed supervisor-mcp binary.");
}

main().catch((err) => {
  console.error(err.message);
  process.exit(1);
});
