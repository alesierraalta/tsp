export function checkSignal(signal) {
  if (signal?.aborted) {
    throw signal.reason ?? new Error('subscription cancelled');
  }
}

export function delay(milliseconds, signal) {
  checkSignal(signal);
  return new Promise((resolve, reject) => {
    const finish = () => {
      signal?.removeEventListener('abort', cancel);
      resolve();
    };
    const timer = setTimeout(finish, milliseconds);
    const cancel = () => {
      clearTimeout(timer);
      signal?.removeEventListener('abort', cancel);
      reject(signal.reason ?? new Error('subscription cancelled'));
    };
    signal?.addEventListener('abort', cancel, { once: true });
  });
}

export async function checkpoint(signal) {
  checkSignal(signal);
  await Promise.resolve();
  checkSignal(signal);
}
