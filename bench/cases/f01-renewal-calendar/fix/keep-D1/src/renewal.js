export function monthlyRenewal(day) {
  const date = new Date(`${day}T00:00:00Z`);
  date.setUTCMonth(date.getUTCMonth() + 1);
  return date.toISOString().slice(0, 10);
}

export function annualRenewal(day) {
  const date = new Date(`${day}T00:00:00Z`);
  const dayNumber = date.getUTCDate();
  date.setUTCFullYear(date.getUTCFullYear() + 1, date.getUTCMonth(), 1);
  const lastDay = new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth() + 1, 0)).getUTCDate();
  date.setUTCDate(Math.min(dayNumber, lastDay));
  return date.toISOString().slice(0, 10);
}
