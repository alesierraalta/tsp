const TIMEOUT = Symbol("timeout");

function withTimeout(promise, timeoutMs) {
  let timer;
  const timeout = new Promise((resolve) => {
    timer = setTimeout(() => resolve(TIMEOUT), timeoutMs);
  });
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
}

function logTimeout(item) {
  console.warn(`timed out item ${item.id}`);
}

async function runItem(item, handler, timeoutMs, onTimeout) {
  const value = await withTimeout(Promise.resolve().then(() => handler(item)), timeoutMs);
  if (value === TIMEOUT) {
    onTimeout(item);
    return undefined;
  }
  return { id: item.id, value };
}

export async function runPipeline(items, handler, { timeoutMs = 1_000, onTimeout = logTimeout } = {}) {
  const results = await Promise.all(items.map(async (item) => {
    try {
      return await runItem(item, handler, timeoutMs, onTimeout);
    } catch (cause) {
      const error = new Error(`item ${item.id} failed: ${cause.message}`, { cause });
      error.itemId = item.id;
      throw error;
    }
  }));
  return results.filter((result) => result !== undefined);
}
