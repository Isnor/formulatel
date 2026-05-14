package timescale

import (
	"context"
	"log/slog"
	"sync"
	"time"

	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/jackc/pgx/v5"
)

// TableBatcher batches messages and flushes to TimescaleDB using pgx.CopyFrom.
type TableBatcher struct {
	ctx           context.Context
	tableName     string
	conn          *pgx.Conn
	msgChan       chan *pb.GameTelemetry
	batchSize     int
	flushInterval time.Duration
	ticker        *time.Ticker
	buffer        []map[string]any
	bufferMu      sync.Mutex
}

// TODO: I hate this, write a function per type instead
// buildRow converts a GameTelemetry to a row map for the specified table.
func buildRow(msg *pb.GameTelemetry, tableName string) map[string]any {
	if tableName == "vehicle_data" {
		vd := msg.GetVehicleData()
		if vd == nil {
			return nil
		}

		row := map[string]any{
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
		}

		if vd.Tires != nil {
			row["fl_brake_temp"] = vd.Tires.FrontLeft.BrakeTemperature
			row["fl_inner_temp"] = vd.Tires.FrontLeft.InnerTemperature
			row["fl_surface_temp"] = vd.Tires.FrontLeft.SurfaceTemperature
			row["fl_pressure"] = vd.Tires.FrontLeft.Pressure
			row["fr_brake_temp"] = vd.Tires.FrontRight.BrakeTemperature
			row["fr_inner_temp"] = vd.Tires.FrontRight.InnerTemperature
			row["fr_surface_temp"] = vd.Tires.FrontRight.SurfaceTemperature
			row["fr_pressure"] = vd.Tires.FrontRight.Pressure
			row["bl_brake_temp"] = vd.Tires.BackLeft.BrakeTemperature
			row["bl_inner_temp"] = vd.Tires.BackLeft.InnerTemperature
			row["bl_surface_temp"] = vd.Tires.BackLeft.SurfaceTemperature
			row["bl_pressure"] = vd.Tires.BackLeft.Pressure
			row["br_brake_temp"] = vd.Tires.BackRight.BrakeTemperature
			row["br_inner_temp"] = vd.Tires.BackRight.InnerTemperature
			row["br_surface_temp"] = vd.Tires.BackRight.SurfaceTemperature
			row["br_pressure"] = vd.Tires.BackRight.Pressure
		}

		return row
	}

	// motion_data
	md := msg.GetMotionData()
	if md == nil {
		return nil
	}

	return map[string]any{
		"time":                msg.Timestamp.AsTime(),
		"session_id":          msg.SessionId,
		"user_id":             msg.UserId,
		"title":               msg.Title,
		"position_x":          md.PositionX,
		"position_y":          md.PositionY,
		"position_z":          md.PositionZ,
		"velocity_x":          md.VelocityX,
		"velocity_y":          md.VelocityY,
		"velocity_z":          md.VelocityZ,
		"gforce_lateral":      md.GForceLateral,
		"gforce_longitudinal": md.GForceLongitudinal,
		"gforce_vertical":     md.GForceVertical,
		"yaw":                 md.Yaw,
		"pitch":               md.Pitch,
		"roll":                md.Roll,
	}
}

// NewTableBatcher creates a new TableBatcher.
func NewTableBatcher(ctx context.Context, conn *pgx.Conn, tableName string, msgChan chan *pb.GameTelemetry, batchSize int, flushInterval time.Duration) *TableBatcher {
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
	}
}

// Start begins listening to the message channel and flushes batches.
func (b *TableBatcher) Start() {
	go b.flusherWorker(b.ctx)
}

// flusherWorker runs the dual-trigger flush worker.
func (b *TableBatcher) flusherWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			b.flush()
			b.ticker.Stop()
			slog.InfoContext(ctx, "table batcher stopped", "reason", "context finished")
			return
		case msg, ok := <-b.msgChan:
			if !ok {
				b.flush()
				b.ticker.Stop()
				slog.InfoContext(ctx, "table batcher stopped", "reason", "channel closed")
				return
			}
			b.bufferMu.Lock()
			row := buildRow(msg, b.tableName)
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

	if err := b.writeBatch(rows); err != nil {
		slog.ErrorContext(b.ctx, "failed to write batch to timescaledb", "table", b.tableName, "error", err)
	} else {
		slog.DebugContext(b.ctx, "flushed batch to timescaledb", "table", b.tableName, "rows", len(rows))
	}
}

// writeBatch writes rows to the database using pgx.CopyFrom.
func (b *TableBatcher) writeBatch(rows []map[string]any) error {
	// Build column order from first row keys
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
	_, err := b.conn.CopyFrom(b.ctx, pgx.Identifier{b.tableName}, keys, sourceRowsType)
	if err != nil {
		return err
	}

	slog.DebugContext(b.ctx, "copy from result", "table", b.tableName, "rows", len(sourceRows))
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

// rowKeys extracts column keys from a row map in a stable order.
func rowKeys(row map[string]any) []string {
	if len(row) == 0 {
		return nil
	}

	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}

	// For consistent ordering, sort keys
	sortStrings(keys)
	return keys
}

// sortStrings sorts a string slice in place using simple insertion sort.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// BatchRouter routes messages to the appropriate TableBatcher.
type BatchRouter struct {
	vehicleBatcher *TableBatcher
	motionBatcher  *TableBatcher
}

// NewBatchRouter creates a new BatchRouter that routes to vehicle and motion batchers.
func NewBatchRouter(ctx context.Context, conn *pgx.Conn, msgChan chan *pb.GameTelemetry, batchSize int, flushInterval time.Duration) (*BatchRouter, error) {
	vehicleChan := make(chan *pb.GameTelemetry, 100)
	motionChan := make(chan *pb.GameTelemetry, 100)

	// Route messages to appropriate channels
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
