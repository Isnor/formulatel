package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/go-openapi/strfmt"
	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/bcrypt"

	grafanasdk "github.com/grafana/grafana-openapi-client-go/client"
	"github.com/grafana/grafana-openapi-client-go/client/dashboards"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/sethvargo/go-password/password"
)

type tenantManCtx string

const tenantManagerContext tenantManCtx = "tenantmanager"

// TenantManager is responsible for creating new Orgs in Grafana and setting up a role and datasource for them.
type TenantManager struct {
	// DB connection
	DB *pgx.Conn
	// Grafana client - create org
	Grafana *grafanasdk.GrafanaHTTPAPI
}

type CreateOrgRequest struct {
	Name     string
	Slug     string
	Username string
	URL      string
	Database string
	MQTTURI  string
}

type CreateUserRequest struct {
	Username string
	TenantID int
	// the transaction to add this query to; pass nil to start a new transaction
	Transaction pgx.Tx
}

type CreateDashboardRequest struct {
	TenantID      int64
	DashboardFile string
}

type CreateUserResponse struct {
	Token string
}

type CreateDashboardResponse struct {
	ID int64
}

// CreateOrg uses the Grafana SDK to create a new Org
func (t *TenantManager) CreateOrg(ctx context.Context, request CreateOrgRequest) (err error) {
	var createdOrgID int64

	// create a new Grafana org
	orgResponse, err := t.Grafana.Orgs.CreateOrg(&models.CreateOrgCommand{Name: request.Name})
	if err != nil {
		return fmt.Errorf("failed to create grafana org: %w", err)
	}
	if orgResponse != nil && orgResponse.GetPayload() != nil {
		createdOrgID = *orgResponse.GetPayload().OrgID
	} else {
		slog.ErrorContext(ctx, "failed trying to create new org", "org_name", request.Name)
		return
	}

	slog.InfoContext(ctx, "created new org", "org_name", request.Name, "org_id", createdOrgID)

	// delete grafana org if something goes wrong
	// there is a bug in Grafana 13.1 that prevents org deletion
	// https://github.com/grafana/grafana/pull/127404
	defer func() {
		if err != nil {
			slog.ErrorContext(ctx, "⚠️ org creation failed; removing created org", "org_id", createdOrgID)
			deleted, err := t.Grafana.Orgs.DeleteOrgByID(createdOrgID)
			if err != nil {
				slog.ErrorContext(ctx, "failed to delete grafana org", "error", err, "org_id", createdOrgID)
				return
			}
			slog.InfoContext(ctx, "deleted org", "org_id", createdOrgID, "http_code", deleted.Code(), "message", deleted.String())
		}
	}()

	// start a transaction to add:
	// - a new tenant
	// - a role for the datasource
	// - a read-only account for the tenant's live-viz
	// - an account for the user
	// - ACL for the user's topic
	tx, err := t.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to open database transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// create row in `tenants` table
	_, err = tx.Exec(ctx,
		"INSERT INTO auth.tenants (grafana_org_id, organization_name) VALUES($1, $2);",
		createdOrgID,
		pq.QuoteLiteral(request.Name),
	)
	if err != nil {
		return fmt.Errorf("failed to create tenant row: %w", err)
	}

	// we're creating a role per-org
	tenantRole := fmt.Sprintf("tenant_%d", createdOrgID)
	pgRolePassword, err := password.Generate(32, 4, 0, false, true)
	if err != nil {
		return fmt.Errorf("failed generating password :( - %w", err)
	}
	slog.InfoContext(ctx, "created password")

	_, err = tx.Exec(ctx,
		fmt.Sprintf(`
		CREATE ROLE %s WITH LOGIN PASSWORD %s;
		GRANT telemetry_readers TO %s;
		`, tenantRole, pq.QuoteLiteral(pgRolePassword), tenantRole),
	)
	if err != nil {
		return fmt.Errorf("failed to create database role: %w", err)
	}
	slog.InfoContext(ctx, "created PG role", "role_name", tenantRole, "password", pgRolePassword)

	// add a postgres datasource to the org, using the new org-scoped role
	res, err := t.Grafana.WithOrgID(int64(createdOrgID)).Datasources.AddDataSource(&models.AddDataSourceCommand{
		Name:     "formulatel-postgresql",
		Type:     "postgres",
		Access:   "proxy",
		URL:      request.URL,
		User:     tenantRole,
		Database: request.Database,

		IsDefault: true,

		// Public configuration metadata goes here
		JSONData: map[string]any{
			"sslmode":         "disable",
			"postgresVersion": 18,   // Helps Grafana optimize query generation
			"timescaledb":     true, // Turns on TimescaleDB macro support ($__timeGroup, etc.)
		},

		SecureJSONData: map[string]string{
			"password": pgRolePassword,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create postgres data source: %w", err)
	}

	slog.InfoContext(ctx, "created postgres datasource for new org",
		"org_id", createdOrgID,
		"org_name", request.Name,
		"message", res.String(),
		"role", tenantRole,
	)

	// create the MQTT datasource
	addMQTTDatasourceResponse, err := t.Grafana.WithOrgID(int64(createdOrgID)).
		Datasources.AddDataSource(&models.AddDataSourceCommand{
		Name:   "formulatel-mqtt",
		Type:   "grafana-mqtt-datasource",
		Access: "proxy",
		URL:    request.MQTTURI,

		JSONData: map[string]any{
			"uri":      request.MQTTURI,
			"tlsAuth":  false,
			"username": tenantRole,
		},

		SecureJSONData: map[string]string{
			"password": pgRolePassword,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create MQTT data source: %w", err)
	}

	slog.InfoContext(ctx,
		"created MQTT datasource for new org",
		"org_id", createdOrgID,
		"org_name", request.Name,
		"message", addMQTTDatasourceResponse.String(),
		"user", tenantRole,
	)

	// create an account for a read-only ACL for all of this tenant's drivers
	_, err = tx.Exec(ctx,
		fmt.Sprintf(
			`
			-- add an account and an ACL to allow that account to read and write
			WITH new_account AS (
					INSERT INTO auth.accounts (grafana_org_id, username, password_hash, is_human)
					VALUES ($1, $2, %s, false)
					RETURNING id
			)
			INSERT INTO auth.mqtt_acls (account_id, topic, access_level)
			SELECT
					id,
					'formulatel/+/' || $1 || '/#',
					1
			FROM new_account;
			`,
			pq.QuoteLiteral(string(mustHash(pgRolePassword))),
		),
		createdOrgID,
		tenantRole,
	)
	if err != nil {
		return fmt.Errorf("failed to register account and MQTT ACL: %w", err)
	}

	// create an "account" for the telemetry stream
	userName := request.Username
	user, err := t.CreateUser(ctx, CreateUserRequest{
		Username:    userName,
		TenantID:    int(createdOrgID),
		Transaction: tx,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed creating user", "error", err)
		return err
	}

	// If this succeeds, tx.Rollback() becomes a safe no-op.
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit database transaction: %w", err)
	}

	slog.InfoContext(
		ctx,
		"✅ Created tenant",
		"org_id",
		createdOrgID,
		"org_name",
		request.Name,
		"db_role",
		tenantRole,
		"db_password",
		pgRolePassword,
		"mqtt_user",
		userName,
		"token",
		user.Token,
	)
	return nil
}

// CreateUser adds a row to `auth.accounts` and a corresponding read/write ACL to `auth.mqtt_acls`
func (t *TenantManager) CreateUser(ctx context.Context, request CreateUserRequest) (*CreateUserResponse, error) {
	userToken, err := password.Generate(64, 4, 0, false, true)
	if err != nil {
		return nil, fmt.Errorf("failed generating password :( - %w", err)
	}
	userToken = "ftel-" + userToken
	slog.InfoContext(ctx, "created password")

	var tx pgx.Tx = request.Transaction
	if tx == nil {
		tx, err = t.DB.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to open database transaction: %w", err)
		}
	}

	_, err = tx.Exec(ctx,
		fmt.Sprintf(
			`
			-- add an account and an ACL to allow that account to read and write
			WITH new_account AS (
					INSERT INTO auth.accounts (grafana_org_id, username, password_hash, is_human)
					VALUES ($1, $2, %s, true)
					RETURNING id
			),
			new_user AS (
				INSERT INTO telemetry.users (id, tenant_id, username)
					SELECT id, $1, $2 FROM new_account
			)
			INSERT INTO auth.mqtt_acls (account_id, topic, access_level)
			SELECT
					id,
					'formulatel/+/' || $1 || '/' || $2 || '/#',
					3
			FROM new_account;
			`,
			pq.QuoteLiteral(string(mustHash(userToken))),
		),
		request.TenantID,
		request.Username,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register account and MQTT ACL: %w", err)
	}
	// if the user didn't pass in a transaction then commit immediately
	if request.Transaction == nil {
		err = tx.Commit(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed creating user account and ACL %w", err)
		}
		return &CreateUserResponse{
			Token: userToken,
		}, nil
	}

	return &CreateUserResponse{
		Token: userToken,
	}, nil
}

func (t *TenantManager) CreateDashboard(ctx context.Context, request CreateDashboardRequest) (*CreateDashboardResponse, error) {
	rawJSON, err := os.ReadFile(request.DashboardFile)
	if err != nil {
		return nil, fmt.Errorf("failed reading dashboard JSON %w:", err)
	}

	var dashboardMap map[string]any
	if err := json.Unmarshal(rawJSON, &dashboardMap); err != nil {
		return nil, fmt.Errorf("invalid dashboard JSON %w:", err)
	}

	params := dashboards.NewPostDashboardParamsWithContext(context.Background()).
		WithBody(&models.SaveDashboardCommand{
			Dashboard: dashboardMap["dashboard"],
			Overwrite: true,
		})

	resp, err := t.Grafana.WithOrgID(request.TenantID).Dashboards.PostDashboardWithParams(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create dashboard: %w", err)
	}
	if resp.IsSuccess() {
		slog.InfoContext(ctx, "created dashboard",
			"url", *(resp.Payload.URL),
			"title", resp.Payload.Title,
			"org_id", request.TenantID,
		)
		return &CreateDashboardResponse{
			ID: *resp.Payload.ID,
		}, err
	}
	slog.ErrorContext(ctx, "failed to create dashboard", "code", resp.Code(), "response", resp.String())
	return nil, fmt.Errorf("failed to create dashboard")

}

func NewTenantManager(grafanaOpts *grafanasdk.TransportConfig, dbConnString string) (*TenantManager, error) {
	conn, err := pgx.Connect(context.Background(), dbConnString)
	if err != nil {
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	return &TenantManager{
		DB:      conn,
		Grafana: grafanasdk.NewHTTPClientWithConfig(strfmt.Default, grafanaOpts),
	}, nil
}

func mustHash(s string) []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte(s), 10)
	if err != nil {
		panic(fmt.Errorf("failed hashing password: %w", err))
	}

	return hash
}

// CLI commands

func CreateTenant() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "Provision a new racing team tenant",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "The display name of the racing team",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "slug",
				Aliases:  []string{"s"},
				Usage:    "URL-safe identifier for the team (e.g., apex)",
				Required: true,
			},
			// TODO: maybe we shouldn't make a user here?
			&cli.StringFlag{
				Name:     "username",
				Aliases:  []string{"user", "u"},
				Usage:    "name of the root user for the tenant's stream",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "url",
				Usage:    "host:port of the postgres instance",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "database",
				Aliases:  []string{"d"},
				Usage:    "Database name",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "mqtt-uri",
				Aliases:  []string{"m"},
				Usage:    "Database name",
				Required: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			t, ok := ctx.Value(tenantManagerContext).(*TenantManager)
			if !ok {
				return fmt.Errorf("no manager setup")
			}
			name := cmd.String("name")
			slug := cmd.String("slug")
			url := cmd.String("url")
			database := cmd.String("database")
			username := cmd.String("username")
			mqttURI := cmd.String("mqtt-uri")

			slog.Info("creating tenant", "name", name, "slug", slug)

			return t.CreateOrg(ctx, CreateOrgRequest{
				Name:     name,
				Slug:     slug,
				Username: username,
				URL:      url,
				Database: database,
				MQTTURI:  mqttURI,
			})

		},
	}
}

func CreateUser() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "add a user to a tenant",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:     "tenant",
				Aliases:  []string{"tid", "t"},
				Usage:    "The tenant ID to add the user to",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "username",
				Aliases:  []string{"user", "u"},
				Required: true,
				Usage:    "name of the root user for the tenant's stream",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			t, ok := ctx.Value(tenantManagerContext).(*TenantManager)
			if !ok {
				return fmt.Errorf("no manager setup")
			}
			tenant := cmd.Int("tenant")
			username := cmd.String("username")

			slog.Info("creating user", "name", tenant, "username", username)

			user, err := t.CreateUser(ctx, CreateUserRequest{
				Username: username,
				TenantID: tenant,
			})

			if err != nil {
				slog.ErrorContext(ctx, "⚠️ failed creating user", "error", err)
				return err
			}
			slog.InfoContext(ctx, "✅ created user", "username", username, "token", user.Token)
			return nil
		},
	}
}

func CreateDashboard() *cli.Command {
	return &cli.Command{
		Name: "create",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "dashboard-file",
				Required: true,
			},
			&cli.IntFlag{
				Name:     "tenant",
				Aliases:  []string{"tid", "t", "tenant-id"},
				Usage:    "The tenant ID to create the dashboard for",
				Required: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			t, ok := ctx.Value(tenantManagerContext).(*TenantManager)
			if !ok {
				return fmt.Errorf("no manager setup")
			}
			tenantID := cmd.Int("tenant")
			dashboardFile := cmd.String("dashboard-file")

			r, err := t.CreateDashboard(ctx, CreateDashboardRequest{
				TenantID:      int64(tenantID),
				DashboardFile: dashboardFile,
			})

			if err != nil {
				slog.ErrorContext(ctx, "⚠️ failed creating dashboard", "error", err)
				return err
			}

			slog.InfoContext(ctx, "✅ created dashboard", "id", r.ID)

			return nil
		},
	}
}
