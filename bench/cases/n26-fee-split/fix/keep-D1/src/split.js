import { pathToFileURL } from 'node:url';

function assertPartCount(partCount) {
  if (!Number.isInteger(partCount) || partCount < 1) throw new Error('usage: split <total> <parts>');
  if (partCount > 100) throw new Error('usage: split <total> <parts> (maximum 100)');
}

export function split(total, partCount) {
  assertPartCount(partCount);
  if (!Number.isSafeInteger(total) || total < 0) {
    throw new Error('usage: split <total> <parts> (total must be a non-negative safe integer)');
  }
  const share = Math.floor(total / partCount);
  return Array.from({ length: partCount }, () => share);
}

function usage(err) {
  err.write('usage: split <total> <parts>\n');
  return 2;
}

export function run(argv, { out = process.stdout, err = process.stderr } = {}) {
  if (argv.length !== 2 || argv.some((value) => !/^\d+$/.test(value))) return usage(err);
  try {
    const parts = split(Number(argv[0]), Number(argv[1]));
    out.write(`${parts.join(',')}\n`);
    return 0;
  } catch (error) {
    err.write(`${error.message}\n`);
    return 2;
  }
}

if (process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url) {
  process.exitCode = run(process.argv.slice(2));
}
