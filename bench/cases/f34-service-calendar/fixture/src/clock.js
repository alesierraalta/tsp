export class Clock {
  #wall;
  #elapsed;

  constructor(wall = Date.now(), elapsed = 0) {
    this.#validate(wall);
    this.#validate(elapsed);
    this.#wall = wall;
    this.#elapsed = elapsed;
  }

  #validate(value) {
    if (!Number.isSafeInteger(value) || value < 0) throw new TypeError('clock value');
  }

  wall() {
    return this.#wall;
  }

  elapsed() {
    return this.#elapsed;
  }

  advance(duration) {
    this.#validate(duration);
    this.#validate(this.#wall + duration);
    this.#validate(this.#elapsed + duration);
    this.#wall += duration;
    this.#elapsed += duration;
    return this.snapshot();
  }

  adjustWall(delta) {
    if (!Number.isSafeInteger(delta)) throw new TypeError('adjustment');
    this.#validate(this.#wall + delta);
    this.#wall += delta;
    return this.#wall;
  }

  snapshot() {
    return Object.freeze({ wall: this.#wall, elapsed: this.#elapsed });
  }

  measure(operation) {
    const start = this.elapsed();
    const value = operation();
    return { value, duration: this.elapsed() - start };
  }
}
