package main

import (
	"context"
	"log"
	"net/url"
	"os"

	grafanasdk "github.com/grafana/grafana-openapi-client-go/client"
	"github.com/urfave/cli/v3"
)

func main() {
	cli := &cli.Command{
		Name:  "formulatel-admin",
		Usage: "formulatel admin CLI",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "grafana-admin-user",
				Aliases:  []string{"admin", "grafana-admin"},
				Usage:    "admin user",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "grafana-admin-password",
				Aliases:  []string{"api-key"},
				Usage:    "admin password",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "grafana-url",
				Aliases: []string{"grafana"},
				Usage:   "URL to grafana",
				Value:   "localhost:3000",
			},
			&cli.StringFlag{
				Name:     "grafana-api-scheme",
				Usage:    "http or https; defaults to http",
				Required: false,
				Value:    "http",
			},
			&cli.StringFlag{
				Name:     "connstring",
				Usage:    "DB connection string",
				Required: true,
			},
		},
		// our Before function uses the global api-key and connstring flags to setup the TenantManager
		// that will be used for the admin CLI commands. The manager is added to the returned context
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			adminUser := cmd.String("grafana-admin-user")
			adminPassword := cmd.String("grafana-admin-password")
			grafanaURL := cmd.String("grafana-url")
			grafanaAPIScheme := cmd.String("grafana-api-scheme")
			connStr := cmd.String("connstring")

			grafanaSettings := &grafanasdk.TransportConfig{
				Host:     grafanaURL,
				BasePath: "/api",
				Schemes:  []string{grafanaAPIScheme},
				// don't do this; service accounts are inherently org-scoped and can't create orgs
				// HTTPHeaders: map[string]string{
				// 	"Authentication": fmt.Sprintf("Bearer %s", apiKey),
				// },
				// don't try this; service account keys go in the Auth header, API keys are deprecated
				// APIKey:    apiKey,
				BasicAuth: url.UserPassword(adminUser, adminPassword),
				OrgID:     1,
			}

			manager, err := NewTenantManager(grafanaSettings, connStr)
			if err != nil {
				return ctx, err
			}

			return context.WithValue(ctx, tenantManagerContext, manager), nil
		},
		Commands: []*cli.Command{
			{
				Name:     "tenant",
				Usage:    "Tenant management operations",
				Category: "tenants",
				Commands: []*cli.Command{
					CreateTenant(),
					// DeleteTenant(),
				},
			},
			{
				Name:     "user",
				Usage:    "User management operations",
				Category: "users",
				Commands: []*cli.Command{
					CreateUser(),
					// DeleteUser(),
				},
			},
			{
				Name: "dashboard",
				Usage: "dashboard management operations",
				Category: "dashboards",
				Commands: []*cli.Command{
					CreateDashboard(),
				},
			},
		},
	}

	if err := cli.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
