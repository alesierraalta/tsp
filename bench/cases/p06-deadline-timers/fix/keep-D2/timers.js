export class Timers {
  constructor(clock) {
    this.clock = clock;
    this.pending = [];
  }
  at(wallDeadline, callback) {
    this.pending.push({wallDue:wallDeadline, callback});
  }
  after(delay, callback) {
    this.pending.push({wallDue:this.clock.wall + delay, callback});
  }
  poll() {
    const ready = [], waiting = [];
    for (const timer of this.pending) {
      const expired = timer.wallDue === undefined ? this.clock.mono >= timer.due : this.clock.wall >= timer.wallDue;
      (expired ? ready : waiting).push(timer);
    }
    this.pending = waiting;
    for (const timer of ready) timer.callback();
    return ready.length;
  }
}
