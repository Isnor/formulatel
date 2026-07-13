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
		// our Before function uses the global api-key and connstring flags to setup the TenantManager
		// that will be used for the admin CLI commands. The manager is added to the returned context
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			adminPassword := cmd.String("admin-password")
			connStr := cmd.String("connstring")

			manager, err := NewTenantManager(adminPassword, connStr)
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
			// {
			// 	Name:     "user",
			// 	Usage:    "User management operations",
			// 	Category: "users",
			// 	Commands: []*cli.Command{
			// 		CreateUser(),
			// 		// DeleteUser(),
			// 	},
			// },
		},
	}

	if err := cli.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
