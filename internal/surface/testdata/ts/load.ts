import { readFileSync } from "node:fs";

export function load(name: string, count: number): string {
  const body = readFileSync(name, "utf8");
  return body.repeat(count);
}
