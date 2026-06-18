package timescale

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Isnor/testtable"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const migrationsPath = "file://../../../migrations/"
const testDBName = "postgres"
const testDBUser = "postgres"
const testDBPassword = "postgres"
const pgImage = "timescale/timescaledb:latest-pg18"

// mustPostgresContainer is a singleton that is instantiated by the first test that
// uses a postgres container and reused by all other tests that require postgres.
var mustPostgresContainer = sync.OnceValue(func() *postgres.PostgresContainer {
	ctx := context.Background()
	postgresContainer, err := postgres.Run(ctx,
		pgImage,
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

// WithMigrationsTest is a test that uses a postgres instance that has some or all migrations
// applied to it.
type WithMigrationsTest struct {
	Name string
	// which migration number to migrate up to
	MigrationNum uint
	Expectations func(*testing.T, *pgxpool.Pool, error)
}

// Run orchestrates a test that uses a set of database migrations.
//  1. Connects to the test postgres container - halts if there is an error
//  2. Creates a database to run migrations against that will be used for the test - halts if there is an error
//  3. Runs some or all of the migrations - halts if there is an error
//  4. Runs the expectations / assertions
//  5. Runs the down migrations - halts if there is an error
func (mt *WithMigrationsTest) Run(t *testing.T) {

	t.Run(mt.Name, func(t *testing.T) {
		t.Parallel()
		pgContainer := mustPostgresContainer()

		// create a test database for this test to have a "sandbox" to run in
		connPool, err := pgxpool.New(t.Context(), pgContainer.MustConnectionString(t.Context(), "sslmode=disable"))
		require.NoError(t, err, "could not connect to postgres container %v", err)
		dbName := strings.ReplaceAll(strings.ToLower(mt.Name), " ", "_")
		_, err = connPool.Exec(t.Context(), fmt.Sprintf("CREATE DATABASE %s", dbName))
		require.NoError(t, err, "could not create new database")

		port, err := pgContainer.MappedPort(t.Context(), "5432")
		require.NoError(t, err)
		connectionString := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", testDBUser, testDBPassword, port.Port(), dbName)
		testDB, err := pgxpool.New(t.Context(), connectionString)
		require.NoError(t, err, "could not connect to new DB %s", dbName)

		migrations, err := migrate.New(migrationsPath, connectionString)
		require.NoError(t, err)
		defer migrations.Close()

		if mt.MigrationNum == 0 {
			require.NoError(t, migrations.Up())
		} else {
			require.NoError(t, migrations.Migrate(mt.MigrationNum))
		}
		// now we can run the assertions
		mt.Expectations(t, testDB, err)
		require.NoError(t, migrations.Down())
	})
}

// TestMain stops the singleton postgres container
func TestMain(m *testing.M) {
	m.Run()
	mustPostgresContainer().Terminate(context.Background(), testcontainers.StopTimeout(time.Second))
}

func pseudoRandomVehicleTelemetry() *pb.GameTelemetry {
	return &pb.GameTelemetry{
		Title:     pb.GameTitle_GAME_TITLE_UNKNOWN,
		SessionId: "testing",
		UserId:    "testuser",
		Timestamp: timestamppb.Now(),
		Data: &pb.GameTelemetry_VehicleData{
			VehicleData: &pb.VehicleData{
				Speed:             rand.Uint32N(400),
				Rpm:               rand.Uint32N(14000),
				Throttle:          rand.Float32(),
				Brake:             rand.Float32(),
				Steering:          rand.Float32(),
				Gear:              rand.Int32N(8),
				EngineTemperature: rand.Uint32N(1000),
				Tires: &pb.VehicleData_Tires{
					FrontLeft:  &pb.TireData{BrakeTemperature: rand.Uint32N(1000), InnerTemperature: rand.Uint32N(1000), SurfaceTemperature: rand.Uint32N(1000), Pressure: rand.Uint32N(1000)},
					FrontRight: &pb.TireData{BrakeTemperature: rand.Uint32N(1000), InnerTemperature: rand.Uint32N(1000), SurfaceTemperature: rand.Uint32N(1000), Pressure: rand.Uint32N(1000)},
					BackLeft:   &pb.TireData{BrakeTemperature: rand.Uint32N(1000), InnerTemperature: rand.Uint32N(1000), SurfaceTemperature: rand.Uint32N(1000), Pressure: rand.Uint32N(1000)},
					BackRight:  &pb.TireData{BrakeTemperature: rand.Uint32N(1000), InnerTemperature: rand.Uint32N(1000), SurfaceTemperature: rand.Uint32N(1000), Pressure: rand.Uint32N(1000)},
				},
			},
		},
	}
}

func pseudoRandomMotionTelemetry() *pb.GameTelemetry {

	return &pb.GameTelemetry{
		Title:     pb.GameTitle_GAME_TITLE_UNKNOWN,
		SessionId: "testing",
		UserId:    "testuser",
		Timestamp: timestamppb.Now(),
		Data: &pb.GameTelemetry_MotionData{
			MotionData: &pb.MotionData{
				PositionX:          rand.Float32() * 500,
				PositionY:          rand.Float32() * 500,
				PositionZ:          rand.Float32() * 500,
				VelocityX:          rand.Float32() * 400,
				VelocityY:          rand.Float32() * 400,
				VelocityZ:          rand.Float32() * 400,
				GForceLateral:      rand.Float32() * 10,
				GForceLongitudinal: rand.Float32() * 10,
				GForceVertical:     rand.Float32() * 10,
				Yaw:                rand.Float32(),
				Pitch:              rand.Float32(),
				Roll:               rand.Float32(),
			},
		},
	}

}

func pseudoRandomLapTelemetry() *pb.GameTelemetry {
	sector1Valid, sector2Valid, sector3Valid := rand.IntN(2) == 0, rand.IntN(2) == 0, rand.IntN(2) == 0
	lapValid := sector1Valid && sector2Valid && sector3Valid
	lapTimeMillis := rand.Uint32N(180000)
	return &pb.GameTelemetry{
		Title:     pb.GameTitle_GAME_TITLE_F123,
		SessionId: "testing",
		UserId:    "testuser",
		Timestamp: timestamppb.Now(),
		Data: &pb.GameTelemetry_LapTimesData{
			LapTimesData: &pb.HistoricLapData{
				LapNum:       rand.Uint32N(1000000),
				LapTime:      durationpb.New(time.Duration(lapTimeMillis) * time.Millisecond),
				Sector1Time:  durationpb.New(time.Duration(lapTimeMillis/3) * time.Millisecond),
				Sector2Time:  durationpb.New(time.Duration(lapTimeMillis/3) * time.Millisecond),
				Sector3Time:  durationpb.New(time.Duration(lapTimeMillis/3) * time.Millisecond),
				LapValid:     lapValid,
				Sector1Valid: sector1Valid,
				Sector2Valid: sector1Valid,
				Sector3Valid: sector1Valid,
			},
		},
	}
}

func pseudoRandomLiveLapDataTelemetry() *pb.GameTelemetry {
	lapTime := rand.Uint32N(45000) * 3 // 45 seconds
	return &pb.GameTelemetry{
		Title:     pb.GameTitle_GAME_TITLE_UNKNOWN,
		SessionId: "testing",
		UserId:    "testuser",
		Timestamp: timestamppb.Now(),
		Data: &pb.GameTelemetry_CurrentLapData{
			CurrentLapData: &pb.CurrentLapData{
				LapNum:            rand.Uint32N(90),
				LapTime:           lapTime,
				Sector:            rand.Uint32N(3),
				Sector1Time:       lapTime / 3,
				Sector2Time:       lapTime / 3,
				LapDistance:       rand.Float32() * float32(rand.Int32N(1000)),
				DeltaToCarInFront: rand.Uint32N(1000),
				DeltaToRaceLeader: rand.Uint32N(1000),
				TotalDistance:     rand.Float32() * float32(rand.Int32N(100000)),
			},
		},
	}
}

func pseudoRandomMotionExTelemetry() *pb.GameTelemetry {
	return &pb.GameTelemetry{
		Title:     pb.GameTitle_GAME_TITLE_F123,
		SessionId: "motion-ex-test",
		UserId:    "testuser",
		Timestamp: timestamppb.Now(),
		Data: &pb.GameTelemetry_WheelData{
			WheelData: &pb.ExtendedFourWheelData{
				BackLeft: &pb.ExtendedWheelData{
					WheelSpeed:         50.0,
					VerticalForce:      4000.0,
					SlipAngle:          0.05,
					SlipRatio:          0.15,
					LateralForce:       200.0,
					LongitudinalForce:  150.0,
					SuspensionPosition: 0.1,
					SuspensionVelocity: -0.01,
				},
				BackRight: &pb.ExtendedWheelData{
					WheelSpeed:         48.0,
					VerticalForce:      4200.0,
					SlipAngle:          0.03,
					SlipRatio:          0.12,
					LateralForce:       180.0,
					LongitudinalForce:  120.0,
					SuspensionPosition: 0.15,
					SuspensionVelocity: 0.02,
				},
				FrontLeft: &pb.ExtendedWheelData{
					WheelSpeed:         52.0,
					VerticalForce:      3800.0,
					SlipAngle:          0.10,
					SlipRatio:          0.08,
					LateralForce:       250.0,
					LongitudinalForce:  80.0,
					SuspensionPosition: 0.08,
					SuspensionVelocity: -0.03,
				},
				FrontRight: &pb.ExtendedWheelData{
					WheelSpeed:         51.0,
					VerticalForce:      4100.0,
					SlipAngle:          0.02,
					SlipRatio:          0.05,
					LateralForce:       220.0,
					LongitudinalForce:  100.0,
					SuspensionPosition: 0.12,
					SuspensionVelocity: 0.01,
				},
			},
		},
	}
}

func TestSimpleDBWrites(t *testing.T) {
	t.Parallel()
	testtable.TestTable{
		&WithMigrationsTest{
			Name:         "migrate up and down",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {},
		},
		&WithMigrationsTest{
			Name: "insert vehicle data",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				// Create a batcher for vehicle_data
				msgChan := make(chan GameTelemetryWithContext, 10)
				batcher := NewTableBatcher(t.Context(), p, "vehicle_data", msgChan, 5, 50*time.Millisecond)
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
				batcher.msgChan <- GameTelemetryWithContext{ctx: t.Context(), msg: msg}

				// Wait for flush
				time.Sleep(100 * time.Millisecond)
				close(msgChan)

				// Verify data was written
				var speed int32
				err = p.QueryRow(t.Context(),
					"SELECT speed FROM vehicle_data WHERE session_id = $1 AND user_id = $2",
					"vehicle-test", "test-user").Scan(&speed)
				require.NoError(t, err)
				assert.EqualValues(t, 150, speed)
			},
		},
		&WithMigrationsTest{
			Name: "insert motion data",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {
				// Create a batcher for motion_data
				msgChan := make(chan GameTelemetryWithContext, 10)
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
				batcher.msgChan <- GameTelemetryWithContext{ctx: t.Context(), msg: msg}

				// Wait for flush
				time.Sleep(100 * time.Millisecond)
				close(msgChan)

				// Verify data was written
				var posX float32
				err = p.QueryRow(t.Context(),
					"SELECT position_x FROM motion_data WHERE session_id = $1",
					"motion-test").Scan(&posX)
				require.NoError(t, err)
				assert.EqualValues(t, 100.5, posX)
			},
		},
		&WithMigrationsTest{
			Name: "insert extended wheel data",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {
				// Create a batcher for extended_wheel_data
				msgChan := make(chan GameTelemetryWithContext, 10)
				batcher := NewTableBatcher(t.Context(), p, "extended_wheel_data", msgChan, 5, 50*time.Millisecond)
				batcher.Start()

				// Create test telemetry message with motion ex (extended wheel) data
				msg := pseudoRandomMotionExTelemetry()

				// Send message directly to the batcher channel
				batcher.msgChan <- GameTelemetryWithContext{ctx: t.Context(), msg: msg}

				// Wait for flush
				time.Sleep(100 * time.Millisecond)
				close(msgChan)

				// Verify data was written
				var wheelSpeed float32
				err = p.QueryRow(t.Context(),
					"SELECT fl_wheel_speed FROM extended_wheel_data WHERE session_id = $1",
					"motion-ex-test").Scan(&wheelSpeed)
				require.NoError(t, err)
				assert.EqualValues(t, 52.0, wheelSpeed)

				// Verify other wheel data
				var blSpeed, brSpeed, frSpeed float32
				err = p.QueryRow(t.Context(),
					"SELECT bl_wheel_speed, br_wheel_speed, fr_wheel_speed FROM extended_wheel_data WHERE session_id = $1",
					"motion-ex-test").Scan(&blSpeed, &brSpeed, &frSpeed)
				require.NoError(t, err)
				assert.EqualValues(t, 50.0, blSpeed)
				assert.EqualValues(t, 48.0, brSpeed)
				assert.EqualValues(t, 51.0, frSpeed)
			},
		},
	}.Run(t)
}

func TestBatchRouter(t *testing.T) {
	t.Parallel()
	testtable.TestTable{
		&WithMigrationsTest{
			Name: "batching router",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				msgChan := make(chan *pb.GameTelemetry)
				router, err := NewBatchRouter(t.Context(), p, msgChan, 1, 10*time.Millisecond)

				// create a motion telemetry and vehicle telemetry
				motionTelemetryWritten := pseudoRandomMotionTelemetry()
				vehicleTelemetryWritten := pseudoRandomVehicleTelemetry()
				lapDataTelemetryWritten := pseudoRandomLapTelemetry()
				currentLapDataTelemetryWritten := pseudoRandomLiveLapDataTelemetry()

				// since the batch size is 1, the router should write these immediately
				router.Add(t.Context(), motionTelemetryWritten)
				router.Add(t.Context(), vehicleTelemetryWritten)
				router.Add(t.Context(), lapDataTelemetryWritten)
				router.Add(t.Context(), currentLapDataTelemetryWritten)

				// Wait for the batcher to complete the flush. The batcher uses a ticker-based flush
				// with a 10ms interval, so we wait for at least one flush cycle plus some buffer.
				time.Sleep(500 * time.Millisecond)

				motionTelemetryRead := &pb.MotionData{}
				vehicleTelemetryRead := &pb.VehicleData{}
				lapTelemetryRead := &pb.HistoricLapData{}
				currentLapTelemetryRead := &pb.CurrentLapData{}
				p.QueryRow(t.Context(), "select position_z from motion_data limit 1").Scan(&motionTelemetryRead.PositionZ)
				p.QueryRow(t.Context(), "select speed, rpm from vehicle_data limit 1").Scan(&vehicleTelemetryRead.Speed, &vehicleTelemetryRead.Rpm)
				p.QueryRow(t.Context(), "select lap_num from session_lap_data").Scan(&lapTelemetryRead.LapNum)
				p.QueryRow(t.Context(), "select current_lap_time, sector1_time from live_lap_data").Scan(&currentLapTelemetryRead.LapTime, &currentLapTelemetryRead.Sector1Time)

				assert.EqualValues(t, motionTelemetryWritten.GetMotionData().GetPositionZ(), motionTelemetryRead.GetPositionZ())

				assert.EqualValues(t, vehicleTelemetryWritten.GetVehicleData().GetSpeed(), vehicleTelemetryRead.GetSpeed())
				assert.EqualValues(t, vehicleTelemetryWritten.GetVehicleData().GetRpm(), vehicleTelemetryRead.GetRpm())

				assert.EqualValues(t, lapDataTelemetryWritten.GetLapTimesData().GetLapNum(), lapTelemetryRead.GetLapNum())

				assert.EqualValues(t, currentLapDataTelemetryWritten.GetCurrentLapData().GetLapTime(), currentLapTelemetryRead.GetLapTime())
				assert.EqualValues(t, currentLapDataTelemetryWritten.GetCurrentLapData().GetSector1Time(), currentLapTelemetryRead.GetSector1Time())
			},
		},
	}.Run(t)
}

func TestDuplicateLapTimes(t *testing.T) {
	t.Parallel()
	testtable.TestTable{
		&WithMigrationsTest{
			Name: "duplicate_session_lap_data",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {
				msgChan := make(chan GameTelemetryWithContext, 10)
				batcher := NewTableBatcher(t.Context(), p, "session_lap_data", msgChan, 5, 10*time.Millisecond)
				batcher.Start()

				lapDataTelemetryWritten := &pb.GameTelemetry{
					Title:     pb.GameTitle_GAME_TITLE_F123,
					SessionId: "testing",
					UserId:    "testuser",
					Timestamp: timestamppb.Now(),
					Data: &pb.GameTelemetry_LapTimesData{
						LapTimesData: &pb.HistoricLapData{
							LapNum:       1,
							LapTime:      durationpb.New(2 * time.Minute),
							Sector1Time:  durationpb.New(30 * time.Second),
							Sector2Time:  durationpb.New(time.Minute),
							Sector3Time:  durationpb.New(30 * time.Second),
							LapValid:     true,
							Sector1Valid: true,
							Sector2Valid: true,
							Sector3Valid: true,
						},
					},
				}
				for range 3 {
					assert.NoError(t, batcher.WriteLapRow(t.Context(), lapDataTelemetryWritten), "should not get error when inserting duplicate row")
				}
				rows, err := p.Query(t.Context(), "select lap_num, lap_time from session_lap_data")
				assert.NoError(t, err, "did not get rows from session_lap_data")
				lapTelemetryRead, err := pgx.CollectExactlyOneRow(rows, func(row pgx.CollectableRow) (*pb.HistoricLapData, error) {
					result := pb.HistoricLapData{}
					var lapTime time.Duration
					err := row.Scan(&result.LapNum, &lapTime)
					result.LapTime = durationpb.New(lapTime)
					return &result, err
					// this is neat, but the struct fields need to match the column names or have `db` tags, neither
					// of which are true for our protobufs
					// tel, err := pgx.RowToStructByName[pb.HistoricLapData](row)
					// return &tel, err
				})
				require.NoError(t, err, "expected a single lap to have been recorded")
				require.NotNil(t, lapTelemetryRead)
				assert.EqualValues(t, lapDataTelemetryWritten.GetLapTimesData().LapNum, lapTelemetryRead.LapNum)
				assert.EqualValues(t, lapDataTelemetryWritten.GetLapTimesData().LapTime, lapTelemetryRead.LapTime)
			},
		},
	}.Run(t)
}

// TestBatchRouterExtendedWheelData tests that extended wheel data is properly routed and written
func TestBatchRouterExtendedWheelData(t *testing.T) {
	t.Parallel()
	testtable.TestTable{
		&WithMigrationsTest{
			Name: "extended wheel data routing",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {
				msgChan := make(chan *pb.GameTelemetry)
				router, err := NewBatchRouter(t.Context(), p, msgChan, 1, 10*time.Millisecond)
				require.NoError(t, err)

				// Create motion ex telemetry with known values
				motionExTelemetry := &pb.GameTelemetry{
					Title:     pb.GameTitle_GAME_TITLE_F123,
					SessionId: "motion-ex-router-test",
					UserId:    "testuser",
					Timestamp: timestamppb.Now(),
					Data: &pb.GameTelemetry_WheelData{
						WheelData: &pb.ExtendedFourWheelData{
							BackLeft: &pb.ExtendedWheelData{
								WheelSpeed:         50.0,
								VerticalForce:      4000.0,
								SlipAngle:          0.05,
								SlipRatio:          0.15,
								LateralForce:       200.0,
								LongitudinalForce:  150.0,
								SuspensionPosition: 0.1,
								SuspensionVelocity: -0.01,
							},
							BackRight: &pb.ExtendedWheelData{
								WheelSpeed:         48.0,
								VerticalForce:      4200.0,
								SlipAngle:          0.03,
								SlipRatio:          0.12,
								LateralForce:       180.0,
								LongitudinalForce:  120.0,
								SuspensionPosition: 0.15,
								SuspensionVelocity: 0.02,
							},
							FrontLeft: &pb.ExtendedWheelData{
								WheelSpeed:         52.0,
								VerticalForce:      3800.0,
								SlipAngle:          0.10,
								SlipRatio:          0.08,
								LateralForce:       250.0,
								LongitudinalForce:  80.0,
								SuspensionPosition: 0.08,
								SuspensionVelocity: -0.03,
							},
							FrontRight: &pb.ExtendedWheelData{
								WheelSpeed:         51.0,
								VerticalForce:      4100.0,
								SlipAngle:          0.02,
								SlipRatio:          0.05,
								LateralForce:       220.0,
								LongitudinalForce:  100.0,
								SuspensionPosition: 0.12,
								SuspensionVelocity: 0.01,
							},
						},
					},
				}

				// Route the message
				router.Add(t.Context(), motionExTelemetry)

				// Wait for the batcher to complete the flush
				time.Sleep(100 * time.Millisecond)

				// Verify data was written to extended_wheel_data table
				extendedWheelData := &pb.ExtendedFourWheelData{
					BackLeft:   &pb.ExtendedWheelData{},
					BackRight:  &pb.ExtendedWheelData{},
					FrontLeft:  &pb.ExtendedWheelData{},
					FrontRight: &pb.ExtendedWheelData{},
				}
				err = p.QueryRow(t.Context(), "SELECT fl_wheel_speed FROM extended_wheel_data WHERE session_id = $1", "motion-ex-router-test").Scan(&(extendedWheelData.FrontLeft).WheelSpeed)
				require.NoError(t, err)
				assert.EqualValues(t, float32(52.0), extendedWheelData.FrontLeft.WheelSpeed)

				// Verify all four wheels
				var blSpeed, brSpeed, frSpeed float32
				err = p.QueryRow(t.Context(), "SELECT bl_wheel_speed, br_wheel_speed, fr_wheel_speed FROM extended_wheel_data WHERE session_id = $1", "motion-ex-router-test").Scan(&blSpeed, &brSpeed, &frSpeed)
				require.NoError(t, err)
				assert.EqualValues(t, float32(50.0), blSpeed)
				assert.EqualValues(t, float32(48.0), brSpeed)
				assert.EqualValues(t, float32(51.0), frSpeed)

				// Verify suspension data
				var blSuspensionPos, frSuspensionPos float32
				err = p.QueryRow(t.Context(), "SELECT bl_suspension_position, fr_suspension_position FROM extended_wheel_data WHERE session_id = $1", "motion-ex-router-test").Scan(&blSuspensionPos, &frSuspensionPos)
				require.NoError(t, err)
				assert.EqualValues(t, float32(0.1), blSuspensionPos)
				assert.EqualValues(t, float32(0.12), frSuspensionPos)

				// Verify slip angles
				var blSlipAngle, frSlipAngle float32
				err = p.QueryRow(t.Context(), "SELECT bl_slip_angle, fr_slip_angle FROM extended_wheel_data WHERE session_id = $1", "motion-ex-router-test").Scan(&blSlipAngle, &frSlipAngle)
				require.NoError(t, err)
				assert.EqualValues(t, float32(0.05), blSlipAngle)
				assert.EqualValues(t, float32(0.02), frSlipAngle)
			},
		},
	}.Run(t)
}
