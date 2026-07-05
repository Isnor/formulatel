package timescale

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// GameTelemetryWithContext wraps a GameTelemetry message with its context.
// This allows trace context to be propagated from MQTT receive through to database writes.
type GameTelemetryWithContext struct {
	ctx context.Context
	msg *pb.GameTelemetry
}

// TableBatcher batches messages and flushes to TimescaleDB using pgx.CopyFrom.
type TableBatcher struct {
	ctx           context.Context
	tableName     string
	conn          *pgxpool.Pool
	msgChan       chan GameTelemetryWithContext
	batchSize     int
	flushInterval time.Duration
	ticker        *time.Ticker
	buffer        []map[string]any
	bufferMu      sync.Mutex
	tracer        trace.Tracer
}

// buildRowForCopy converts a GameTelemetry to a row map for the specified table.
func buildRowForCopy(msg *pb.GameTelemetry, tableName string) (map[string]any, error) {
	// TODO: is there a less terrible way of implementing this? I don't like that we need to map each data type
	//	to its respective table, it feels clunky.
	switch tableName {
	case "vehicle_data":
		row, err := buildVehicleDataRow(msg)
		if err != nil {
			return nil, err
		}
		return row, nil
	case "motion_data":
		row, err := buildMotionDataRow(msg)
		if err != nil {
			return nil, err
		}
		return row, nil
	case "extended_wheel_data":
		row, err := buildExtendedWheelDataRow(msg)
		if err != nil {
			return nil, err
		}
		return row, nil
	case "live_lap_data":
		row, err := buildCurrentLapDataRow(msg)
		if err != nil {
			return nil, err
		}
		return row, nil
	default:
		return nil, fmt.Errorf("unknown table name %s", tableName)
	}
}

// these `buildXRow` functions convert a GameTelemetry into a row for the table for that
// type of telemetry.

func buildMotionDataRow(msg *pb.GameTelemetry) (map[string]any, error) {
	if motionData := msg.GetMotionData(); motionData != nil {
		return map[string]any{
			"time":                msg.Timestamp.AsTime(),
			"session_id":          msg.SessionId,
			"user_id":             msg.UserId,
			"title":               msg.Title,
			"position_x":          motionData.PositionX,
			"position_y":          motionData.PositionY,
			"position_z":          motionData.PositionZ,
			"velocity_x":          motionData.VelocityX,
			"velocity_y":          motionData.VelocityY,
			"velocity_z":          motionData.VelocityZ,
			"gforce_lateral":      motionData.GForceLateral,
			"gforce_longitudinal": motionData.GForceLongitudinal,
			"gforce_vertical":     motionData.GForceVertical,
			"yaw":                 motionData.Yaw,
			"pitch":               motionData.Pitch,
			"roll":                motionData.Roll,
		}, nil
	}
	return nil, errors.New("did not write motion telemetry: no motion data found")
}

func buildVehicleDataRow(msg *pb.GameTelemetry) (map[string]any, error) {
	if vd := msg.GetVehicleData(); vd != nil {
		return map[string]any{
			"time":               msg.Timestamp.AsTime(),
			"session_id":         msg.SessionId,
			"user_id":            msg.UserId,
			"title":              msg.Title,
			"speed":              vd.Speed,
			"rpm":                vd.Rpm,
			"throttle":           vd.Throttle,
			"brake":              vd.Brake,
			"steering":           vd.Steering,
			"gear":               vd.Gear,
			"engine_temperature": vd.EngineTemperature,
			// Tire columns - include even if Tires is nil (nullable columns)
			"fl_brake_temp":   vd.Tires.FrontLeft.BrakeTemperature,
			"fl_inner_temp":   vd.Tires.FrontLeft.InnerTemperature,
			"fl_surface_temp": vd.Tires.FrontLeft.SurfaceTemperature,
			"fl_pressure":     vd.Tires.FrontLeft.Pressure,
			"fr_brake_temp":   vd.Tires.FrontRight.BrakeTemperature,
			"fr_inner_temp":   vd.Tires.FrontRight.InnerTemperature,
			"fr_surface_temp": vd.Tires.FrontRight.SurfaceTemperature,
			"fr_pressure":     vd.Tires.FrontRight.Pressure,
			"bl_brake_temp":   vd.Tires.BackLeft.BrakeTemperature,
			"bl_inner_temp":   vd.Tires.BackLeft.InnerTemperature,
			"bl_surface_temp": vd.Tires.BackLeft.SurfaceTemperature,
			"bl_pressure":     vd.Tires.BackLeft.Pressure,
			"br_brake_temp":   vd.Tires.BackRight.BrakeTemperature,
			"br_inner_temp":   vd.Tires.BackRight.InnerTemperature,
			"br_surface_temp": vd.Tires.BackRight.SurfaceTemperature,
			"br_pressure":     vd.Tires.BackRight.Pressure,
		}, nil
	}
	return nil, errors.New("did not write vehicle telemetry: no vehicle data found")
}

func buildCurrentLapDataRow(msg *pb.GameTelemetry) (map[string]any, error) {
	if lapTimes := msg.GetCurrentLapData(); lapTimes != nil {
		return map[string]any{
			"time":                  msg.Timestamp.AsTime(),
			"session_id":            msg.SessionId,
			"user_id":               msg.UserId,
			"title":                 msg.Title,
			"lap_num":               lapTimes.LapNum,
			"current_lap_time":      lapTimes.LapTime,
			"sector":                lapTimes.Sector,
			"sector1_time":          lapTimes.Sector1Time,
			"sector2_time":          lapTimes.Sector2Time,
			"delta_to_car_in_front": lapTimes.DeltaToCarInFront,
			"delta_to_race_leader":  lapTimes.DeltaToRaceLeader,
			"lap_distance":          lapTimes.LapDistance,
			"total_distance":        lapTimes.TotalDistance,
		}, nil
	}
	return nil, errors.New("did not write current lap data telemetry: no lap times data found")
}

func buildExtendedWheelDataRow(msg *pb.GameTelemetry) (map[string]any, error) {
	if exTires := msg.GetWheelData(); exTires != nil {
		return map[string]any{
			"time":       msg.Timestamp.AsTime(),
			"session_id": msg.SessionId,
			"user_id":    msg.UserId,
			"title":      msg.Title,
			// Back Left Wheel
			"bl_wheel_speed":         exTires.BackLeft.WheelSpeed,
			"bl_vertical_force":      exTires.BackLeft.VerticalForce,
			"bl_slip_angle":          exTires.BackLeft.SlipAngle,
			"bl_slip_ratio":          exTires.BackLeft.SlipRatio,
			"bl_lateral_force":       exTires.BackLeft.LateralForce,
			"bl_longitudinal_force":  exTires.BackLeft.LongitudinalForce,
			"bl_suspension_position": exTires.BackLeft.SuspensionPosition,
			"bl_suspension_velocity": exTires.BackLeft.SuspensionVelocity,
			// Back Right Wheel
			"br_wheel_speed":         exTires.BackRight.WheelSpeed,
			"br_vertical_force":      exTires.BackRight.VerticalForce,
			"br_slip_angle":          exTires.BackRight.SlipAngle,
			"br_slip_ratio":          exTires.BackRight.SlipRatio,
			"br_lateral_force":       exTires.BackRight.LateralForce,
			"br_longitudinal_force":  exTires.BackRight.LongitudinalForce,
			"br_suspension_position": exTires.BackRight.SuspensionPosition,
			"br_suspension_velocity": exTires.BackRight.SuspensionVelocity,
			// Front Left Wheel
			"fl_wheel_speed":         exTires.FrontLeft.WheelSpeed,
			"fl_vertical_force":      exTires.FrontLeft.VerticalForce,
			"fl_slip_angle":          exTires.FrontLeft.SlipAngle,
			"fl_slip_ratio":          exTires.FrontLeft.SlipRatio,
			"fl_lateral_force":       exTires.FrontLeft.LateralForce,
			"fl_longitudinal_force":  exTires.FrontLeft.LongitudinalForce,
			"fl_suspension_position": exTires.FrontLeft.SuspensionPosition,
			"fl_suspension_velocity": exTires.FrontLeft.SuspensionVelocity,
			// Front Right Wheel
			"fr_wheel_speed":         exTires.FrontRight.WheelSpeed,
			"fr_vertical_force":      exTires.FrontRight.VerticalForce,
			"fr_slip_angle":          exTires.FrontRight.SlipAngle,
			"fr_slip_ratio":          exTires.FrontRight.SlipRatio,
			"fr_lateral_force":       exTires.FrontRight.LateralForce,
			"fr_longitudinal_force":  exTires.FrontRight.LongitudinalForce,
			"fr_suspension_position": exTires.FrontRight.SuspensionPosition,
			"fr_suspension_velocity": exTires.FrontRight.SuspensionVelocity,
		}, nil
	}
	return nil, errors.New("did not write extended wheel data telemetry: no motion ex tires data found")
}

// NewTableBatcher creates a new TableBatcher.
func NewTableBatcher(ctx context.Context, conn *pgxpool.Pool, tableName string, msgChan chan GameTelemetryWithContext, batchSize int, flushInterval time.Duration) *TableBatcher {
	return &TableBatcher{
		ctx:           ctx,
		tableName:     tableName,
		conn:          conn,
		msgChan:       msgChan,
		buffer:        make([]map[string]any, 0),
		bufferMu:      sync.Mutex{},
		batchSize:     batchSize,
		flushInterval: flushInterval,
		ticker:        time.NewTicker(flushInterval),
		tracer:        otel.Tracer(fmt.Sprintf("formulatel/persist/%s", tableName)),
	}
}

// TODO: why not let the user run [flusherWorker] themselves?
// Start begins listening to the message channel and flushes batches.
func (b *TableBatcher) Start() {
	go b.flusherWorker()
}

// WriteLapRow writes a single LapTimesData row
func (b *TableBatcher) WriteLapRow(ctx context.Context, row *pb.GameTelemetry) error {
	if lapTime := row.GetLapTimesData(); lapTime != nil {
		_, err := b.conn.Exec(
			ctx,
			`INSERT INTO telemetry.session_lap_data
				(session_id, user_id, title, lap_num, lap_time, sector1_time, sector2_time, sector3_time, lap_valid, sector1_valid, sector2_valid, sector3_valid)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT DO NOTHING`,
			row.SessionId,
			row.UserId,
			row.Title.Number(),
			lapTime.LapNum,
			lapTime.LapTime.AsDuration(),
			lapTime.Sector1Time.AsDuration(),
			lapTime.Sector2Time.AsDuration(),
			lapTime.Sector3Time.AsDuration(),
			lapTime.LapValid,
			lapTime.Sector1Valid,
			lapTime.Sector2Valid,
			lapTime.Sector3Valid,
		)
		if err != nil {
			return fmt.Errorf("%w: failed writing lap time row", err)
		}
		slog.DebugContext(ctx, "wrote lap time", "session_id", row.SessionId, "user_id", row.UserId, "lap_num", lapTime.LapNum)
		return nil
	}
	slog.ErrorContext(ctx, "row did not have lap data", "row", row)
	return errors.New("cannot write lap row: no lap time data found")
}

// flusherWorker runs the dual-trigger flush worker.
func (b *TableBatcher) flusherWorker() {
	// TODO: test for edge cases on this, it might not be a good idea. It's written this way to try to
	// have a new context for each batch written that somehow links to the receive context of a message.
	for {
		traceCtx, span := b.tracer.Start(b.ctx, "table.batch")
		defer span.End()
		select {
		case <-b.ctx.Done():
			b.flush(b.ctx)
			b.ticker.Stop()
			slog.InfoContext(b.ctx, "table batcher stopped", "reason", "context finished")
			return
		case msg, ok := <-b.msgChan:
			// TODO: !ok tells us that msgChan is closed, not empty. If this isn't flushed properly,
			// we will lose rows that aren't buffered from the channel yet.
			if !ok {
				b.flush(b.ctx)
				b.ticker.Stop()
				slog.InfoContext(b.ctx, "table batcher stopped", "reason", "channel closed")
				return
			}
			// TODO: if the msg type is LapData (not the live data), we need to add it to its table with
			//	insert into ... on conflict do nothing to make sure we don't add duplicates due to how we
			//	receive lap data.
			if msg.msg.GetLapTimesData() != nil {
				// TODO: this is probably a terrible idea
				go func() {
					if err := b.WriteLapRow(traceCtx, msg.msg); err != nil {
						slog.ErrorContext(traceCtx, "did not write lap data: no lap data found", "error", err)
						span.RecordError(err)
						span.SetStatus(codes.Error, "failed writing lap data")
					}
					span.End()
				}()
				continue
			}
			// Use the per-message context for logging (for trace correlation)
			b.bufferMu.Lock()
			// Extract telemetry data from wrapped message
			row, err := buildRowForCopy(msg.msg, b.tableName)
			if err != nil {
				slog.ErrorContext(msg.ctx, "failed building row", "table_name", b.tableName, "reason", err.Error())
				span.RecordError(err)
				span.SetStatus(codes.Error, "failed building row for copy")
				span.End()
				continue
			}
			span.AddLink(trace.LinkFromContext(msg.ctx))
			if row != nil {
				b.buffer = append(b.buffer, row)
			}
			b.bufferMu.Unlock()
			if len(b.buffer) >= b.batchSize {
				b.flush(traceCtx)
				span.AddEvent("flushed batch")
				span.End()
			}
		case <-b.ticker.C:
			span.AddEvent("flushed batch")
			b.flush(traceCtx)
			span.End()
		}
	}
}

// flush writes the buffered rows to TimescaleDB.
func (b *TableBatcher) flush(ctx context.Context) {
	b.bufferMu.Lock()
	if len(b.buffer) == 0 {
		b.bufferMu.Unlock()
		return
	}

	// in the buffer (we only buffer row maps). The trace context is already propagated
	// to the flusherWorker via the message context before building rows.
	// TODO: consider storing context alongside rows for better trace correlation.
	rows := b.buffer
	b.buffer = nil
	b.bufferMu.Unlock()

	// TODO: this is a manual span created for testing, probably remove it when we get OBI working properly
	// Use b.ctx for now - per-message context was lost when building row maps
	// In a future improvement, we could store the first message's context in the buffer
	writeCtx, span := b.tracer.Start(ctx, "datastore.write", trace.WithAttributes(
		attribute.Float64("batch_size", float64(len(rows))),
		attribute.String("table", b.tableName),
	))
	defer span.End()

	if err := b.writeBatch(writeCtx, rows); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed writing batch to datastore")
		slog.ErrorContext(writeCtx, "failed to write batch to timescaledb", "table", b.tableName, "error", err)
	} else {
		slog.DebugContext(writeCtx, "flushed batch to timescaledb", "table", b.tableName, "rows", len(rows))
	}
}

// writeBatch writes rows to the database using pgx.CopyFrom.
func (b *TableBatcher) writeBatch(ctx context.Context, rows []map[string]any) error {
	// Build column order from first row keys using fixed column order
	keys := rowKeys(rows[0])

	// Build pgx.CopyFromSource with properly typed values
	sourceRows := make([][]any, len(rows))
	for i, row := range rows {
		values := make([]any, len(keys))
		for j, key := range keys {
			values[j] = row[key]
		}
		sourceRows[i] = values
	}

	// pgx.CopyFromSource requires a custom type with specific methods
	sourceRowsType := &copyFromSource{
		rows: sourceRows,
	}
	_, err := b.conn.CopyFrom(ctx, pgx.Identifier{"telemetry", b.tableName}, keys, sourceRowsType)
	if err != nil {
		return err
	}

	slog.DebugContext(ctx, "copy from result", "table", b.tableName, "rows", len(sourceRows))
	return nil
}

// copyFromSource implements pgx.CopyFromSource for our typed rows.
type copyFromSource struct {
	rows [][]any
}

func (c *copyFromSource) ReadRow(ctx context.Context) ([]any, error) {
	if len(c.rows) == 0 {
		return nil, pgx.ErrNoRows
	}
	row := c.rows[0]
	c.rows = c.rows[1:]
	return row, nil
}

func (c *copyFromSource) Err() error {
	return nil
}

func (c *copyFromSource) Next() bool {
	return len(c.rows) > 0
}

func (c *copyFromSource) Values() ([]any, error) {
	if len(c.rows) == 0 {
		return nil, nil
	}
	values := c.rows[0]
	c.rows = c.rows[1:]
	return values, nil
}

// rowKeys returns the column names of a row
func rowKeys(row map[string]any) []string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	return keys
}

// BatchRouter routes messages to the appropriate TableBatcher.
type BatchRouter struct {
	vehicleBatcher           *TableBatcher
	motionBatcher            *TableBatcher
	sessionLapDataBatcher    *TableBatcher
	liveLapDataBatcher       *TableBatcher
	extendedWheelDataBatcher *TableBatcher
}

// TODO: add config struct with envconfig tags
//
// NewBatchRouter creates a new BatchRouter that routes GameTelemetry into a channel for specific to the type of data in the GameTelemetry.
func NewBatchRouter(ctx context.Context, conn *pgxpool.Pool, msgChan chan *pb.GameTelemetry, batchSize int, flushInterval time.Duration) (*BatchRouter, error) {
	// TODO: why 100? should be configurable at least.
	vehicleChan := make(chan GameTelemetryWithContext, 100)
	motionChan := make(chan GameTelemetryWithContext, 100)
	sessionLapDataChan := make(chan GameTelemetryWithContext, 100)
	liveLapDataChan := make(chan GameTelemetryWithContext, 100)
	extendedWheelDataChan := make(chan GameTelemetryWithContext, 100)

	// TODO: probably don't need this
	go func() {
		<-ctx.Done()
		close(vehicleChan)
		close(motionChan)
		close(sessionLapDataChan)
		close(liveLapDataChan)
		close(extendedWheelDataChan)
	}()

	// TODO: we create N batchers, 1 per table, meaning the BatchRouter actually has N*batchSize capacity
	//	Maybe this is fine, but it makes the signature of this function misleading
	vehicleBatcher := NewTableBatcher(ctx, conn, "vehicle_data", vehicleChan, batchSize, flushInterval)
	motionBatcher := NewTableBatcher(ctx, conn, "motion_data", motionChan, batchSize, flushInterval)
	liveLapDataBatcher := NewTableBatcher(ctx, conn, "live_lap_data", liveLapDataChan, batchSize, flushInterval)
	sessionLapDataBatcher := NewTableBatcher(ctx, conn, "session_lap_data", sessionLapDataChan, batchSize, flushInterval)
	extendedWheelDataBatcher := NewTableBatcher(ctx, conn, "extended_wheel_data", extendedWheelDataChan, batchSize, flushInterval)

	// Start all batchers
	vehicleBatcher.Start()
	motionBatcher.Start()
	sessionLapDataBatcher.Start()
	liveLapDataBatcher.Start()
	extendedWheelDataBatcher.Start()

	return &BatchRouter{
		vehicleBatcher:           vehicleBatcher,
		motionBatcher:            motionBatcher,
		sessionLapDataBatcher:    sessionLapDataBatcher,
		liveLapDataBatcher:       liveLapDataBatcher,
		extendedWheelDataBatcher: extendedWheelDataBatcher,
	}, nil
}

// Add routes a message to all batchers (it will be processed by each appropriately).
func (b *BatchRouter) Add(ctx context.Context, msg *pb.GameTelemetry) {
	// Wrap the message with context for trace propagation
	wrapped := GameTelemetryWithContext{
		ctx: ctx,
		msg: msg,
	}

	go func() {
		// Vehicle data
		if msg.GetVehicleData() != nil {
			b.vehicleBatcher.msgChan <- wrapped
		}

		// Motion data
		if msg.GetMotionData() != nil {
			b.motionBatcher.msgChan <- wrapped
		}

		// Session lap data
		if msg.GetLapTimesData() != nil {
			slog.DebugContext(ctx, "read lap data", "lap_time", msg.GetLapTimesData())
			b.sessionLapDataBatcher.msgChan <- wrapped
		}

		// current lap data
		if msg.GetCurrentLapData() != nil {
			slog.DebugContext(ctx, "read live lap data")
			b.liveLapDataBatcher.msgChan <- wrapped
		}

		// Extended wheel data
		if msg.GetWheelData() != nil {
			slog.DebugContext(ctx, "read motion ex data", "session_id", msg.SessionId, "user_id", msg.UserId)
			b.extendedWheelDataBatcher.msgChan <- wrapped
		}
	}()
}
