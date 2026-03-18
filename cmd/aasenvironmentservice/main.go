package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	envapi "github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment/api"
	aasrepo "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	cdrepo "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
	smrepo "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	gen "github.com/eclipse-basyx/basyx-go-components/pkg/aasenvironmentapi/go"
)

func runServer(ctx context.Context, configPath string) error {
	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.DBName)

	aasDB, err := aasrepo.NewAssetAdministrationShellDatabase(dsn, cfg.Postgres.MaxOpenConnections, cfg.Postgres.MaxIdleConnections, cfg.Postgres.ConnMaxLifetimeMinutes, "", cfg.Server.StrictVerification)
	if err != nil {
		return err
	}

	smDB, err := smrepo.NewSubmodelDatabase(dsn, cfg.Postgres.MaxOpenConnections, cfg.Postgres.MaxIdleConnections, cfg.Postgres.ConnMaxLifetimeMinutes, "", nil, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}

	cdDB, err := cdrepo.NewConceptDescriptionBackend(dsn, int32(cfg.Postgres.MaxOpenConnections), cfg.Postgres.MaxIdleConnections, cfg.Postgres.ConnMaxLifetimeMinutes, "")
	if err != nil {
		return err
	}

	envSvc := &aasenvironment.AASEnvironment{
		AasBackend: aasDB,
		SmBackend:  smDB,
		CdBackend:  cdDB,
	}

	envAPISvc := envapi.NewAASEnvironmentAPIService(envSvc)
	envAPIController := gen.NewAASEnvironmentAPIController(envAPISvc)

	r := chi.NewRouter()
	r.Use(common.ConfigMiddleware(cfg))
	common.AddCors(r, cfg)
	common.AddHealthEndpoint(r, cfg)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)
	r.Mount(base, envAPIController.Routes())

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("AAS Environment Service listening on %s\n", addr)

	srv := &http.Server{Addr: addr, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(http.ErrServerClosed, err) {
			log.Printf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	return srv.Shutdown(context.Background())
}

func main() {
	configPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(context.Background(), configPath); err != nil {
		log.Fatal(err)
	}
}
