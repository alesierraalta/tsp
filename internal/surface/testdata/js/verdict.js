import fs from "node:fs";

export function load(name) {
  const exists = fs.existsSync(name);
  if (!exists) {
    return null;
  }
  return fs.readFileSync(name);
}
