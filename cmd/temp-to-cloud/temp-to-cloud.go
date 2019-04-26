package main

// Copyright 2019 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	i2cID          = flag.String("i2c", "", "I²C bus to use")
	csvFile        = flag.String("csv", "", "Output file to append to.")
	deviceID       = flag.String("device", "test", "Device this runs on.") // TODO: if empty use hostname.
	sensorID       = flag.String("sensor", "test", "Sensor name. IOW: What/where this is measuring.")
	projectID      = flag.String("project", "", "GCP Project ID that will own the data.")
	doUpdateScreen = flag.Bool("update_screen", false, "Update an OLED screen.")
	location       = flag.String("location", "us-east1-a", "Location to store timeseries in.")
	freq           = flag.Duration("duration", time.Minute, "")
	stackdriver    = flag.Bool("stackdriver", true, "Send to stackdriver.")
	metricName     = flag.String("metric", "custom.googleapis.com/sensors/temperature", "Metric name.")

	metricClient *monitoring.MetricClient
)

func toCloud(ctx context.Context, value float64, now time.Time) error {
	req := monitoringpb.CreateTimeSeriesRequest{
		Name: "projects/" + *projectID,
		TimeSeries: []*monitoringpb.TimeSeries{
			{
				Metric: &metricpb.Metric{
					Type:   *metricName,
					Labels: map[string]string{},
				},
				// https://cloud.google.com/monitoring/custom-metrics
				// https://cloud.google.com/monitoring/api/resources#tag_generic_node
				Resource: &respb.MonitoredResource{
					Labels: map[string]string{
						"node_id":   *deviceID,
						"namespace": *sensorID,
						"location":  *location,
					},
					Type: "generic_node",
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
	if *projectID == "" {
		log.Fatal("-project is mandatory")
	}
	log.Infof("temp-to-cloud starting up...")

	// Init i2c.
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}
	b, err := i2creg.Open(*i2cID)
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	d, err := mcp9808.New(b, &mcp9808.DefaultOpts)
	if err != nil {
		log.Fatal(err)
	}

	if *doUpdateScreen {
		cb, err := initScreen()
		if err != nil {
			log.Fatalf("Failed to init screen: %v", err)
		}
		defer cb()
	}

	// Connect to cloud.
	ctx := context.Background()
	if metricClient, err = monitoring.NewMetricClient(ctx); err != nil {
		log.Fatal(err)
	}
	defer metricClient.Close()

	// Open file.
	f, err := os.OpenFile(*csvFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Opening %q: %v", *csvFile, err)
	}

	ticker := time.Tick(*freq)
	for {
		temp, err := d.SenseTemp()
		if err != nil {
			log.Fatal(err)
		}
		now := time.Now()
		t := float64(now.UnixNano()) / 1e9
		c := temp.Celsius()
		fmt.Fprintf(f, "%f,%g\n", t, c)
		if *stackdriver {
			log.Debugf("Logging to stackdriver...")
			if err := toCloud(ctx, c, now); err != nil {
				log.Errorf("Failed to log (%f,%g) to cloud: %v", t, c, err)
			}
			if *doUpdateScreen {
				if err := updateScreen(fmt.Sprintf("%.2f C", c)); err != nil {
					log.Errorf("Failed to update screen: %v", err)
				}
			}
		}
		<-ticker
	}

	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
