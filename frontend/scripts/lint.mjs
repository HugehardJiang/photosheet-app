import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "src");
const sourceFiles = [];

function collect(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      collect(entryPath);
    } else if (/\.(ts|tsx)$/.test(entry.name)) {
      sourceFiles.push(entryPath);
    }
  }
}

collect(sourceRoot);

const rules = [
  { pattern: /\bany\b/, message: "avoid implicit or explicit any" },
  { pattern: /console\.(log|debug|info|warn|error)\s*\(/, message: "remove console logging from production source" },
  { pattern: /(?:C:\\Users\\|\/Users\/|\/home\/)[^"'\s]+/, message: "do not commit a developer-specific filesystem path" },
];

let failed = false;
for (const filePath of sourceFiles) {
  const source = fs.readFileSync(filePath, "utf8");
  for (const rule of rules) {
    if (rule.pattern.test(source)) {
      console.error(filePath + ": " + rule.message);
      failed = true;
    }
  }
}

if (failed) {
  process.exit(1);
}

console.log("Frontend lint passed (" + sourceFiles.length + " TypeScript files).");
