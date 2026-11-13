package store

import (
	"fmt"

	"example.com/billing/internal/query"
)

// PutPlan stores one plan and refreshes the projection.
func (s *Store) PutPlan(plan Plan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureTenant(plan.Tenant); err != nil {
		return err
	}
	if _, exists := s.plans[plan.ID]; exists {
		return fmt.Errorf("%w: plan %q", ErrDuplicate, plan.ID)
	}
	s.plans[plan.ID] = plan
	return s.refresh()
}

// UpdatePlan replaces a plan the store already carries. A new plan is a PutPlan: an update that
// silently created a plan would let a caller write a price list nobody reviewed.
func (s *Store) UpdatePlan(plan Plan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.plans[plan.ID]; !exists {
		return fmt.Errorf("%w: plan %q", ErrNotFound, plan.ID)
	}
	s.plans[plan.ID] = plan
	return s.refresh()
}

// Plan returns one plan.
func (s *Store) Plan(id string) (Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	plan, ok := s.plans[id]
	if !ok {
		return Plan{}, fmt.Errorf("%w: plan %q", ErrNotFound, id)
	}
	return plan, nil
}

// Plans returns the plans of a tenant in id order.
func (s *Store) Plans(tenant string) []Plan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Plan, 0, len(s.plans))
	for _, plan := range s.plans {
		if plan.Tenant == tenant {
			out = append(out, plan)
		}
	}
	sortPlans(out)
	return out
}

// ActivePlans returns the plans of a tenant a subscription may be created on.
func (s *Store) ActivePlans(tenant string) []Plan {
	plans := s.Plans(tenant)
	out := plans[:0]
	for _, plan := range plans {
		if plan.Active {
			out = append(out, plan)
		}
	}
	return out
}

// PlanByCode returns the plan a tenant sells under a code. The code is unique inside a tenant, so two
// tenants may both sell a plan called "team" and neither sees the other's price.
func (s *Store) PlanByCode(tenant, code string) (Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, plan := range s.plans {
		if plan.Tenant == tenant && plan.Code == code {
			return plan, nil
		}
	}
	return Plan{}, fmt.Errorf("%w: plan %q in tenant %q", ErrNotFound, code, tenant)
}

// PlanNames is the code list a report's plan filter accepts for a tenant.
func (s *Store) PlanNames(tenant string) []string {
	plans := s.Plans(tenant)
	codes := make([]string, 0, len(plans))
	for _, plan := range plans {
		codes = append(codes, plan.Code)
	}
	return codes
}

// planFilter is the declaration of the plan filter the subscription and invoice reports accept.
func planFilter() query.Filter {
	return query.Filter{Param: "plan", Column: "plan_code", Op: "="}
}

// sortPlans orders plans by id, which is the order a listing pages through.
func sortPlans(plans []Plan) {
	for i := 1; i < len(plans); i++ {
		for j := i; j > 0 && plans[j].ID < plans[j-1].ID; j-- {
			plans[j], plans[j-1] = plans[j-1], plans[j]
		}
	}
}
