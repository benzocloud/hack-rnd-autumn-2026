package httpapi

import (
	"strconv"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func paymentStatus(raw string) (corev1.PaymentStatus, bool) {
	switch raw {
	case "planned":
		return corev1.PaymentStatus_PAYMENT_STATUS_PLANNED, true
	case "partially_matched":
		return corev1.PaymentStatus_PAYMENT_STATUS_PARTIALLY_MATCHED, true
	case "matched":
		return corev1.PaymentStatus_PAYMENT_STATUS_MATCHED, true
	case "cancelled":
		return corev1.PaymentStatus_PAYMENT_STATUS_CANCELLED, true
	default:
		return corev1.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED, false
	}
}
func paymentStatusName(status corev1.PaymentStatus) string {
	switch status {
	case corev1.PaymentStatus_PAYMENT_STATUS_PLANNED:
		return "planned"
	case corev1.PaymentStatus_PAYMENT_STATUS_PARTIALLY_MATCHED:
		return "partially_matched"
	case corev1.PaymentStatus_PAYMENT_STATUS_MATCHED:
		return "matched"
	case corev1.PaymentStatus_PAYMENT_STATUS_CANCELLED:
		return "cancelled"
	case corev1.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}
func paymentDTO(p *corev1.ExpectedPayment, account string) (object, error) {
	if p == nil || !resourceID(p.Id) || p.AccountId != account || !contains(directionName(p.Direction), "inflow", "outflow") || !positiveMoney(p.Amount) || !nonnegativeMoney(p.RemainingAmount) || p.RemainingAmount.AmountMinor > p.Amount.AmountMinor || !businessDate(p.ScheduledDate) || !optionalID(p.CategoryId) || !optionalID(p.ReconciliationGroupId) || !bounded(p.CounterpartyName, 500) || !bounded(p.Description, 2000) || p.Version < 1 || !timestamp(p.CreatedAt) || p.Source == nil || !validSource(p.Source) {
		return nil, errProjection
	}
	status := paymentStatusName(p.Status)
	if status == "" {
		return nil, errProjection
	}
	switch status {
	case "planned":
		if p.RemainingAmount.AmountMinor != p.Amount.AmountMinor {
			return nil, errProjection
		}
	case "partially_matched":
		if p.RemainingAmount.AmountMinor == 0 || p.RemainingAmount.AmountMinor == p.Amount.AmountMinor {
			return nil, errProjection
		}
	case "matched":
		if p.RemainingAmount.AmountMinor != 0 {
			return nil, errProjection
		}
	case "cancelled":
		if p.RemainingAmount.AmountMinor == 0 {
			return nil, errProjection
		}
	}
	source := p.Source
	if source.Kind == corev1.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL {
		if source.SourceSystem != "manual_v1" || source.SourceCompanyId != nil || source.ExternalOperationId != nil || source.ExternalPaymentId != nil || source.ImportId != nil || source.ConnectionId != nil {
			return nil, errProjection
		}
	} else if source.SourceCompanyId == nil || source.ExternalPaymentId == nil || source.ExternalOperationId != nil {
		return nil, errProjection
	}
	return object{"id": p.Id, "account_id": p.AccountId, "direction": directionName(p.Direction), "amount": moneyDTO(p.Amount), "remaining_amount": moneyDTO(p.RemainingAmount), "scheduled_date": p.ScheduledDate, "category_id": textPointer(p.CategoryId), "counterparty_name": p.CounterpartyName, "description": p.Description, "status": status, "source": sourceDTO(p.Source), "version": strconv.FormatInt(p.Version, 10), "created_at": utc(p.CreatedAt), "reconciliation_group_id": textPointer(p.ReconciliationGroupId)}, nil
}
func paymentMatchDTO(m *corev1.PaymentMatch, payment string) (object, error) {
	if m == nil || !resourceID(m.Id) || m.PaymentId != payment || !resourceID(m.TransactionId) || !positiveMoney(m.Amount) || !timestamp(m.CreatedAt) || m.RemovedAt != nil && (!timestamp(m.RemovedAt) || m.RemovedAt.AsTime().Before(m.CreatedAt.AsTime())) {
		return nil, errProjection
	}
	var removed any
	if m.RemovedAt != nil {
		removed = utc(m.RemovedAt)
	}
	return object{"id": m.Id, "payment_id": m.PaymentId, "transaction_id": m.TransactionId, "amount": moneyDTO(m.Amount), "created_at": utc(m.CreatedAt), "removed_at": removed}, nil
}
