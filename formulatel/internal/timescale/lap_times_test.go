package timescale

import (
	"context"
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
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestLapDataBasic writes basic lap data to the database.
func TestLapDataBasic(t *testing.T) {
	ctx := context.Background()
	t.Parallel()

	testtable.TestTable{
		&PostgresTest{
			Name: "insert lap data with all fields",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				// Create a batcher for lap_times
				connStr := p.Config().ConnString()
				msgChan := make(chan GameTelemetryWithContext, 10)
				defer close(msgChan)

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()
				assert.NoError(t, migrator.Up())

				// Create batcher
				batcher := NewTableBatcher(ctx, p, "lap_times", msgChan, 1, 50*time.Millisecond)
				batcher.Start()

				// Create lap data with all fields
				msg := &pb.GameTelemetry{
					Title:     pb.GameTitle_GAME_TITLE_F123,
					SessionId: "lap-test",
					UserId:    "test-user",
					Timestamp: timestamppb.New(time.Now()),
					Data: &pb.GameTelemetry_LapTimesData{
						LapTimesData: &pb.LapTimesData{
							LapTime:            90000,
							CurrentLapTime:     85000,
							Sector1Time:        25000,
							Sector2Time:        24500,
							Sector3Time:        24000,
							DeltaToCarInFront:  1200,
							DeltaToRaceLeader:  2300,
							LapDistance:        5200.5,
							TotalDistance:      15800.0,
							CarPosition:        3,
							CurrentLapNum:      5,
							GridPosition:       4,
							PitStatus:          0,
							NumPitStops:        1,
							DriverStatus:       1,
							ResultStatus:       0,
							PitLaneTimerActive: 0,
							PitLaneTime:        0,
						},
					},
				}

				// Send message directly to the batcher channel
				batcher.msgChan <- GameTelemetryWithContext{ctx: ctx, msg: msg}

				// Wait for flush
				time.Sleep(150 * time.Millisecond)

				// Verify data was written
				var lapTime int32
				err = p.QueryRow(ctx,
					"SELECT lap_time FROM lap_times WHERE session_id = $1 AND user_id = $2",
					"lap-test", "test-user").Scan(&lapTime)
				require.NoError(t, err, "lap data should have been written")
				assert.EqualValues(t, 90000, lapTime)

				// Verify all fields
				var currentTime int32
				p.QueryRow(ctx, "SELECT current_lap_time FROM lap_times WHERE session_id = $1", "lap-test").Scan(&currentTime)
				assert.EqualValues(t, 85000, currentTime)

				var sector1Time int32
				p.QueryRow(ctx, "SELECT sector1_time FROM lap_times WHERE session_id = $1", "lap-test").Scan(&sector1Time)
				assert.EqualValues(t, 25000, sector1Time)
			},
		},
		&PostgresTest{
			Name: "insert lap data with minimal fields",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				connStr := p.Config().ConnString()
				msgChan := make(chan GameTelemetryWithContext, 10)
				defer close(msgChan)

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()
				assert.NoError(t, migrator.Up())

				batcher := NewTableBatcher(ctx, p, "lap_times", msgChan, 1, 50*time.Millisecond)
				batcher.Start()

				// Create lap data with minimal fields (optional fields are 0)
				msg := &pb.GameTelemetry{
					Title:     pb.GameTitle_GAME_TITLE_F123,
					SessionId: "lap-test-partial",
					UserId:    "test-user-partial",
					Timestamp: timestamppb.New(time.Now()),
					Data: &pb.GameTelemetry_LapTimesData{
						LapTimesData: &pb.LapTimesData{
							LapTime:        65000,
							CurrentLapTime: 62000,
							CarPosition:    1,
							CurrentLapNum:  1,
							GridPosition:   1,
							DriverStatus:   1,
							ResultStatus:   0,
						},
					},
				}

				batcher.msgChan <- GameTelemetryWithContext{ctx: ctx, msg: msg}

				time.Sleep(150 * time.Millisecond)

				// Verify data was written
				var lapTime int32
				p.QueryRow(ctx, "SELECT lap_time FROM lap_times WHERE session_id = $1", "lap-test-partial").Scan(&lapTime)
				assert.EqualValues(t, 65000, lapTime)
			},
		},
		&PostgresTest{
			Name: "insert multiple lap times for same session",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				connStr := p.Config().ConnString()
				msgChan := make(chan GameTelemetryWithContext, 10)
				defer close(msgChan)

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()
				assert.NoError(t, migrator.Up())

				batcher := NewTableBatcher(ctx, p, "lap_times", msgChan, 5, 100*time.Millisecond)
				batcher.Start()

				// Insert 5 lap times
				for i := range 5 {
					lapNum := uint32(i + 1)
					msg := &pb.GameTelemetry{
						Title:     pb.GameTitle_GAME_TITLE_F123,
						SessionId: "multi-lap-test",
						UserId:    "test-user-multi",
						Timestamp: timestamppb.New(time.Now()),
						Data: &pb.GameTelemetry_LapTimesData{
							LapTimesData: &pb.LapTimesData{
								LapTime:           uint32(88000 + (i * 100)),
								CurrentLapTime:    uint32(85000 + (i * 100)),
								CarPosition:       uint32(i + 1),
								CurrentLapNum:     lapNum,
								DeltaToRaceLeader: uint32(i * 500),
								DriverStatus:      1,
								ResultStatus:      0,
							},
						},
					}
					batcher.msgChan <- GameTelemetryWithContext{ctx: ctx, msg: msg}
				}

				time.Sleep(500 * time.Millisecond)

				// Verify all laps were written
				var count int
				err = p.QueryRow(ctx, "SELECT COUNT(*) FROM lap_times WHERE session_id = $1", "multi-lap-test").Scan(&count)
				require.NoError(t, err)
				assert.EqualValues(t, 5, count)
			},
		},
		&PostgresTest{
			Name: "insert lap data with zero values",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				connStr := p.Config().ConnString()
				msgChan := make(chan GameTelemetryWithContext, 10)
				defer close(msgChan)

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()
				assert.NoError(t, migrator.Up())

				batcher := NewTableBatcher(ctx, p, "lap_times", msgChan, 1, 50*time.Millisecond)
				batcher.Start()

				// Lap data with all optional fields zeroed
				msg := &pb.GameTelemetry{
					Title:     pb.GameTitle_GAME_TITLE_F123,
					SessionId: "lap-test-zeros",
					UserId:    "test-user-zeros",
					Timestamp: timestamppb.New(time.Now()),
					Data: &pb.GameTelemetry_LapTimesData{
						LapTimesData: &pb.LapTimesData{
							LapTime:            0,
							CurrentLapTime:     0,
							Sector1Time:        0,
							Sector2Time:        0,
							Sector3Time:        0,
							DeltaToCarInFront:  0,
							DeltaToRaceLeader:  0,
							LapDistance:        0,
							TotalDistance:      0,
							CarPosition:        0,
							CurrentLapNum:      0,
							PitStatus:          0,
							NumPitStops:        0,
							GridPosition:       0,
							DriverStatus:       0,
							ResultStatus:       0,
							PitLaneTimerActive: 0,
							PitLaneTime:        0,
						},
					},
				}

				batcher.msgChan <- GameTelemetryWithContext{ctx: ctx, msg: msg}

				time.Sleep(150 * time.Millisecond)

				// Verify data was written with zero values
				var lapTime int32
				err = p.QueryRow(ctx, "SELECT lap_time FROM lap_times WHERE session_id = $1", "lap-test-zeros").Scan(&lapTime)
				// Should be able to read zero values
				assert.NoError(t, err, "zero value lap data should be written")
				assert.EqualValues(t, 0, lapTime)
			},
		},
	}.Run(t)
}

// TestLapDataBatcher tests the lap times batcher specifically.
func TestLapDataBatcher(t *testing.T) {
	ctx := context.Background()
	t.Parallel()

	testtable.TestTable{
		&PostgresTest{
			Name: "lap times batcher with batch size 3",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				connStr := p.Config().ConnString()
				msgChan := make(chan GameTelemetryWithContext, 10)
				defer close(msgChan)

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()
				assert.NoError(t, migrator.Up())

				// Batch size of 3
				batcher := NewTableBatcher(ctx, p, "lap_times", msgChan, 3, 1*time.Second)
				batcher.Start()

				// Send 3 lap messages
				for i := 0; i < 3; i++ {
					msg := &pb.GameTelemetry{
						Title:     pb.GameTitle_GAME_TITLE_F123,
						SessionId: "batch-lap-test",
						UserId:    "test-user-batch",
						Timestamp: timestamppb.New(time.Now()),
						Data: &pb.GameTelemetry_LapTimesData{
							LapTimesData: &pb.LapTimesData{
								LapTime:        90000 + uint32(i*100),
								CurrentLapTime: 87000 + uint32(i*100),
								CarPosition:    uint32(i + 1),
								CurrentLapNum:  uint32(i + 1),
							},
						},
					}
					batcher.msgChan <- GameTelemetryWithContext{ctx: ctx, msg: msg}
				}

				// Wait for batch to flush
				time.Sleep(500 * time.Millisecond)

				// Verify all laps were written
				var count int
				p.QueryRow(ctx, "SELECT COUNT(*) FROM lap_times WHERE session_id = $1", "batch-lap-test").Scan(&count)
				assert.EqualValues(t, 3, count)
			},
		},
		&PostgresTest{
			Name: "lap times batcher flush on interval",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				connStr := p.Config().ConnString()
				msgChan := make(chan GameTelemetryWithContext, 10)
				defer close(msgChan)

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()
				assert.NoError(t, migrator.Up())

				// Batch size larger than we'll send, but flush interval is short
				batcher := NewTableBatcher(ctx, p, "lap_times", msgChan, 100, 50*time.Millisecond)
				batcher.Start()

				// Send 1 lap
				msg := &pb.GameTelemetry{
					Title:     pb.GameTitle_GAME_TITLE_F123,
					SessionId: "interval-lap-test",
					UserId:    "test-user-interval",
					Timestamp: timestamppb.New(time.Now()),
					Data: &pb.GameTelemetry_LapTimesData{
						LapTimesData: &pb.LapTimesData{
							LapTime:        95000,
							CurrentLapTime: 92000,
							CarPosition:    1,
							CurrentLapNum:  1,
						},
					},
				}
				batcher.msgChan <- GameTelemetryWithContext{ctx: ctx, msg: msg}

				// Wait for interval-based flush
				time.Sleep(200 * time.Millisecond)

				// Verify lap was written
				var lapTime int32
				p.QueryRow(ctx, "SELECT lap_time FROM lap_times WHERE session_id = $1", "interval-lap-test").Scan(&lapTime)
				assert.EqualValues(t, 95000, lapTime)
			},
		},
	}.Run(t)
}

// TestLapDataIntegration tests end-to-end lap data flow through router.
func TestLapDataIntegration(t *testing.T) {
	ctx := context.Background()
	t.Parallel()

	testtable.TestTable{
		&PostgresTest{
			Name: "router with lap data",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				connStr := p.Config().ConnString()
				msgChan := make(chan *pb.GameTelemetry)
				defer close(msgChan)

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()
				assert.NoError(t, migrator.Up())

				// Create router with batch size 1 (write immediately)
				router, err := NewBatchRouter(ctx, p, msgChan, 1, 10*time.Millisecond)
				require.NoError(t, err)

				// Create lap data message
				msg := &pb.GameTelemetry{
					Title:     pb.GameTitle_GAME_TITLE_F123,
					SessionId: "router-lap-test",
					UserId:    "test-user-router",
					Timestamp: timestamppb.New(time.Now()),
					Data: &pb.GameTelemetry_LapTimesData{
						LapTimesData: &pb.LapTimesData{
							LapTime:           95000,
							CurrentLapTime:    92000,
							Sector1Time:       30000,
							Sector2Time:       28000,
							Sector3Time:       27000,
							DeltaToCarInFront: 0,
							DeltaToRaceLeader: 0,
							CarPosition:       1,
							CurrentLapNum:     1,
							GridPosition:      1,
							DriverStatus:      1,
							ResultStatus:      0,
						},
					},
				}

				// Route should write to lap_times table
				router.Add(ctx, msg)

				// Wait for flush
				time.Sleep(100 * time.Millisecond)

				// Verify lap data was written
				var lapTime int32
				err = p.QueryRow(ctx, "SELECT lap_time FROM lap_times WHERE session_id = $1", "router-lap-test").Scan(&lapTime)
				require.NoError(t, err, "lap data should be written by router")
				assert.EqualValues(t, 95000, lapTime)
			},
		},
		&PostgresTest{
			Name: "router with empty lap data",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				connStr := p.Config().ConnString()
				msgChan := make(chan *pb.GameTelemetry)
				defer close(msgChan)

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()
				assert.NoError(t, migrator.Up())

				// Create router
				router, err := NewBatchRouter(ctx, p, msgChan, 1, 10*time.Millisecond)
				require.NoError(t, err)

				// Create message without lap data
				msg := &pb.GameTelemetry{
					Title:     pb.GameTitle_GAME_TITLE_F123,
					SessionId: "router-empty-lap-test",
					UserId:    "test-user-empty",
					Timestamp: timestamppb.New(time.Now()),
					Data:      nil,
				}

				// Router should handle gracefully
				router.Add(ctx, msg)

				// Wait for flush
				time.Sleep(100 * time.Millisecond)

				// Verify no lap data was written
				var count int
				err = p.QueryRow(ctx, "SELECT COUNT(*) FROM lap_times WHERE session_id = $1", "router-empty-lap-test").Scan(&count)
				require.NoError(t, err)
				assert.EqualValues(t, 0, count)
			},
		},
		&PostgresTest{
			Name: "router concurrent lap data",
			Expectations: func(t *testing.T, p *pgxpool.Pool, err error) {

				connStr := p.Config().ConnString()
				msgChan := make(chan *pb.GameTelemetry, 100)
				defer close(msgChan)

				migrator, err := migrate.New(migrationsPath, connStr)
				require.NoError(t, err)
				defer migrator.Close()
				assert.NoError(t, migrator.Up())

				// Create router
				router, err := NewBatchRouter(ctx, p, msgChan, 5, 50*time.Millisecond)
				require.NoError(t, err)

				// Concurrently route multiple lap messages
				for i := 0; i < 10; i++ {
					go func(i int) {
						msg := &pb.GameTelemetry{
							Title:     pb.GameTitle_GAME_TITLE_F123,
							SessionId: "router-concurrent-lap-test",
							UserId:    "test-user-concurrent",
							Timestamp: timestamppb.New(time.Now()),
							Data: &pb.GameTelemetry_LapTimesData{
								LapTimesData: &pb.LapTimesData{
									LapTime:        90000 + uint32(i*100),
									CurrentLapTime: 87000 + uint32(i*100),
									CarPosition:    uint32(i + 1),
									CurrentLapNum:  uint32(i + 1),
								},
							},
						}
						router.Add(ctx, msg)
					}(i)
				}

				// Wait for all to complete
				time.Sleep(500 * time.Millisecond)

				// Verify all laps were written
				var count int
				p.QueryRow(ctx, "SELECT COUNT(*) FROM lap_times WHERE session_id = $1", "router-concurrent-lap-test").Scan(&count)
				assert.EqualValues(t, 10, count)
			},
		},
	}.Run(t)
}

// TestLapTimesDataRowBuilding tests the buildLapTimesDataRow function directly.
func TestBuildLapTimesDataRow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		input       *pb.GameTelemetry
		expectError bool
		expectedKey string
		expectedVal uint32
	}{
		{
			name: "complete lap data",
			input: &pb.GameTelemetry{
				Title:     pb.GameTitle_GAME_TITLE_F123,
				SessionId: "test-session",
				UserId:    "test-user",
				Timestamp: timestamppb.New(time.Now()),
				Data: &pb.GameTelemetry_LapTimesData{
					LapTimesData: &pb.LapTimesData{
						LapTime:           90000,
						CurrentLapTime:    85000,
						Sector1Time:       25000,
						Sector2Time:       24000,
						Sector3Time:       24000,
						DeltaToCarInFront: 500,
						DeltaToRaceLeader: 1000,
						CarPosition:       2,
						CurrentLapNum:     3,
						GridPosition:      2,
						DriverStatus:      1,
						ResultStatus:      0,
					},
				},
			},
			expectError: false,
			expectedKey: "lap_time",
			expectedVal: 90000,
		},
		{
			name: "empty lap data",
			input: &pb.GameTelemetry{
				Title:     pb.GameTitle_GAME_TITLE_F123,
				SessionId: "test-session",
				UserId:    "test-user",
				Timestamp: timestamppb.New(time.Now()),
				Data: &pb.GameTelemetry_LapTimesData{
					LapTimesData: &pb.LapTimesData{},
				},
			},
			expectError: false,
			expectedKey: "lap_time",
			expectedVal: 0,
		},
		{
			name: "nil lap data",
			input: &pb.GameTelemetry{
				Title:     pb.GameTitle_GAME_TITLE_F123,
				SessionId: "test-session",
				UserId:    "test-user",
				Timestamp: timestamppb.New(time.Now()),
				Data:      nil,
			},
			expectError: true,
		},
		{
			name: "zero lap data",
			input: &pb.GameTelemetry{
				Title:     pb.GameTitle_GAME_TITLE_F123,
				SessionId: "test-session",
				UserId:    "test-user",
				Timestamp: timestamppb.New(time.Now()),
				Data: &pb.GameTelemetry_LapTimesData{
					LapTimesData: &pb.LapTimesData{
						LapTime: uint32(0),
					},
				},
			},
			expectError: false,
			expectedKey: "lap_time",
			expectedVal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, err := buildLapTimesDataRow(tt.input)
			if tt.expectError {
				assert.Error(t, err, "expected error for %s", tt.name)
			} else {
				assert.NoError(t, err, "no error expected for %s", tt.name)
				assert.NotNil(t, row, "row should not be nil for %s", tt.name)
				assert.EqualValues(t, tt.expectedVal, row[tt.expectedKey], "value mismatch for %s", tt.name)
			}
		})
	}
}

// TestLapTimesRowKeys tests column ordering for lap times table.
func TestLapTimesRowKeys(t *testing.T) {
	tests := []struct {
		name      string
		row       map[string]any
		tableName string
		expected  []string
	}{
		{
			name: "complete row",
			row: map[string]any{
				"time":                  time.Now(),
				"session_id":            "test",
				"user_id":               "user",
				"title":                 1,
				"lap_time":              90000,
				"current_lap_time":      85000,
				"sector1_time":          25000,
				"sector2_time":          24000,
				"sector3_time":          24000,
				"delta_to_car_in_front": 500,
				"delta_to_race_leader":  1000,
				"lap_distance":          5200.5,
				"total_distance":        15800.0,
				"car_position":          2,
				"current_lap_num":       3,
				"pit_status":            0,
				"num_pit_stops":         1,
				"grid_position":         2,
				"driver_status":         1,
				"result_status":         0,
				"pit_lane_timer_active": 0,
				"pit_lane_time":         0,
			},
			tableName: "lap_times",
			expected:  []string{"time", "session_id", "user_id", "title", "lap_time", "current_lap_time", "sector1_time", "sector2_time", "sector3_time", "delta_to_car_in_front", "delta_to_race_leader", "lap_distance", "total_distance", "car_position", "current_lap_num", "pit_status", "num_pit_stops", "grid_position", "driver_status", "result_status", "pit_lane_timer_active", "pit_lane_time"},
		},
		{
			name: "partial row",
			row: map[string]any{
				"time":             time.Now(),
				"session_id":       "test",
				"user_id":          "user",
				"title":            1,
				"lap_time":         90000,
				"current_lap_time": 85000,
				"sector1_time":     25000,
				"sector2_time":     24000,
				"sector3_time":     24000,
				"lap_distance":     5200.5,
				"total_distance":   15800.0,
			},
			tableName: "lap_times",
			expected:  []string{"time", "session_id", "user_id", "title", "lap_time", "current_lap_time", "sector1_time", "sector2_time", "sector3_time", "lap_distance", "total_distance"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := rowKeys(tt.row, tt.tableName)
			assert.Equal(t, tt.expected, keys, "column order mismatch for %s", tt.name)
		})
	}
}

// TestLapTimesColumnOrder validates the expected column order.
func TestLapTimesColumnOrder(t *testing.T) {
	assert.NotNil(t, lapTimesColumnOrder, "lapTimesColumnOrder should be defined")
	assert.Len(t, lapTimesColumnOrder, 22, "lapTimesColumnOrder should have 22 columns")

	// Verify first few columns
	assert.Equal(t, "time", lapTimesColumnOrder[0], "first column should be time")
	assert.Equal(t, "session_id", lapTimesColumnOrder[1], "second column should be session_id")
	assert.Equal(t, "user_id", lapTimesColumnOrder[2], "third column should be user_id")
	assert.Equal(t, "title", lapTimesColumnOrder[3], "fourth column should be title")
	assert.Equal(t, "lap_time", lapTimesColumnOrder[4], "fifth column should be lap_time")
}
