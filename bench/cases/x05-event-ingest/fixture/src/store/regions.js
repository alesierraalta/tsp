export const REGIONS = { 'eu-west-1': { label: 'Ireland', latency: 12 }, 'us-east-1': { label: 'Virginia', latency: 84 }, 'ap-south-1': { label: 'Mumbai', latency: 140 } };

export function regionOf(value, fallback = 'eu-west-1') {
  return REGIONS[String(value)] ? String(value) : fallback;
}

export function nearestRegion(latencyTable) {
  const entries = Object.entries(latencyTable ?? REGIONS);
  return entries.reduce((best, entry) => (entry[1].latency < best[1].latency ? entry : best))[0];
}

export function replicaPlan(tenant, regions = ['eu-west-1', 'us-east-1']) {
  return regions.map((region, index) => ({
    region,
    role: index === 0 ? 'primary' : 'replica',
    latency: (REGIONS[region] ?? { latency: 100 }).latency,
    tenant,
  }));
}

export function failover(plan, failedRegion) {
  const primary = plan.find((entry) => entry.role === 'primary');
  if (!primary || primary.region !== failedRegion) return plan;
  const next = plan.find((entry) => entry.role === 'replica');
  if (!next) return plan;
  return plan.map((entry) => ({
    ...entry,
    role: entry.region === next.region ? 'primary' : entry.region === primary.region ? 'replica' : entry.role,
  }));
}

export function regionReport(plan) {
  return plan.map((entry) => ({ region: entry.region, role: entry.role, latency: entry.latency }));
}

export function latencyBudget(plan, limit = 100) {
  return plan.filter((entry) => entry.latency > limit).map((entry) => entry.region);
}

export function regionLabels(names) {
  return (names ?? []).map((name) => REGIONS[name]?.label ?? name);
}

export function replicationPlan(tenants, regions = ['eu-west-1', 'us-east-1', 'ap-south-1']) {
  return (tenants ?? []).map((tenant) => ({
    tenant,
    replicas: replicaPlan(tenant, regions),
    primary: regions[0],
    read_regions: regions.slice(1),
  }));
}

export function promote(plan, region) {
  const replica = plan.find((entry) => entry.region === region);
  if (!replica) return plan;
  return plan.map((entry) => ({ ...entry, role: entry.region === region ? 'primary' : 'replica' }));
}

export function regionLoad(plans) {
  const load = {};
  for (const plan of plans ?? []) {
    for (const entry of plan.replicas ?? []) load[entry.region] = (load[entry.region] ?? 0) + 1;
  }
  return load;
}

export function slowestReplica(plans, limit = 100) {
  const candidates = (plans ?? []).flatMap((plan) => (plan.replicas ?? []).filter((entry) => entry.latency > limit));
  return candidates.sort((left, right) => right.latency - left.latency)[0];
}

export function regionPlanReport(plans) {
  return {
    tenants: (plans ?? []).length,
    regions: Object.keys(regionLoad(plans)).length,
    load: regionLoad(plans),
    slow: slowestReplica(plans)?.region ?? null,
  };
}
