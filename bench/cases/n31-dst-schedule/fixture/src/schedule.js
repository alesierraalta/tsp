export function withJitter(runAt, random = Math.random) {
  const offsetMinutes = Math.floor(random() * 11) - 5;
  return new Date(runAt.getTime() + offsetMinutes * 60_000);
}

const MINUTES_PER_DAY = 24 * 60;

function startOfLocalDay(date, offset = 0) {
  const day = new Date(date);
  day.setHours(0, 0, 0, 0);
  day.setDate(day.getDate() + offset);
  return day;
}

function firstValidTime(day, hour, minute) {
  const requestedMinute = hour * 60 + minute;
  for (let minuteOfDay = requestedMinute; minuteOfDay < MINUTES_PER_DAY; minuteOfDay += 1) {
    const candidate = new Date(day);
    const candidateHour = Math.floor(minuteOfDay / 60);
    const candidateMinute = minuteOfDay % 60;
    candidate.setHours(candidateHour, candidateMinute, 0, 0);
    if (
      candidate.getFullYear() === day.getFullYear() &&
      candidate.getMonth() === day.getMonth() &&
      candidate.getDate() === day.getDate() &&
      candidate.getHours() === candidateHour &&
      candidate.getMinutes() === candidateMinute
    ) {
      return candidate;
    }
  }
  throw new RangeError("no valid scheduled minute remains on this local date");
}

export function nextRunAt({ hour = 2, minute = 30, now = () => new Date() } = {}) {
  if (!Number.isInteger(hour) || hour < 0 || hour > 23) throw new RangeError("hour must be 0–23");
  if (!Number.isInteger(minute) || minute < 0 || minute > 59) throw new RangeError("minute must be 0–59");

  const current = now();
  const today = startOfLocalDay(current);
  const todayRun = firstValidTime(today, hour, minute);
  if (todayRun > current) return todayRun;
  return new Date(todayRun.getTime() + 24 * 60 * 60 * 1000);
}
