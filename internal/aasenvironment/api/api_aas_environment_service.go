package api

import (
	"context"
	"io"
	"net/http"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	gen "github.com/eclipse-basyx/basyx-go-components/pkg/aasenvironmentapi/go"
)

type AASEnvironmentAPIService struct {
	envSvc *aasenvironment.AASEnvironment
}

func NewAASEnvironmentAPIService(envSvc *aasenvironment.AASEnvironment) gen.AASEnvironmentAPIServicer {
	return &AASEnvironmentAPIService{
		envSvc: envSvc,
	}
}

func (s *AASEnvironmentAPIService) PostUpload(ctx context.Context, env types.IEnvironment, files map[string]io.Reader, ignoreDuplicates bool) (model.ImplResponse, error) {
	if err := s.envSvc.LoadEnvironment(ctx, env, files, ignoreDuplicates); err != nil {
		return model.ImplResponse{Code: http.StatusInternalServerError, Body: err.Error()}, err
	}
	return model.ImplResponse{Code: http.StatusAccepted, Body: "Environment uploaded successfully"}, nil
}
