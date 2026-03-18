package openapi

import (
	"context"
	"io"
	"net/http"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// AASEnvironmentAPIRouter defines the required methods for binding the api requests to a response for the AASEnvironmentAPI
type AASEnvironmentAPIRouter interface {
	PostUpload(http.ResponseWriter, *http.Request)
}

// AASEnvironmentAPIServicer defines the api actions for the AASEnvironmentAPI service
type AASEnvironmentAPIServicer interface {
	PostUpload(ctx context.Context, env types.IEnvironment, files map[string]io.Reader, ignoreDuplicates bool) (model.ImplResponse, error)
}
