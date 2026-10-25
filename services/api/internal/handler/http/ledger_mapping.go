package httpapi

import (
	"strconv"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func transactionDTO(transaction *corev1.Transaction, account string) (object, error) {
	if transaction == nil || !resourceID(transaction.Id) || transaction.AccountId != account || !businessDate(transaction.OperationDate) || !contains(directionName(transaction.Direction), "inflow", "outflow") || !positiveMoney(transaction.Amount) || transaction.CounterpartyInn != nil && !innPattern.MatchString(*transaction.CounterpartyInn) || !bounded(transaction.CounterpartyName, 500) || !bounded(transaction.PaymentPurpose, 2000) || !optionalID(transaction.CategoryId) || transaction.Source == nil || !validSource(transaction.Source) || transaction.Version < 1 || (transaction.CreatedAt != nil && !timestamp(transaction.CreatedAt)) {
		return nil, errProjection
	}
	source := transaction.Source
	if source.Kind == corev1.RecordSourceKind_RECORD_SOURCE_KIND_MANUAL {
		if source.SourceSystem != "manual_v1" || source.SourceCompanyId != nil || source.ExternalOperationId != nil || source.ExternalPaymentId != nil || source.ImportId != nil || source.ConnectionId != nil {
			return nil, errProjection
		}
	} else if source.SourceCompanyId == nil || source.ExternalOperationId == nil || source.ExternalPaymentId != nil {
		return nil, errProjection
	}
	var createdAt any
	// Migration 00005 preserves unknown legacy creation times as NULL.
	// Keep that absence instead of inventing the operation's creation time.
	if transaction.CreatedAt != nil {
		createdAt = utc(transaction.CreatedAt)
	}
	return object{"id": transaction.Id, "account_id": transaction.AccountId, "operation_date": transaction.OperationDate, "direction": directionName(transaction.Direction), "amount": moneyDTO(transaction.Amount), "counterparty_inn": textPointer(transaction.CounterpartyInn), "counterparty_name": transaction.CounterpartyName, "payment_purpose": transaction.PaymentPurpose, "category_id": textPointer(transaction.CategoryId), "source": sourceDTO(transaction.Source), "version": strconv.FormatInt(transaction.Version, 10), "created_at": createdAt}, nil
}
