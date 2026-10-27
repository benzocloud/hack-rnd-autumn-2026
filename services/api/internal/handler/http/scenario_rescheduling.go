package httpapi

import (
	"encoding/json"
	"strings"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func delayChangeBody(f map[string]json.RawMessage) (*corev1.ScenarioChange, bool) {
	for key := range f {
		if !contains(key, "id", "type", "target", "amount", "new_date") {
			return nil, false
		}
	}
	v := &corev1.DelayReceiptChange{}
	amount, valid := paymentMoney(f["amount"])
	if !valid || json.Unmarshal(f["id"], &v.Id) != nil || !resourceID(v.Id) || json.Unmarshal(f["new_date"], &v.NewDate) != nil || !businessDate(v.NewDate) {
		return nil, false
	}
	v.Amount = amount
	var target map[string]json.RawMessage
	if json.Unmarshal(f["target"], &target) != nil || len(target) != 2 {
		return nil, false
	}
	v.Target = &corev1.OccurrenceTarget{}
	if json.Unmarshal(target["component_id"], &v.Target.ComponentId) != nil || json.Unmarshal(target["occurrence_id"], &v.Target.OccurrenceId) != nil || !nonempty(v.Target.ComponentId, 128) || !nonempty(v.Target.OccurrenceId, 128) {
		return nil, false
	}
	return &corev1.ScenarioChange{Change: &corev1.ScenarioChange_DelayReceipt{DelayReceipt: v}}, true
}
func responseActionBody(raw json.RawMessage) (*corev1.ReschedulePaymentPart, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 5 {
		return nil, false
	}
	for key := range fields {
		if !contains(key, "type", "target", "amount", "new_date", "feasibility_condition") {
			return nil, false
		}
	}
	var kind string
	v := &corev1.ReschedulePaymentPart{}
	amount, valid := paymentMoney(fields["amount"])
	if !valid || json.Unmarshal(fields["type"], &kind) != nil || kind != "reschedule_payment_part" || json.Unmarshal(fields["new_date"], &v.NewDate) != nil || !businessDate(v.NewDate) || json.Unmarshal(fields["feasibility_condition"], &v.FeasibilityCondition) != nil || !nonempty(v.FeasibilityCondition, 1000) || strings.TrimSpace(v.FeasibilityCondition) == "" {
		return nil, false
	}
	v.Amount = amount
	var target map[string]json.RawMessage
	if json.Unmarshal(fields["target"], &target) != nil || len(target) != 1 || json.Unmarshal(target["effective_occurrence_id"], &v.EffectiveOccurrenceId) != nil || !nonempty(v.EffectiveOccurrenceId, 128) {
		return nil, false
	}
	return v, true
}
