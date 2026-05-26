package timescale

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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

// TableBatcher batches messages and flushes to TimescaleDB using pgx.CopyFrom.
type TableBatcher struct {
	ctx           context.Context
	tableName     string
	conn          *pgxpool.Pool
	msgChan       chan *pb.GameTelemetry
	batchSize     int
	flushInterval time.Duration
	ticker        *time.Ticker
	buffer        []map[string]any
	bufferMu      sync.Mutex
	tracer        trace.Tracer
}

// buildRow converts a GameTelemetry to a row map for the specified table.
func buildRow(msg *pb.GameTelemetry, tableName string) (map[string]any, error) {
	// vehicle data
	if tableName == "vehicle_data" {
		row, err := buildVehicleDataRow(msg)
		if err != nil {
			return nil, err
		}
		return row, nil
	}

	// motion_data
	if tableName == "motion_data" {
		row, err := buildMotionDataRow(msg)
		if err != nil {
			return nil, err
		}
		return row, nil
	}

	return nil, fmt.Errorf("unknown table name %s", tableName)
}

// TODO: I'm not overly fond of these functions; the reason it's written this way is to try to take advantage
//	of the postgres COPY protocol. Take a look at the writeBatch function for more on that implementation.

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

// NewTableBatcher creates a new TableBatcher.
func NewTableBatcher(ctx context.Context, conn *pgxpool.Pool, tableName string, msgChan chan *pb.GameTelemetry, batchSize int, flushInterval time.Duration) *TableBatcher {
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

// Start begins listening to the message channel and flushes batches.
func (b *TableBatcher) Start() {
	go b.flusherWorker()
}

// flusherWorker runs the dual-trigger flush worker.
func (b *TableBatcher) flusherWorker() {
	for {
		select {
		case <-b.ctx.Done():
			b.flush()
			b.ticker.Stop()
			slog.InfoContext(b.ctx, "table batcher stopped", "reason", "context finished")
			return
		case msg, ok := <-b.msgChan:
			if !ok {
				b.flush()
				b.ticker.Stop()
				slog.InfoContext(b.ctx, "table batcher stopped", "reason", "channel closed")
				return
			}
			b.bufferMu.Lock()
			row, err := buildRow(msg, b.tableName)
			if err != nil {
				slog.ErrorContext(b.ctx, "failed building row", "table_name", b.tableName, "reason", err.Error())
			}
			if row != nil {
				b.buffer = append(b.buffer, row)
			}
			b.bufferMu.Unlock()
			if len(b.buffer) >= b.batchSize {
				b.flush()
			}
		case <-b.ticker.C:
			b.flush()
		}
	}
}

// flush writes the buffered rows to TimescaleDB.
func (b *TableBatcher) flush() {
	b.bufferMu.Lock()
	if len(b.buffer) == 0 {
		b.bufferMu.Unlock()
		return
	}
	rows := b.buffer
	b.buffer = nil
	b.bufferMu.Unlock()

	// TODO: this is a manual span created for testing, probably remove it when we get OBI working properly
	ctx, span := b.tracer.Start(b.ctx, "datastore.write", trace.WithAttributes(
		attribute.Float64("batch_size", float64(len(rows))),
	))
	defer func() {
		slog.DebugContext(ctx, "flushed batch to timescaledb", "table", b.tableName, "rows", len(rows))
		span.End()
	}()

	if err := b.writeBatch(ctx, rows); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed writing batch to datastore")
		slog.ErrorContext(ctx, "failed to write batch to timescaledb", "table", b.tableName, "error", err)
	} else {
		slog.DebugContext(ctx, "flushed batch to timescaledb", "table", b.tableName, "rows", len(rows))
	}
}

// writeBatch writes rows to the database using pgx.CopyFrom.
func (b *TableBatcher) writeBatch(ctx context.Context, rows []map[string]any) error {
	// Build column order from first row keys using fixed column order
	keys := rowKeys(rows[0], b.tableName)

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
	_, err := b.conn.CopyFrom(ctx, pgx.Identifier{b.tableName}, keys, sourceRowsType)
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

// columnOrder maps column name to its position in the database schema
// This ensures consistent column ordering across all batch writes
var vehicleDataColumnOrder = []string{
	"time", "session_id", "user_id", "title", "speed", "rpm", "throttle",
	"brake", "steering", "gear", "engine_temperature",
	"fl_brake_temp", "fl_inner_temp", "fl_surface_temp", "fl_pressure",
	"fr_brake_temp", "fr_inner_temp", "fr_surface_temp", "fr_pressure",
	"bl_brake_temp", "bl_inner_temp", "bl_surface_temp", "bl_pressure",
	"br_brake_temp", "br_inner_temp", "br_surface_temp", "br_pressure",
}

var motionDataColumnOrder = []string{
	"time", "session_id", "user_id", "title",
	"position_x", "position_y", "position_z",
	"velocity_x", "velocity_y", "velocity_z",
	"gforce_lateral", "gforce_longitudinal", "gforce_vertical",
	"yaw", "pitch", "roll",
}

// rowKeys extracts column keys from a row map in database schema order.
func rowKeys(row map[string]any, tableName string) []string {
	if tableName == "vehicle_data" {
		// Return columns in fixed database order, filtering to only columns present in row
		keys := make([]string, 0, len(vehicleDataColumnOrder))
		for _, col := range vehicleDataColumnOrder {
			if _, ok := row[col]; ok {
				keys = append(keys, col)
			}
		}
		return keys
	}

	if tableName == "motion_data" {
		keys := make([]string, 0, len(motionDataColumnOrder))
		for _, col := range motionDataColumnOrder {
			if _, ok := row[col]; ok {
				keys = append(keys, col)
			}
		}
		return keys
	}

	// Fallback: use alphabetically sorted keys
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// BatchRouter routes messages to the appropriate TableBatcher.
type BatchRouter struct {
	vehicleBatcher *TableBatcher
	motionBatcher  *TableBatcher
}

// TODO: I hate everything about this; use a struct to define the apparently endless number of parameters we will need
//
//	or just pass the Config object in
//
// NewBatchRouter creates a new BatchRouter that routes to vehicle and motion batchers.
func NewBatchRouter(ctx context.Context, conn *pgxpool.Pool, msgChan chan *pb.GameTelemetry, batchSize int, flushInterval time.Duration) (*BatchRouter, error) {
	// TODO: why 100? should be configurable at least.
	vehicleChan := make(chan *pb.GameTelemetry, 100)
	motionChan := make(chan *pb.GameTelemetry, 100)

	// Route messages to appropriate channels
	// TODO: the relationship between this routine and the Add function are confusing. Why do we need Add if we have this?
	// 	Why isn't Add simply stuffing everything into `msgChan` and letting this routine do the routing?
	go func() {
		for msg := range msgChan {
			if msg.GetVehicleData() != nil {
				vehicleChan <- msg
			}
			if msg.GetMotionData() != nil {
				motionChan <- msg
			}
		}
		close(vehicleChan)
		close(motionChan)
	}()

	// TODO: we create N batchers, 1 per table, meaning the BatchRouter actually has N*batchSize capacity
	//	Maybe this is fine, but it makes the signature of this function misleading
	vehicleBatcher := NewTableBatcher(ctx, conn, "vehicle_data", vehicleChan, batchSize, flushInterval)
	motionBatcher := NewTableBatcher(ctx, conn, "motion_data", motionChan, batchSize, flushInterval)

	// Start both batchers
	vehicleBatcher.Start()
	motionBatcher.Start()

	return &BatchRouter{
		vehicleBatcher: vehicleBatcher,
		motionBatcher:  motionBatcher,
	}, nil
}

// Add routes a message to both batchers (it will be processed by each appropriately).
func (b *BatchRouter) Add(msg *pb.GameTelemetry) {
	// Vehicle data takes priority
	if msg.GetVehicleData() != nil {
		select {
		case b.vehicleBatcher.msgChan <- msg:
		default:
			// Channel full, skip to avoid blocking
		}
	}

	// Motion data
	if msg.GetMotionData() != nil {
		select {
		case b.motionBatcher.msgChan <- msg:
		default:
			// Channel full, skip to avoid blocking
		}
	}
}
