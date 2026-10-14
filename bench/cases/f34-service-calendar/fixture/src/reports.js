import { dayKey, monthKey, format, range } from './dates.js';

export function agenda(reservations, day) {
  const key = dayKey(day);
  return reservations.list()
    .filter(item => dayKey(item.start) === key)
    .sort((a, b) => a.start - b.start || a.id.localeCompare(b.id))
    .map(item => ({ id: item.id, service: item.service, start: format(item.start),
      end: format(item.end), status: item.status }));
}

export function monthly(reservations, month) {
  const key = monthKey(month);
  const result = {};
  for (const item of reservations.list()) {
    if (monthKey(item.start) !== key) continue;
    result[item.service] = (result[item.service] ?? 0) + 1;
  }
  return result;
}

export function utilization(catalog, reservations) {
  return catalog.list().map(service => ({
    service: service.id,
    occupied: reservations.occupancy(service.id),
    capacity: service.capacity,
    enabled: service.enabled,
  }));
}

export function calendar(reservations, start, count) {
  return range(start, count).map(day => ({ day, items: agenda(reservations, day) }));
}

export function exportRows(reservations) {
  return reservations.list().map(item => [
    item.id,
    item.service,
    format(item.start),
    format(item.end),
    item.status,
  ]);
}

export function summary(catalog, reservations) {
  const rows = utilization(catalog, reservations);
  return {
    services: rows.length,
    enabled: rows.filter(row => row.enabled).length,
    occupied: rows.reduce((total, row) => total + row.occupied, 0),
  };
}
