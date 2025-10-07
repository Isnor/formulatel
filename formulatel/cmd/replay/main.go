package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

// we have a directory of captured packets that we'd like to replay, i.e.
// read in files and write them to a UDP port. it'd be nice to be able to:
// 	read from a configurable directory;
// 	write to a configurable port;
// 	with a configurable delay

// flags
var (
	readDirectory = flag.String("read-dir", "./captured_packets", "directory of packet files to read")
	writePort     = flag.Int("write-port", 27543, "port on which to write packet data")
	writeDelay    = flag.Duration("write-delay", 100*time.Millisecond, "delay between writing each packet")
)

type PacketReplayer struct {
	Delay time.Duration
	Port  int
}

func (p *PacketReplayer) ReplayPackets(ctx context.Context, packets [][]byte) error {
	if len(packets) == 0 {
		return errors.New("no packets")
	}
	conn, err := net.Dial("udp", fmt.Sprintf("0.0.0.0:%d", p.Port))
	if err != nil {
		slog.ErrorContext(ctx, "couldn't open udp port")
		return err
	}
	defer conn.Close()
	slog.Debug("replaying packets...")
	ticker := time.NewTicker(p.Delay)
	for {
		select {
		case <-ctx.Done():
			slog.Debug("closing connection")
			return conn.Close()
		default:
			// write each packet to the connection until we're done, adding a delay in between
			for _, packet := range packets {
				conn.Write(packet)
				select {
				case <-ctx.Done():
					return conn.Close()
				case <-ticker.C:
				}

			}
		}
	}
}

func main() {
	flag.Parse()
	slog.SetLogLoggerLevel(slog.LevelDebug)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	var packets [][]byte
	if err := filepath.WalkDir(*readDirectory, func(path string, d fs.DirEntry, err error) error {
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
	}); err != nil {
		cancel()
		os.Exit(1)
	}

	replayer := &PacketReplayer{
		Delay: *writeDelay,
		Port:  *writePort,
	}
	if err := replayer.ReplayPackets(ctx, packets); err != nil {
		slog.Error("we have failed", "error", err.Error())
		cancel()
		os.Exit(1)
	}

	slog.Info("replay finished")
}
