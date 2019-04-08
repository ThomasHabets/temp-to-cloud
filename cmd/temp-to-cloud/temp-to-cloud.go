package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	// Third party.
	log "github.com/sirupsen/logrus"
	"periph.io/x/periph/conn/i2c/i2creg"
	"periph.io/x/periph/experimental/devices/mcp9808"
	"periph.io/x/periph/host"

	// google cloud.
	monitoring "cloud.google.com/go/monitoring/apiv3"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	respb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

var (
	csvFile   = flag.String("csv", "", "Output file to append to.")
	deviceID  = flag.String("device", "test", "")
	sensorID  = flag.String("sensor", "test", "")
	projectID = flag.String("project", "", "")
	freq = flag.Duration("duration", time.Minute, "")

	metricClient *monitoring.MetricClient
)

func toCloud(ctx context.Context,value float64, now time.Time) error {
	metric := "custom.googleapis.com/sensors/temperature"

	req := monitoringpb.CreateTimeSeriesRequest{
		Name: "projects/" + *projectID,
		TimeSeries: []*monitoringpb.TimeSeries{
			{
				Metric: &metricpb.Metric{
					Type: metric,
					Labels: map[string]string{
						"device": *deviceID,
						"sensor": *sensorID,
					},
				},
				Resource: &respb.MonitoredResource{
					Labels: map[string]string{},
					Type:   "global",
				},
				Points: []*monitoringpb.Point{
					{
						Interval: &monitoringpb.TimeInterval{
							StartTime: &tspb.Timestamp{Seconds: now.Unix()},
							EndTime:   &tspb.Timestamp{Seconds: now.Unix()},
						},
						Value: &monitoringpb.TypedValue{
							Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: value},
						},
					},
				},
			},
		},
	}
	return metricClient.CreateTimeSeries(ctx, &req)
}

func main() {
	flag.Parse()

	// Init i2c.
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}
	b, err := i2creg.Open("")
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	d, err := mcp9808.New(b, &mcp9808.DefaultOpts)
	if err != nil {
		log.Fatal(err)
	}

	// Connect to cloud.
	ctx := context.Background()
	if metricClient, err = monitoring.NewMetricClient(ctx);err != nil {
		log.Fatal(err)
	}
	defer metricClient.Close()

	// Open file.
	f, err := os.OpenFile(*csvFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Opening %q: %v", *csvFile, err)
	}

	for range time.Tick(*freq) {
		temp, err := d.SenseTemp()
		if err != nil {
			log.Fatal(err)
		}
		now := time.Now()
		t := float64(now.UnixNano()) / 1e9
		c := temp.Celsius()
		fmt.Fprintf(f, "%f,%g\n", t, c)
		if err := toCloud(ctx, c, now); err != nil {
			log.Errorf("Failed to log (%f,%g) to cloud: %v", t, c, err)
		}
	}

	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
