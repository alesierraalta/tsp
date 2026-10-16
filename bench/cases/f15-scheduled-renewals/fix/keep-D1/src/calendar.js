export class RenewalCalendar {
  constructor(start) {
    this.current = new Date(start);
    this.anchorDay = this.current.getUTCDate();
  }

  advance() {
    const year = this.current.getUTCFullYear();
    const month = this.current.getUTCMonth() + 1;
    const lastDay = new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
    const day = Math.min(this.current.getUTCDate(), lastDay);
    this.current = new Date(Date.UTC(year, month, day));
    return this.current.toISOString().slice(0, 10);
  }
}
