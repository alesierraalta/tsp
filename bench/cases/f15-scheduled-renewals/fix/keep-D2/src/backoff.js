export class RetrySchedule {
  constructor(baseDelay, maxDelay) {
    this.baseDelay = baseDelay;
    this.maxDelay = maxDelay;
    this.attempt = 0;
    this.elapsed = 0;
  }

  next() {
    const delay = Math.min(this.maxDelay, this.baseDelay * (this.attempt + 1));
    this.attempt += 1;
    this.elapsed += delay;
    return this.elapsed;
  }
}
