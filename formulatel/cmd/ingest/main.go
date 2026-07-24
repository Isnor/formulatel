package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/isnor/formulatel/f123"
	pb "github.com/isnor/formulatel/internal/genproto"
	"github.com/isnor/formulatel/internal/mqttutil"
	"github.com/kelseyhightower/envconfig"
	"github.com/urfave/cli/v3"
)

func ingestCLI() *cli.Command {
	return &cli.Command{
		Name:        "formulatel-ingest",
		Description: "ingest and dispatch sim-racing telemetry to `formulatel`",
		Usage:       "ingest --username <user> --tenant-id <grafana-org-id> --mqtt-uri <broker-uri> --token <token>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "username",
				Aliases: []string{"user", "driver"},
			},
			&cli.StringFlag{
				Name: "token",
			},
			&cli.StringFlag{
				Name:    "verbosity",
				Aliases: []string{"loglevel", "log-level"},
				Value:   "info",
			},
			&cli.StringFlag{
				Name:    "mqtt",
				Aliases: []string{"persist-uri", "formulatel-uri", "mqtt-uri"},
			},
			&cli.IntFlag{
				Name:        "tenant",
				Aliases:     []string{"tenant-id"},
				DefaultText: "-1",
				Value:       -1,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			// read environment variables, then override with CLI flags
			var ingestConfig f123.F123IngestConfig
			envconfig.MustProcess("formulatel", &ingestConfig)
			token := ingestConfig.Token
			ingestConfig.Token = "***"
			usernameCLI := c.String("username")
			tokenCLI := c.String("token")
			tenantIDCLI := c.Int("tenant")
			logLevelStr := c.String("verbosity")
			mqttBroker := c.String("mqtt")

			if usernameCLI != "" {
				ingestConfig.Username = usernameCLI
			}
			if tokenCLI != "" {
				token = tokenCLI
			}
			if mqttBroker != "" {
				ingestConfig.MQTTBroker = mqttBroker
			}
			if tenantIDCLI > -1 {
				ingestConfig.TenantID = tenantIDCLI
			}
			if logLevelStr != "info" {
				ingestConfig.LogLevel = logLevelStr
			}

			serverContext, cancel := signal.NotifyContext(ctx, os.Interrupt, os.Kill)
			defer cancel()
			// TODO: we probably shouldn't bind to 0.0.0.0; add a flag for listen address
			conn, err := (&net.ListenConfig{}).ListenPacket(serverContext, "udp4", fmt.Sprintf("0.0.0.0:%d", ingestConfig.UDPPort))
			if err != nil {
				slog.Error("failed listening for UDP packets", "port", ingestConfig.UDPPort, "error", err.Error())
				os.Exit(1)
			}
			defer conn.Close()

			logLevel, err := parseLogLevel(ingestConfig.LogLevel)
			if err != nil {
				slog.Error("invalid log level provided", "log level", ingestConfig.LogLevel, "error", err)
				return err
			}
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: logLevel,
			})))
			if err := ingestConfig.Validate(); err != nil {
				slog.ErrorContext(serverContext, "invalid config", "error", err)
				return fmt.Errorf("invalid config: %w", err)
			}
			slog.Info("starting ingest", "config", ingestConfig)

			ingestConfig.VehicleDataChannel = make(chan *pb.GameTelemetry, 100)
			ingestConfig.MotionDataChannel = make(chan *pb.GameTelemetry, 100)
			ingestConfig.CurrentLapDataChannel = make(chan *pb.GameTelemetry, 100)
			ingestConfig.LapTimesDataChannel = make(chan *pb.GameTelemetry, 100)
			// TODO: ingest silently failed / didn't send any telemetry when I forgot to
			// add this after implementing it in the f123 package. surely there is a better
			// design pattern for the dumbassery we're trying to do here
			ingestConfig.ExtendedWheelDataChannel = make(chan *pb.GameTelemetry, 100)

			ingest := f123.NewF123Ingest(ingestConfig, conn)

			// the rest of this is setting up MQTT publishers that listen to channels of GameTelemetry
			// and publish them to an MQTT topic. If we're going to support other kinds of ingestion, e.g.
			// kafka or gRPC, then we should factor this out and configure it with the CLI flags

			mqtt.ERROR = slog.NewLogLogger(slog.NewTextHandler(os.Stderr, nil), slog.LevelError)
			mqtt.DEBUG = slog.NewLogLogger(slog.NewTextHandler(os.Stdout, nil), slog.LevelDebug)

			connectionOptions := mqttutil.GenerateMQTTv3Options().AddBroker(ingestConfig.MQTTBroker)
			connectionOptions.ClientID = fmt.Sprintf("%s_%d", ingestConfig.Username, ingestConfig.TenantID)
			connectionOptions.Username = ingestConfig.Username
			connectionOptions.Password = token

			// connect to an MQTT broker
			mqttClient, err := mqttutil.NewMQTTv3Connection(connectionOptions)
			if err != nil {
				slog.ErrorContext(serverContext, "could not connect to MQTT broker; stopping ingest", "error", err.Error())
				cancel()
				return err
			}

			var wg sync.WaitGroup
			// consume packets
			wg.Go(func() {
				if err := ingest.Listen(serverContext); err != nil {
					slog.ErrorContext(serverContext, "error reading packets. stopping.", "error", err.Error())
					cancel()
				}
			})

			// transform packets
			wg.Go(func() {
				defer func() {
					if err := recover(); err != nil {
						cancel()
						slog.Error("You've met with a terrible fate, haven't you?", "error", err)
					}
				}()
				ingest.Consume(serverContext)
			})

			// connect each telemetry channel to an mqtt topic
			for topic, channel := range map[string]chan *pb.GameTelemetry{
				prefixTopic("vehicledata", ingestConfig.Username, ingestConfig.TenantID):       ingestConfig.VehicleDataChannel,
				prefixTopic("motiondata", ingestConfig.Username, ingestConfig.TenantID):        ingestConfig.MotionDataChannel,
				prefixTopic("currentlapdata", ingestConfig.Username, ingestConfig.TenantID):    ingestConfig.CurrentLapDataChannel,
				prefixTopic("laptimesdata", ingestConfig.Username, ingestConfig.TenantID):      ingestConfig.LapTimesDataChannel,
				prefixTopic("extendedwheeldata", ingestConfig.Username, ingestConfig.TenantID): ingestConfig.ExtendedWheelDataChannel,
			} {
				wg.Go(func() {
					ctx, cancel := context.WithCancel(serverContext)
					defer cancel()
					slog.InfoContext(ctx, "starting mqtt publisher", "topic", topic)
					if err := RunMQTTv3Publisher(ctx, StartPublisherConfig{
						mqttClient: mqttClient,
						data:       channel,
						topic:      topic,
					}); err != nil {
						slog.ErrorContext(ctx, "mqtt publisher failed", "error", err.Error(), "topic", topic)
					} else {
						slog.InfoContext(ctx, "finished mqtt publisher", "topic", topic)
					}
				})
			}
			wg.Wait()
			slog.InfoContext(serverContext, "terminated ingest")
			return nil
		},
	}
}

func main() {
	cli := ingestCLI()
	if err := cli.Run(context.Background(), os.Args); err != nil {
		slog.Error("ingest had a problem", "error", err)
	}
}

// prefixTopic prepends org/user to a given topic
func prefixTopic(topic, user string, tenant int) string {
	return fmt.Sprintf("formulatel/f123/%d/%s/%s", tenant, user, topic)
}

func parseLogLevel(levelStr string) (slog.Level, error) {
	var level slog.Level
	return level, level.UnmarshalText([]byte(levelStr))
}
