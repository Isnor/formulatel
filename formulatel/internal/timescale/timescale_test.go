package timescale

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Isnor/testtable"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const migrationsPath = "file://../../../migrations/"
const testDBName = "postgres"
const testDBUser = "postgres"
const testDBPassword = "postgres"

var mustPostgresContainer = sync.OnceValue(func() *postgres.PostgresContainer {
	ctx := context.Background()
	postgresContainer, err := postgres.Run(ctx,
		"timescale/timescaledb:latest-pg18",
		postgres.WithDatabase(testDBName),
		postgres.WithUsername(testDBUser),
		postgres.WithPassword(testDBPassword),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		log.Fatal("could not start postgres container")
	}
	return postgresContainer
})

type DBMigrateTest struct {
	Name         string
	Expectations func(*testing.T, *pgxpool.Pool, error)
}

func (dbt *DBMigrateTest) Run(t *testing.T) {

	t.Run(dbt.Name, func(t *testing.T) {
		t.Parallel()
		pgContainer := mustPostgresContainer()

		// create a test database for this test to have a "sandbox" to run in
		connPool, err := pgxpool.New(t.Context(), pgContainer.MustConnectionString(t.Context(), "sslmode=disable"))
		require.NoError(t, err, "could not connect to postgres container %v", err)
		dbName := strings.ReplaceAll(strings.ToLower(dbt.Name), " ", "_")
		_, err = connPool.Exec(t.Context(), fmt.Sprintf("CREATE DATABASE %s", dbName))
		require.NoError(t, err, "could not create new database")

		// This is just getting the connection string to the newly created database so we can run migrations per-test.
		// Some tests might want to run N migrations instead of all of them, some may want to run the `down` migrations, etc.
		port, err := pgContainer.MappedPort(t.Context(), "5432")
		require.NoError(t, err)
		connectionString := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", testDBUser, testDBPassword, port.Port(), dbName)
		testDB, err := pgxpool.New(t.Context(), connectionString)
		require.NoError(t, err, "could not connect to new DB %s", dbName)

		// now we can run the assertions
		dbt.Expectations(t, testDB, err)
	})

}

func TestMain(m *testing.M) {
	m.Run()
	mustPostgresContainer().Terminate(context.Background(), testcontainers.StopTimeout(time.Second))
}

func TestSimpleDBWrites(t *testing.T) {
	testtable.TestTable{
		&DBMigrateTest{
			Name: "migrate up and down",
			Expectations: func(t *testing.T, c *pgxpool.Pool, err error) {

				connStr := c.Config().ConnString()

				migrations, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrations.Close()

				assert.NoError(t, migrations.Up())
				assert.NoError(t, migrations.Down())
			},
		},
		&DBMigrateTest{
			Name: "insert vehicle data",
			Expectations: func(t *testing.T, c *pgxpool.Pool, err error) {

				connStr := c.Config().ConnString()

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()

				assert.NoError(t, migrator.Up())
				// Create a batcher for vehicle_data
				msgChan := make(chan *pb.GameTelemetry, 10)
				batcher := NewTableBatcher(t.Context(), c, "vehicle_data", msgChan, 5, 50*time.Millisecond)
				batcher.Start()

				// Create test telemetry message with vehicle data
				msg := &pb.GameTelemetry{
					Title:     pb.GameTitle_GAME_TITLE_F123,
					SessionId: "vehicle-test",
					UserId:    "test-user",
					Timestamp: timestamppb.New(time.Now()),
					Data: &pb.GameTelemetry_VehicleData{
						VehicleData: &pb.VehicleData{
							Speed:             150,
							Rpm:               8000,
							Throttle:          0.8,
							Brake:             0.2,
							Steering:          0.5,
							Gear:              5,
							EngineTemperature: 90,
							Tires: &pb.VehicleData_Tires{
								FrontLeft:  &pb.TireData{BrakeTemperature: 200, InnerTemperature: 60, SurfaceTemperature: 150, Pressure: 28},
								FrontRight: &pb.TireData{BrakeTemperature: 210, InnerTemperature: 62, SurfaceTemperature: 155, Pressure: 29},
								BackLeft:   &pb.TireData{BrakeTemperature: 180, InnerTemperature: 58, SurfaceTemperature: 145, Pressure: 27},
								BackRight:  &pb.TireData{BrakeTemperature: 190, InnerTemperature: 61, SurfaceTemperature: 152, Pressure: 28},
							},
						},
					},
				}

				// Send message directly to the batcher channel
				batcher.msgChan <- msg

				// Wait for flush
				time.Sleep(100 * time.Millisecond)
				close(msgChan)

				// Verify data was written
				var speed int32
				err = c.QueryRow(t.Context(),
					"SELECT speed FROM vehicle_data WHERE session_id = $1 AND user_id = $2",
					"vehicle-test", "test-user").Scan(&speed)
				require.NoError(t, err)
				require.Equal(t, int32(150), speed)
			},
		},
		&DBMigrateTest{
			Name: "insert motion data",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				connStr := p.Config().ConnString()

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()

				assert.NoError(t, migrator.Up())
				// Create a batcher for motion_data
				msgChan := make(chan *pb.GameTelemetry, 10)
				batcher := NewTableBatcher(t.Context(), p, "motion_data", msgChan, 5, 50*time.Millisecond)
				batcher.Start()

				// Create test telemetry message with motion data
				msg := &pb.GameTelemetry{
					Title:     pb.GameTitle_GAME_TITLE_F123,
					SessionId: "motion-test",
					UserId:    "test-user",
					Timestamp: timestamppb.New(time.Now()),
					Data: &pb.GameTelemetry_MotionData{
						MotionData: &pb.MotionData{
							PositionX:          100.5,
							PositionY:          200.5,
							PositionZ:          10.0,
							VelocityX:          50.0,
							VelocityY:          30.0,
							VelocityZ:          0.0,
							GForceLateral:      1.5,
							GForceLongitudinal: 2.0,
							GForceVertical:     1.0,
							Yaw:                0.5,
							Pitch:              0.1,
							Roll:               0.05,
						},
					},
				}

				// Send message directly to the batcher channel
				batcher.msgChan <- msg

				// Wait for flush
				time.Sleep(100 * time.Millisecond)
				close(msgChan)

				// Verify data was written
				var posX float32
				err = p.QueryRow(t.Context(),
					"SELECT position_x FROM motion_data WHERE session_id = $1",
					"motion-test").Scan(&posX)
				require.NoError(t, err)
				require.EqualValues(t, 100.5, posX)
			},
		},
	}.Run(t)

}
