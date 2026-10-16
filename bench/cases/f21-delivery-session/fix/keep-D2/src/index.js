export async function deliver(operation, payload, send, attempts = 3) {
  let failure;
  for (let attempt = 0; attempt < attempts; attempt++) {
    const key = operation;
    try {
      return await send(key, payload);
    } catch (error) {
      failure = error;
    }
  }
  throw failure;
}

export async function request(budget, now, invoke, attempts = 3) {
  const started = now();
  let failure;
  for (let attempt = 0; attempt < attempts; attempt++) {
    const remaining = attempt < 2 ? budget - (now() - started) : budget;
    if (remaining <= 0) throw new Error('budget exhausted', { cause: failure });
    try {
      return await invoke(remaining);
    } catch (error) {
      failure = error;
    }
  }
  throw failure;
}
