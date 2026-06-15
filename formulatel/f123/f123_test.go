package f123

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Isnor/testtable"
	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/nettest"
)

func readTestPackets() (map[PacketType][][]byte, error) {
	result := make(map[PacketType][][]byte)

	for _, x := range []PacketType{CarMotionPacket, CarTelemetryPacket, LapDataPacket, SessionHistoryPacket} {
		var packets [][]byte
		err := filepath.WalkDir(fmt.Sprintf("./testdata/%d", x), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			packet, err := os.ReadFile(path)
			if err != nil {
				slog.Error("failed reading file", "filename", path, "error", err.Error())
				return err
			}
			packets = append(packets, packet)
			return nil
		})
		if err != nil {
			return nil, err
		}
		result[x] = packets
	}
	return result, nil
}

// TODO: read test packet data from github, s3, etc. - something.
type F123IngestTest struct {
	name         string
	config       F123IngestConfig
	expectations func(t *testing.T, conn net.PacketConn, packets map[PacketType][][]byte)
}

func (it *F123IngestTest) Run(t *testing.T) {
	// create packet listener to write test packets to
	conn, err := nettest.NewLocalPacketListener("udp")
	require.NoError(t, err)
	ingest := NewF123Ingest(it.config, conn)

	ingestCtx, cancelIngest := context.WithCancel(t.Context())
	defer cancelIngest()

	packets, err := readTestPackets()
	require.NoError(t, err)
	// listen for packets from UDP connection
	go ingest.Listen(ingestCtx)
	// convert packets we read into formulatel format
	go ingest.Consume(ingestCtx)

	t.Run(it.name, func(t *testing.T) {
		it.expectations(t, conn, packets)
	})
}

func TestF123Ingest(t *testing.T) {
	// "when I receive f123 telemetry, I should receive a corresponding GameTelemetry in the vehicle data channel"
	config := F123IngestConfig{
		VehicleDataChannel:    make(chan *pb.GameTelemetry, 100),
		MotionDataChannel:     make(chan *pb.GameTelemetry, 100),
		CurrentLapDataChannel: make(chan *pb.GameTelemetry, 100),
		LapTimesDataChannel:   make(chan *pb.GameTelemetry, 100),
	}
	testtable.TestTable{
		&F123IngestTest{
			name:   "motion data",
			config: config,
			expectations: func(t *testing.T, conn net.PacketConn, packets map[PacketType][][]byte) {
				// writeConn, err := net.Dial("udp", conn.LocalAddr().String())
				// require.NoError(t, err)
				// defer conn.Close()
				for _, p := range packets[CarMotionPacket] {
					n, err := conn.WriteTo(p, conn.LocalAddr())
					// n, err := writeConn.Write(p)
					assert.NoError(t, err)
					assert.Equal(t, len(p), n)
				}

				time.Sleep(100 * time.Millisecond)
				assert.Equal(t, len(packets[CarMotionPacket]), len(config.MotionDataChannel))
			},
		},
		&F123IngestTest{
			name:   "vehicle data",
			config: config,
			expectations: func(t *testing.T, conn net.PacketConn, packets map[PacketType][][]byte) {
				// writeConn, err := net.Dial("udp", conn.LocalAddr().String())
				// require.NoError(t, err)
				// defer conn.Close()
				for _, p := range packets[CarTelemetryPacket] {
					// n, err := writeConn.Write(p)
					n, err := conn.WriteTo(p, conn.LocalAddr())
					assert.NoError(t, err)
					assert.Equal(t, len(p), n)
				}

				time.Sleep(100 * time.Millisecond)
				assert.Equal(t, len(packets[CarTelemetryPacket]), len(config.VehicleDataChannel))
			},
		},
		&F123IngestTest{
			name:   "current lap data",
			config: config,
			expectations: func(t *testing.T, conn net.PacketConn, packets map[PacketType][][]byte) {
				// writeConn, err := net.Dial("udp", conn.LocalAddr().String())
				// require.NoError(t, err)
				// defer conn.Close()
				for _, p := range packets[LapDataPacket] {
					n, err := conn.WriteTo(p, conn.LocalAddr())
					// n, err := writeConn.Write(p)
					assert.NoError(t, err)
					assert.Equal(t, len(p), n)
				}

				time.Sleep(100 * time.Millisecond)
				assert.Equal(t, len(packets[LapDataPacket]), len(config.CurrentLapDataChannel))
			},
		},
		&F123IngestTest{
			name:   "lap times data",
			config: config,
			expectations: func(t *testing.T, conn net.PacketConn, packets map[PacketType][][]byte) {
				for _, p := range packets[SessionHistoryPacket] {
					n, err := conn.WriteTo(p, conn.LocalAddr())

					assert.NoError(t, err)
					assert.Equal(t, len(p), n)
				}
				// this is kind of a weird test, but the SessionHistory packet we have saved for this test has 3 laps in it,
				// 2 of which are completed.
				time.Sleep(100 * time.Millisecond)
				assert.Equal(t, 2, len(config.LapTimesDataChannel), "should have only read 2 laps from test data")

				// if we write the same packets again to simulate getting lap data that we've already received,
				// ingest should be keeping track of the latest lap it sent and not add any more to the channel
				for _, p := range packets[SessionHistoryPacket] {
					n, err := conn.WriteTo(p, conn.LocalAddr())

					assert.NoError(t, err)
					assert.Equal(t, len(p), n)
				}
				time.Sleep(100 * time.Millisecond)
				assert.Equal(t, 2, len(config.LapTimesDataChannel), "should not have queued previously-read laps")
			},
		},
	}.Run(t)
}
