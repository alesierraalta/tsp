export class TransferError extends Error {
  constructor(message, retryable = true) {
    super(message);
    this.name = 'TransferError';
    this.retryable = retryable;
  }
}

export async function retry(action, attempts, observe = () => {}) {
  if (!Number.isInteger(attempts) || attempts < 1) {
    throw new RangeError('attempts must be positive');
  }
  for (let attempt = 1; attempt <= attempts; attempt++) {
    try {
      const value = await action(attempt);
      observe({ attempt, status: 'success' });
      return value;
    } catch (error) {
      observe({ attempt, status: 'failure', message: error.message });
      if (!(error instanceof TransferError) || !error.retryable || attempt === attempts) {
        throw error;
      }
    }
  }
}

export function summarize(events) {
  return events.reduce((result, event) => {
    result.attempts++;
    if (event.status === 'success') result.succeeded++;
    else result.failed++;
    return result;
  }, { attempts: 0, succeeded: 0, failed: 0 });
}
