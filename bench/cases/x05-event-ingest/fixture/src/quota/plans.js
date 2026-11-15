export const PLANS = {
  free: { name: 'free', limits: { max_rows: 500, max_samples: 2000, max_bytes: 256 * 1024 } },
  growth: { name: 'growth', limits: { max_rows: 5000, max_samples: 100000, max_bytes: 4 * 1024 * 1024 } },
  scale: { name: 'scale', limits: { max_rows: 20000, max_samples: 400000, max_bytes: 16 * 1024 * 1024 } },
};

export class PlanBook {
  constructor(plans = PLANS) {
    this.plans = new Map(Object.entries(plans));
    this.assignments = new Map();
  }

  get(name) {
    return this.plans.get(String(name)) ?? this.plans.get('free');
  }

  for(tenant) {
    return this.get(this.assignments.get(tenant) ?? 'free');
  }

  assign(tenant, planName) {
    if (!this.plans.has(String(planName))) throw new TypeError(`unknown plan ${planName}`);
    this.assignments.set(tenant, String(planName));
    return this.get(planName);
  }

  list() {
    return [...this.plans.values()].map((plan) => ({
      name: plan.name,
      rows: plan.limits.max_rows,
      samples: plan.limits.max_samples,
      bytes: plan.limits.max_bytes,
    }));
  }

  tenantsOn(planName) {
    return [...this.assignments.entries()].filter(([, name]) => name === planName).map(([tenant]) => tenant);
  }

  stats() {
    const out = {};
    for (const planName of this.assignments.values()) out[planName] = (out[planName] ?? 0) + 1;
    return { plans: this.plans.size, assignments: this.assignments.size, byPlan: out };
  }
}

export function planRank(planName) {
  return Object.keys(PLANS).indexOf(String(planName));
}

export function isUpgrade(from, to) {
  return planRank(to) > planRank(from);
}
