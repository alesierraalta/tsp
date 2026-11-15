// Post-import hook: prints the payload file name it was handed.
const payload = process.argv[2] ?? '';
console.log(`tagged ${payload}`);
