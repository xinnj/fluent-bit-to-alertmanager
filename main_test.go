package main

import (
	"os"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
)

func TestSentTestAlert(t *testing.T) {
	os.Setenv("ALERTMANAGER_URL", "http://localhost:9698/alertmanager/api/v2/")
	parseAlertManagerURL()
	initAlertManagerClient()

	oneLog := LogEntry{
		"time":      strfmt.DateTime(time.Now()).String(),
		"log":       "test",
		"namespace": "test-namespace",
		"app":       "test-app",
	}
	alerts := createAlertFromLog(oneLog)
	err := sendAlerts(alerts)
	if err != nil {
		t.Fatalf("sendAlerts failed: %v", err)
	}
}

func TestCreateAlertFromLogWithEmptyTime(t *testing.T) {
	oneLog := LogEntry{
		"time":      "",
		"log":       "test log",
		"namespace": "test-namespace",
		"app":       "test-app",
	}

	alerts := createAlertFromLog(oneLog)

	if len(alerts) == 0 {
		t.Fatal("expected at least one alert, got none")
	}

	// Verify that time field is set even when input is empty
	if alerts[0].Labels["time"] == "" {
		t.Error("expected time label to be set even when input time is empty")
	}

	// Verify that alert uses current time when input is invalid
	timeDiff := time.Now().Sub(time.Time(alerts[0].StartsAt))
	if timeDiff < -time.Second || timeDiff > time.Second {
		t.Errorf("expected alert time to be close to now, got difference of %v", timeDiff)
	}
}

func TestCreateAlertFromLogWithInvalidTime(t *testing.T) {
	oneLog := LogEntry{
		"time":      "invalid-time-format",
		"log":       "test log",
		"namespace": "test-namespace",
		"app":       "test-app",
	}

	alerts := createAlertFromLog(oneLog)

	if len(alerts) == 0 {
		t.Fatal("expected at least one alert, got none")
	}

	// Verify that alert is created with fallback time
	if alerts[0].StartsAt.IsZero() {
		t.Error("expected start time to be set with fallback value")
	}
}

func TestCreateAlertFromLogWithMissingFields(t *testing.T) {
	tests := []struct {
		name     string
		logEntry LogEntry
		wantErr  bool
	}{
		{
			name: "missing time",
			logEntry: LogEntry{
				"log":       "test log",
				"namespace": "test-namespace",
			},
			wantErr: false,
		},
		{
			name: "missing log",
			logEntry: LogEntry{
				"time":      strfmt.DateTime(time.Now()).String(),
				"namespace": "test-namespace",
				"app":       "test-app",
			},
			wantErr: false,
		},
		{
			name: "missing namespace",
			logEntry: LogEntry{
				"time": strfmt.DateTime(time.Now()).String(),
				"log":  "test log",
				"app":  "test-app",
			},
			wantErr: false,
		},
		{
			name:     "all fields missing",
			logEntry: LogEntry{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alerts := createAlertFromLog(tt.logEntry)

			if len(alerts) == 0 {
				t.Fatal("expected at least one alert to be created")
			}

			// Verify alert is created even with missing fields
			if alerts[0].Labels == nil {
				t.Error("expected labels map to be initialized")
			}
		})
	}
}

func TestCreateAlertFromLogWithNonStringFields(t *testing.T) {
	oneLog := LogEntry{
		"time":      strfmt.DateTime(time.Now()).String(),
		"log":       12345,         // non-string value
		"namespace": true,          // boolean value
		"app":       []string{"a"}, // array value
	}

	alerts := createAlertFromLog(oneLog)

	if len(alerts) == 0 {
		t.Fatal("expected at least one alert, got none")
	}

	// Verify that non-string fields are handled gracefully
	// They should be empty strings instead of causing panic
	if alerts[0].Labels["log"] != "" {
		t.Errorf("expected log label to be empty string for non-string input, got: %v", alerts[0].Labels["log"])
	}
}

func TestCreateAlertFromLogHappyPath(t *testing.T) {
	now := strfmt.DateTime(time.Now())
	oneLog := LogEntry{
		"time":      now.String(),
		"log":       "error message",
		"namespace": "production",
		"app":       "myapp",
	}

	alerts := createAlertFromLog(oneLog)

	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]

	// Verify labels
	if alert.Labels["severity"] != "warning" {
		t.Errorf("expected severity 'warning', got '%s'", alert.Labels["severity"])
	}

	if alert.Labels["alertname"] != "ErrorLog" {
		t.Errorf("expected alertname 'ErrorLog', got '%s'", alert.Labels["alertname"])
	}

	if alert.Labels["time"] != now.String() {
		t.Errorf("expected time '%s', got '%s'", now.String(), alert.Labels["time"])
	}

	if alert.Labels["log"] != "error message" {
		t.Errorf("expected log 'error message', got '%s'", alert.Labels["log"])
	}

	if alert.Labels["namespace"] != "production" {
		t.Errorf("expected namespace 'production', got '%s'", alert.Labels["namespace"])
	}

	if alert.Labels["app"] != "myapp" {
		t.Errorf("expected app 'myapp', got '%s'", alert.Labels["app"])
	}

	// Verify annotations
	if alert.Annotations["summary"] != "ERROR in log" {
		t.Errorf("expected summary 'ERROR in log', got '%s'", alert.Annotations["summary"])
	}

	if alert.Annotations["description"] != "Log level with ERROR in log" {
		t.Errorf("expected description 'Log level with ERROR in log', got '%s'", alert.Annotations["description"])
	}
}
