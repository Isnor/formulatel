package f123

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/nettest"
)

// F123IngestTest is used to define simple tests that run assertions against
// an instance of [F123Ingest]
type F123IngestTest struct {
	name         string
	expectations func(*testing.T, *F123Ingest)
}

// Instead of a "normal" test table that runs the child tests' Run function,
// this TestTable runs the child tests' expectations function directly to allow
// them to share an [F123Ingest] instance
type F123IngestTestTable struct {
	tests  []*F123IngestTest
	config F123IngestConfig
}

// Run provides this TestTable's child tests an instance of ingest to run expectations against.
// The `ingest` should have consumed all of the test packets by the time the expectations
// run.
func (f F123IngestTestTable) Run(t *testing.T) {
	// create packet listener to write test packets to
	conn, err := nettest.NewLocalPacketListener("udp")
	require.NoError(t, err)
	ingest := NewF123Ingest(f.config, conn)

	ingestCtx, cancelIngest := context.WithCancel(t.Context())
	defer cancelIngest()

	packets, err := readTestPackets()
	require.NoError(t, err)
	// listen for packets from UDP connection
	go ingest.Listen(ingestCtx)
	// convert packets we read into formulatel format
	go ingest.Consume(ingestCtx)

	// send all of the test packets to ingest
	for packetType, packet := range packets {
		for _, p := range packet {
			n, err := conn.WriteTo(p, conn.LocalAddr())
			require.NoError(t, err, "failed writing packet type %d", packetType)
			assert.Equal(t, len(p), n)
		}
	}

	// allow time for ingest to read the packets, may need to arbitrarily increase for CI to run
	// properly; maybe we could instead do something that isn't terrible?
	time.Sleep(100 * time.Millisecond)

	for _, test := range f.tests {
		t.Run(test.name, func(t *testing.T) {
			test.expectations(t, ingest)
		})
	}
}

func TestF123Ingest(t *testing.T) {
	config := F123IngestConfig{
		VehicleDataChannel:       make(chan *pb.GameTelemetry, 100),
		MotionDataChannel:        make(chan *pb.GameTelemetry, 100),
		CurrentLapDataChannel:    make(chan *pb.GameTelemetry, 100),
		LapTimesDataChannel:      make(chan *pb.GameTelemetry, 100),
		ExtendedWheelDataChannel: make(chan *pb.GameTelemetry, 100),
	}
	F123IngestTestTable{
		config: config,
		tests: []*F123IngestTest{
			{
				name: "motion data",
				expectations: func(t *testing.T, ingest *F123Ingest) {
					t.Parallel()
					// our test data has a single motion packet
					assert.Equal(t, 1, len(ingest.MotionDataChannel))
				},
			},
			{
				name: "vehicle data",
				expectations: func(t *testing.T, ingest *F123Ingest) {
					t.Parallel()
					// our test data has a single vehicle packet
					assert.Equal(t, 1, len(ingest.VehicleDataChannel))
				},
			},
			{
				name: "current lap data",
				expectations: func(t *testing.T, ingest *F123Ingest) {
					t.Parallel()
					// our test data has a single current lap packet
					assert.Equal(t, 1, len(ingest.CurrentLapDataChannel))
				},
			},
			{
				name: "lap times data",
				expectations: func(t *testing.T, ingest *F123Ingest) {
					t.Parallel()
					// our test data has a single lap packet with 2 completed laps
					assert.Equal(t, 2, len(ingest.LapTimesDataChannel))
				},
			},
			{
				name: "extended wheel data",
				expectations: func(t *testing.T, ingest *F123Ingest) {
					t.Parallel()
					// our test data has a single lap packet with 2 completed laps
					assert.Equal(t, 1, len(ingest.ExtendedWheelDataChannel))
				},
			},
		},
	}.Run(t)
}

// TODO: read test packet data from github, s3, etc. - something.
// readTestPackets reads packets from `./testdata/{packet_type}/`, one packet per-file.
func readTestPackets() (map[PacketType][][]byte, error) {
	result := make(map[PacketType][][]byte)

	for _, x := range []PacketType{CarMotionPacket, CarTelemetryPacket, LapDataPacket, SessionHistoryPacket, MotionExPacket} {
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

// &F123IngestTest{
// 			name:   "lap times data",
// 			config: config,
// 			expectations: func(t *testing.T, conn net.PacketConn, packets map[PacketType][][]byte) {
// 				// t.Parallel()
// 				for _, p := range packets[SessionHistoryPacket] {
// 					n, err := conn.WriteTo(p, conn.LocalAddr())

// 					assert.NoError(t, err)
// 					assert.Equal(t, len(p), n)
// 				}
// 				// this is kind of a weird test, but the SessionHistory packet we have saved for this test has 3 laps in it,
// 				// 2 of which are completed.
// 				time.Sleep(100 * time.Millisecond)
// 				assert.Equal(t, 2, len(config.LapTimesDataChannel), "should have only read 2 laps from test data")

// 				// if we write the same packets again to simulate getting lap data that we've already received,
// 				// ingest should be keeping track of the latest lap it sent and not add any more to the channel
// 				for _, p := range packets[SessionHistoryPacket] {
// 					n, err := conn.WriteTo(p, conn.LocalAddr())

// 					assert.NoError(t, err)
// 					assert.Equal(t, len(p), n)
// 				}
// 				time.Sleep(100 * time.Millisecond)
// 				assert.Equal(t, 2, len(config.LapTimesDataChannel), "should not have queued previously-read laps")
// 			},
// 		}
