package company

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	domain "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/domain/company"
	authpb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/auth/v1"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/company"
)

type operations interface {
	Bootstrap(context.Context, company.BootstrapCommand) (company.BootstrapResult, error)
	GetBootstrapResult(context.Context, uuid.UUID) (company.BootstrapResult, error)
	ReadProfile(context.Context, uuid.UUID) (domain.Profile, error)
	Update(context.Context, company.UpdateCommand) (domain.Profile, error)
}

type authorizer interface {
	AuthorizeCompany(context.Context, *authpb.AuthorizeCompanyRequest) (*authpb.AuthorizeCompanyResponse, error)
}

type handler struct {
	pb.UnimplementedCompanyServiceServer
	companies operations
	auth      authorizer
}

func Register(server grpc.ServiceRegistrar, companies operations, auth authorizer) {
	pb.RegisterCompanyServiceServer(server, &handler{companies: companies, auth: auth})
}
