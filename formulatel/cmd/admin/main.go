package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"
)

func main() {
	cli := &cli.Command{
		Name:  "formulatel-admin",
		Usage: "formulatel admin CLI",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "admin-password",
				Aliases:  []string{"api-key"},
				Usage:    "admin password",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "connstring",
				Usage:    "DB connection string",
				Required: true,
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			// At this layer, cmd represents the root command, so flags parsed
			// directly after the binary invocation are evaluated right here.
			apiKey := cmd.String("api-key")
			connStr := cmd.String("connstring")

			manager, err := NewTenantManager(apiKey, connStr)
			if err != nil {
				return ctx, err
			}

			// Wrap the initialized pointer into the context lifecycle
			enrichedCtx := context.WithValue(ctx, tenantManagerContext, manager)

			// Return the new context. Everything executing downstream will inherit this.
			return enrichedCtx, nil
		},
		Commands: []*cli.Command{
			{
				Name:     "tenant",
				Usage:    "Tenant management operations",
				Category: "tenants",
				Commands: []*cli.Command{
					CreateTenant(),
				},
			},
		},
	}

	if err := cli.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
