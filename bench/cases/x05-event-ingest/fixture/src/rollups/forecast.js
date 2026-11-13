export function linearRegression(values) {
  const points = (values ?? []).map((value, index) => ({ x: index, y: Number(value) || 0 }));
  if (points.length < 2) return { slope: 0, intercept: points[0]?.y ?? 0 };
  const meanX = points.reduce((total, point) => total + point.x, 0) / points.length;
  const meanY = points.reduce((total, point) => total + point.y, 0) / points.length;
  let numerator = 0;
  let denominator = 0;
  for (const point of points) {
    numerator += (point.x - meanX) * (point.y - meanY);
    denominator += (point.x - meanX) ** 2;
  }
  const slope = denominator === 0 ? 0 : numerator / denominator;
  return { slope, intercept: meanY - slope * meanX };
}

export function forecast(points, steps = 7) {
  const { slope, intercept } = linearRegression(points.map((point) => point.events ?? point));
  const out = [];
  for (let index = 0; index < steps; index += 1) {
    const x = points.length + index;
    out.push({ step: index + 1, value: Math.max(0, Math.round(intercept + slope * x)) });
  }
  return out;
}

export function movingAverage(values, window = 3) {
  const width = Math.max(1, Math.trunc(window));
  return values.map((value, index) => {
    const slice = values.slice(Math.max(0, index - width + 1), index + 1);
    return Math.round(slice.reduce((total, item) => total + item, 0) / slice.length);
  });
}

export function seasonality(values, period = 7) {
  const buckets = new Array(period).fill(0).map(() => []);
  values.forEach((value, index) => buckets[index % period].push(value));
  return buckets.map((bucket) => (bucket.length === 0 ? 0 : Math.round(bucket.reduce((total, item) => total + item, 0) / bucket.length)));
}

export function trend(values) {
  const { slope } = linearRegression(values);
  if (Math.abs(slope) < 0.01) return 'flat';
  return slope > 0 ? 'up' : 'down';
}

export function anomalyScore(values, value) {
  if (values.length < 3) return 0;
  const mean = values.reduce((total, item) => total + item, 0) / values.length;
  const variance = values.reduce((total, item) => total + (item - mean) ** 2, 0) / values.length;
  const deviation = Math.sqrt(variance);
  return deviation === 0 ? 0 : Math.round(((value - mean) / deviation) * 100) / 100;
}

export function forecastReport(points, steps = 7) {
  const values = points.map((point) => point.events ?? 0);
  return {
    points: values.length,
    trend: trend(values),
    forecast: forecast(points, steps),
    seasonality: seasonality(values, 7),
    total: values.reduce((total, value) => total + value, 0),
  };
}

export function growthRate(values) {
  const list = (values ?? []).map((value) => Number(value) || 0);
  if (list.length < 2) return 0;
  const first = list[0] === 0 ? 1 : list[0];
  return Math.round(((list[list.length - 1] - first) / first) * 10000) / 100;
}

export function movingTotal(values, window = 7) {
  const width = Math.max(1, Math.trunc(window));
  const out = [];
  for (let index = 0; index < values.length; index += 1) {
    out.push(values.slice(Math.max(0, index - width + 1), index + 1).reduce((total, value) => total + value, 0));
  }
  return out;
}

// Capacity planning asks one question: at the slope the series is moving, when does the plan cap run out. The
// answer is the number of steps until the projection passes the cap, or undefined when it never does.
export function stepsToLimit(values, limit) {
  const cap = Number(limit) || 0;
  if (cap <= 0) return 0;
  const { slope, intercept } = linearRegression(values);
  if (slope <= 0) return undefined;
  const start = Math.max(0, intercept);
  if (start >= cap) return 0;
  return Math.ceil((cap - start) / slope);
}

export function capacityRisk(values, limit) {
  const steps = stepsToLimit(values, limit);
  if (steps === undefined) return { risk: 'low', steps: undefined };
  if (steps <= 7) return { risk: 'high', steps };
  if (steps <= 30) return { risk: 'medium', steps };
  return { risk: 'low', steps };
}

export function projectedTotal(values, steps = 7) {
  const { slope, intercept } = linearRegression(values);
  const total = (values ?? []).reduce((sum, value) => sum + (Number(value) || 0), 0);
  const extra = Array.from({ length: steps }, (_, index) => Math.max(0, intercept + slope * (values.length + index)));
  return Math.round(total + extra.reduce((sum, value) => sum + value, 0));
}

export function forecastSeries(points, steps = 7) {
  const values = (points ?? []).map((point) => point.events ?? 0);
  return {
    observed: values.length,
    total: values.reduce((sum, value) => sum + value, 0),
    growth: growthRate(values),
    projected: projectedTotal(values, steps),
    values: forecast(points ?? [], steps).map((entry) => entry.value),
  };
}
