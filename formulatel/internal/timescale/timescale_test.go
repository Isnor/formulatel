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

// These structs use `db` tags so pgx can scan into the struct directly. Their fields should
// match the schema of the table they are used to scan from, e.g. MotionDataReader has a field
// for every column in the motion_data table of our migrations.
type MotionDataReader struct {
	Time               time.Time    `db:"time"`
	SessionID          string       `db:"session_id"`
	UserId             string       `db:"user_id"`
	Title              pb.GameTitle `db:"title"`
	PositionX          float32      `db:"position_x"`
	PositionY          float32      `db:"position_y"`
	PositionZ          float32      `db:"position_z"`
	VelocityX          float32      `db:"velocity_x"`
	VelocityY          float32      `db:"velocity_y"`
	VelocityZ          float32      `db:"velocity_z"`
	GForceLateral      float32      `db:"gforce_lateral"`
	GForceLongitudinal float32      `db:"gforce_longitudinal"`
	GForceVertical     float32      `db:"gforce_vertical"`
	Yaw                float32      `db:"yaw"`
	Pitch              float32      `db:"pitch"`
	Roll               float32      `db:"roll"`
}

type ExtendedWheelDataReader struct {
	Time                 time.Time    `db:"time"`
	SessionID            string       `db:"session_id"`
	UserId               string       `db:"user_id"`
	Title                pb.GameTitle `db:"title"`
	BlWheelSpeed         float32      `db:"bl_wheel_speed"`
	BlVerticalForce      float32      `db:"bl_vertical_force"`
	BlSlipAngle          float32      `db:"bl_slip_angle"`
	BlSlipRatio          float32      `db:"bl_slip_ratio"`
	BlLateralForce       float32      `db:"bl_lateral_force"`
	BlLongitudinalForce  float32      `db:"bl_longitudinal_force"`
	BlSuspensionPosition float32      `db:"bl_suspension_position"`
	BlSuspensionVelocity float32      `db:"bl_suspension_velocity"`
	BrWheelSpeed         float32      `db:"br_wheel_speed"`
	BrVerticalForce      float32      `db:"br_vertical_force"`
	BrSlipAngle          float32      `db:"br_slip_angle"`
	BrSlipRatio          float32      `db:"br_slip_ratio"`
	BrLateralForce       float32      `db:"br_lateral_force"`
	BrLongitudinalForce  float32      `db:"br_longitudinal_force"`
	BrSuspensionPosition float32      `db:"br_suspension_position"`
	BrSuspensionVelocity float32      `db:"br_suspension_velocity"`
	FlWheelSpeed         float32      `db:"fl_wheel_speed"`
	FlVerticalForce      float32      `db:"fl_vertical_force"`
	FlSlipAngle          float32      `db:"fl_slip_angle"`
	FlSlipRatio          float32      `db:"fl_slip_ratio"`
	FlLateralForce       float32      `db:"fl_lateral_force"`
	FlLongitudinalForce  float32      `db:"fl_longitudinal_force"`
	FlSuspensionPosition float32      `db:"fl_suspension_position"`
	FlSuspensionVelocity float32      `db:"fl_suspension_velocity"`
	FrWheelSpeed         float32      `db:"fr_wheel_speed"`
	FrVerticalForce      float32      `db:"fr_vertical_force"`
	FrSlipAngle          float32      `db:"fr_slip_angle"`
	FrSlipRatio          float32      `db:"fr_slip_ratio"`
	FrLateralForce       float32      `db:"fr_lateral_force"`
	FrLongitudinalForce  float32      `db:"fr_longitudinal_force"`
	FrSuspensionPosition float32      `db:"fr_suspension_position"`
	FrSuspensionVelocity float32      `db:"fr_suspension_velocity"`
}

type VehicleDataReader struct {
	Time              time.Time    `db:"time"`
	SessionID         string       `db:"session_id"`
	UserId            string       `db:"user_id"`
	Title             pb.GameTitle `db:"title"`
	Speed             uint32       `db:"speed"`
	Rpm               uint32       `db:"rpm"`
	Throttle          float32      `db:"throttle"`
	Brake             float32      `db:"brake"`
	Steering          float32      `db:"steering"`
	Gear              int32        `db:"gear"`
	EngineTemperature uint32       `db:"engine_temperature"`
	FlBrakeTemp       uint32       `db:"fl_brake_temp"`
	FlInnerTemp       uint32       `db:"fl_inner_temp"`
	FlSurfaceTemp     uint32       `db:"fl_surface_temp"`
	FlPressure        uint32       `db:"fl_pressure"`
	FrBrakeTemp       uint32       `db:"fr_brake_temp"`
	FrInnerTemp       uint32       `db:"fr_inner_temp"`
	FrSurfaceTemp     uint32       `db:"fr_surface_temp"`
	FrPressure        uint32       `db:"fr_pressure"`
	BlBrakeTemp       uint32       `db:"bl_brake_temp"`
	BlInnerTemp       uint32       `db:"bl_inner_temp"`
	BlSurfaceTemp     uint32       `db:"bl_surface_temp"`
	BlPressure        uint32       `db:"bl_pressure"`
	BrBrakeTemp       uint32       `db:"br_brake_temp"`
	BrInnerTemp       uint32       `db:"br_inner_temp"`
	BrSurfaceTemp     uint32       `db:"br_surface_temp"`
	BrPressure        uint32       `db:"br_pressure"`
}

type LiveLapDataReader struct {
	Time              time.Time    `db:"time"`
	SessionID         string       `db:"session_id"`
	UserId            string       `db:"user_id"`
	Title             pb.GameTitle `db:"title"`
	LapNum            uint32       `db:"lap_num"`
	CurrentLapTime    uint32       `db:"current_lap_time"`
	Sector            uint32       `db:"sector"`
	Sector1Time       uint32       `db:"sector1_time"`
	Sector2Time       uint32       `db:"sector2_time"`
	DeltaToCarInFront uint32       `db:"delta_to_car_in_front"`
	DeltaToRaceLeader uint32       `db:"delta_to_race_leader"`
	LapDistance       float32      `db:"lap_distance"`
	TotalDistance     float32      `db:"total_distance"`
}

type SessionLapDataReader struct {
	SessionID    string        `db:"session_id"`
	UserID       string        `db:"user_id"`
	Title        pb.GameTitle  `db:"title"`
	LapNum       uint32        `db:"lap_num"`
	LapTime      time.Duration `db:"lap_time"`
	Sector1Time  time.Duration `db:"sector1_time"`
	Sector2Time  time.Duration `db:"sector2_time"`
	Sector3Time  time.Duration `db:"sector3_time"`
	LapValid     bool          `db:"lap_valid"`
	Sector1Valid bool          `db:"sector1_valid"`
	Sector2Valid bool          `db:"sector2_valid"`
	Sector3Valid bool          `db:"sector3_valid"`
}

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
		tid := rand.Uint32N(uint32(time.Now().Nanosecond()))
		// create a test database for this test to have a "sandbox" to run in
		connPool, err := pgxpool.New(t.Context(), pgContainer.MustConnectionString(t.Context(), "sslmode=disable"))
		require.NoError(t, err, "could not connect to postgres container %v", err)
		dbName := fmt.Sprintf("%s_%d", strings.ReplaceAll(strings.ToLower(mt.Name), " ", "_"), tid)
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
		testDB.Close()
		_, err = connPool.Exec(t.Context(), fmt.Sprintf("DROP DATABASE %s WITH(FORCE)", dbName))
		require.NoError(t, err)
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

func pseudoRandomExtendedWheelTelemetry() *pb.GameTelemetry {
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
				rows, err := p.Query(t.Context(),
					"SELECT * FROM telemetry.vehicle_data WHERE session_id = $1 AND user_id = $2",
					"vehicle-test", "test-user")
				require.NoError(t, err)
				vehicleData, err := pgx.CollectExactlyOneRow(rows, func(row pgx.CollectableRow) (VehicleDataReader, error) {
					return pgx.RowToStructByName[VehicleDataReader](row)
				})
				require.NoError(t, err)
				assert.EqualValues(t, 150, vehicleData.Speed)
				assert.EqualValues(t, 8000, vehicleData.Rpm)
				assert.EqualValues(t, float32(.8), vehicleData.Throttle)
				assert.EqualValues(t, 200, vehicleData.FlBrakeTemp)
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

				// Verify all motion data columns were written
				rows, err := p.Query(t.Context(),
					`SELECT time, session_id, user_id, title, position_x, position_y, position_z, velocity_x, velocity_y,
						velocity_z, gforce_lateral, gforce_longitudinal, gforce_vertical, yaw, pitch, roll
						FROM telemetry.motion_data WHERE session_id = $1`, "motion-test")
				assert.NoError(t, err)

				motionData, err := pgx.CollectExactlyOneRow(rows, func(row pgx.CollectableRow) (MotionDataReader, error) {
					return pgx.RowToStructByName[MotionDataReader](row)
				})
				require.NoError(t, err)

				assert.EqualValues(t, float32(100.5), motionData.PositionX)
				assert.EqualValues(t, float32(200.5), motionData.PositionY)
				assert.EqualValues(t, float32(10.0), motionData.PositionZ)
				assert.EqualValues(t, float32(50.0), motionData.VelocityX)
				assert.EqualValues(t, float32(30.0), motionData.VelocityY)
				assert.EqualValues(t, float32(0.0), motionData.VelocityZ)
				assert.EqualValues(t, float32(1.5), motionData.GForceLateral)
				assert.EqualValues(t, float32(2.0), motionData.GForceLongitudinal)
				assert.EqualValues(t, float32(1.0), motionData.GForceVertical)
				assert.EqualValues(t, float32(0.5), motionData.Yaw)
				assert.EqualValues(t, float32(0.1), motionData.Pitch)
				assert.EqualValues(t, float32(0.05), motionData.Roll)
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
				msg := pseudoRandomExtendedWheelTelemetry()

				// Send message directly to the batcher channel
				batcher.msgChan <- GameTelemetryWithContext{ctx: t.Context(), msg: msg}

				// Wait for flush
				time.Sleep(100 * time.Millisecond)
				close(msgChan)

				rows, err := p.Query(t.Context(),
					`SELECT time, session_id, user_id, title,
						bl_wheel_speed, bl_vertical_force, bl_slip_angle, bl_slip_ratio,
						bl_lateral_force, bl_longitudinal_force, bl_suspension_position, bl_suspension_velocity,
						br_wheel_speed, br_vertical_force, br_slip_angle, br_slip_ratio,
						br_lateral_force, br_longitudinal_force, br_suspension_position, br_suspension_velocity,
						fl_wheel_speed, fl_vertical_force, fl_slip_angle, fl_slip_ratio,
						fl_lateral_force, fl_longitudinal_force, fl_suspension_position, fl_suspension_velocity,
						fr_wheel_speed, fr_vertical_force, fr_slip_angle, fr_slip_ratio,
						fr_lateral_force, fr_longitudinal_force, fr_suspension_position, fr_suspension_velocity
						FROM telemetry.extended_wheel_data WHERE session_id = $1`,
					"motion-ex-test")
				assert.NoError(t, err)
				assert.True(t, rows.Next())

				wheelData, err := pgx.RowToStructByName[ExtendedWheelDataReader](rows)
				require.NoError(t, err)
				rows.Close()

				// Verify Front Left wheel
				assert.EqualValues(t, float32(52.0), wheelData.FlWheelSpeed)
				assert.EqualValues(t, float32(3800.0), wheelData.FlVerticalForce)
				assert.EqualValues(t, float32(0.10), wheelData.FlSlipAngle)
				assert.EqualValues(t, float32(0.08), wheelData.FlSlipRatio)
				assert.EqualValues(t, float32(250.0), wheelData.FlLateralForce)
				assert.EqualValues(t, float32(80.0), wheelData.FlLongitudinalForce)
				assert.EqualValues(t, float32(0.08), wheelData.FlSuspensionPosition)
				assert.EqualValues(t, float32(-0.03), wheelData.FlSuspensionVelocity)

				// Verify Front Right wheel
				assert.EqualValues(t, float32(51.0), wheelData.FrWheelSpeed)
				assert.EqualValues(t, float32(4100.0), wheelData.FrVerticalForce)
				assert.EqualValues(t, float32(0.02), wheelData.FrSlipAngle)
				assert.EqualValues(t, float32(0.05), wheelData.FrSlipRatio)
				assert.EqualValues(t, float32(220.0), wheelData.FrLateralForce)
				assert.EqualValues(t, float32(100.0), wheelData.FrLongitudinalForce)
				assert.EqualValues(t, float32(0.12), wheelData.FrSuspensionPosition)
				assert.EqualValues(t, float32(0.01), wheelData.FrSuspensionVelocity)

				// Verify Back Left wheel
				assert.EqualValues(t, float32(50.0), wheelData.BlWheelSpeed)
				assert.EqualValues(t, float32(4000.0), wheelData.BlVerticalForce)
				assert.EqualValues(t, float32(0.05), wheelData.BlSlipAngle)
				assert.EqualValues(t, float32(0.15), wheelData.BlSlipRatio)
				assert.EqualValues(t, float32(200.0), wheelData.BlLateralForce)
				assert.EqualValues(t, float32(150.0), wheelData.BlLongitudinalForce)
				assert.EqualValues(t, float32(0.1), wheelData.BlSuspensionPosition)
				assert.EqualValues(t, float32(-0.01), wheelData.BlSuspensionVelocity)

				// Verify Back Right wheel
				assert.EqualValues(t, float32(48.0), wheelData.BrWheelSpeed)
				assert.EqualValues(t, float32(4200.0), wheelData.BrVerticalForce)
				assert.EqualValues(t, float32(0.03), wheelData.BrSlipAngle)
				assert.EqualValues(t, float32(0.12), wheelData.BrSlipRatio)
				assert.EqualValues(t, float32(180.0), wheelData.BrLateralForce)
				assert.EqualValues(t, float32(120.0), wheelData.BrLongitudinalForce)
				assert.EqualValues(t, float32(0.15), wheelData.BrSuspensionPosition)
				assert.EqualValues(t, float32(0.02), wheelData.BrSuspensionVelocity)
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
				p.QueryRow(t.Context(), "select position_z from telemetry.motion_data limit 1").Scan(&motionTelemetryRead.PositionZ)
				p.QueryRow(t.Context(), "select speed, rpm from telemetry.vehicle_data limit 1").Scan(&vehicleTelemetryRead.Speed, &vehicleTelemetryRead.Rpm)
				p.QueryRow(t.Context(), "select lap_num from telemetry.session_lap_data").Scan(&lapTelemetryRead.LapNum)
				p.QueryRow(t.Context(), "select current_lap_time, sector1_time from telemetry.live_lap_data").Scan(&currentLapTelemetryRead.LapTime, &currentLapTelemetryRead.Sector1Time)

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
				rows, err := p.Query(t.Context(), "select * from telemetry.session_lap_data")
				assert.NoError(t, err, "did not get rows from telemetry.session_lap_data")

				lapTelemetryRead, err := pgx.CollectExactlyOneRow(rows, func(row pgx.CollectableRow) (SessionLapDataReader, error) {
					return pgx.RowToStructByName[SessionLapDataReader](row)
				})
				require.NoError(t, err, "expected a single lap to have been recorded")
				assert.EqualValues(t, lapDataTelemetryWritten.GetLapTimesData().LapNum, lapTelemetryRead.LapNum)
				assert.EqualValues(t, lapDataTelemetryWritten.GetLapTimesData().LapTime.AsDuration(), lapTelemetryRead.LapTime)
				assert.EqualValues(t, lapDataTelemetryWritten.GetLapTimesData().Sector1Time.AsDuration(), lapTelemetryRead.Sector1Time)
				assert.EqualValues(t, lapDataTelemetryWritten.GetLapTimesData().Sector2Time.AsDuration(), lapTelemetryRead.Sector2Time)
				assert.EqualValues(t, lapDataTelemetryWritten.GetLapTimesData().Sector3Time.AsDuration(), lapTelemetryRead.Sector3Time)
				assert.EqualValues(t, "testuser", lapTelemetryRead.UserID)
				assert.EqualValues(t, "testing", lapTelemetryRead.SessionID)
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
				extendedWheelTelemetry := &pb.GameTelemetry{
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
				router.Add(t.Context(), extendedWheelTelemetry)

				// Wait for the batcher to complete the flush
				time.Sleep(100 * time.Millisecond)

				// Verify data was written to extended_wheel_data table
				extendedWheelData := &pb.ExtendedFourWheelData{
					FrontLeft: &pb.ExtendedWheelData{},
				}
				err = p.QueryRow(t.Context(), "SELECT fl_wheel_speed FROM telemetry.extended_wheel_data WHERE session_id = $1", "motion-ex-router-test").Scan(&(extendedWheelData.FrontLeft).WheelSpeed)
				require.NoError(t, err)
				assert.EqualValues(t, float32(52.0), extendedWheelData.FrontLeft.WheelSpeed)

				// Verify all four wheels
				var blSpeed, brSpeed, frSpeed float32
				err = p.QueryRow(t.Context(), "SELECT bl_wheel_speed, br_wheel_speed, fr_wheel_speed FROM telemetry.extended_wheel_data WHERE session_id = $1", "motion-ex-router-test").Scan(&blSpeed, &brSpeed, &frSpeed)
				require.NoError(t, err)
				assert.EqualValues(t, float32(50.0), blSpeed)
				assert.EqualValues(t, float32(48.0), brSpeed)
				assert.EqualValues(t, float32(51.0), frSpeed)

				// Verify suspension data
				var blSuspensionPos, frSuspensionPos float32
				err = p.QueryRow(t.Context(), "SELECT bl_suspension_position, fr_suspension_position FROM telemetry.extended_wheel_data WHERE session_id = $1", "motion-ex-router-test").Scan(&blSuspensionPos, &frSuspensionPos)
				require.NoError(t, err)
				assert.EqualValues(t, float32(0.1), blSuspensionPos)
				assert.EqualValues(t, float32(0.12), frSuspensionPos)

				// Verify slip angles
				var blSlipAngle, frSlipAngle float32
				err = p.QueryRow(t.Context(), "SELECT bl_slip_angle, fr_slip_angle FROM telemetry.extended_wheel_data WHERE session_id = $1", "motion-ex-router-test").Scan(&blSlipAngle, &frSlipAngle)
				require.NoError(t, err)
				assert.EqualValues(t, float32(0.05), blSlipAngle)
				assert.EqualValues(t, float32(0.02), frSlipAngle)
			},
		},
	}.Run(t)
}
