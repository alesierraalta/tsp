export class Timers {
  constructor(clock) {
    this.clock = clock;
    this.offset = clock.wall - clock.mono;
    this.pending = [];
  }
  at(wallDeadline, callback) {
    this.pending.push({absolute:wallDeadline, due:wallDeadline - this.offset, callback});
  }
  after(delay, callback) {
    this.pending.push({due:this.clock.mono + delay, callback});
  }
  poll() {
    const ready = [], waiting = [];
    for (const timer of this.pending) {
      const expired = timer.wallDue === undefined ? (timer.absolute === undefined ? this.clock.mono >= timer.due : this.clock.wall >= timer.absolute) : this.clock.wall >= timer.wallDue;
      (expired ? ready : waiting).push(timer);
    }
    this.pending = waiting;
    for (const timer of ready) timer.callback();
    return ready.length;
  }
}
