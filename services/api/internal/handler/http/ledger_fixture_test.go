package httpapi

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	corev1 "github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/core/v1"
)

func (f *financialFixture) ListCategories(context.Context, *corev1.ListCategoriesRequest, ...grpc.CallOption) (*corev1.ListCategoriesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused fixture method")
}
func (f *financialFixture) ListTransactions(context.Context, *corev1.ListTransactionsRequest, ...grpc.CallOption) (*corev1.ListTransactionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused fixture method")
}
func (f *financialFixture) UpdateTransactionCategory(context.Context, *corev1.UpdateTransactionCategoryRequest, ...grpc.CallOption) (*corev1.UpdateTransactionCategoryResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused fixture method")
}
