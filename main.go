package main

import (
	"encoding/json"
	"fmt"
	clientruntime "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/alertmanager/api/v2/models"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

type LogEntry map[string]any

var alertManagerHost, alertManagerBasePath, alertManagerScheme string

func parseLogs(r *http.Request) ([]LogEntry, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading body: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
		}
	}(r.Body)

	var logs []LogEntry
	err = json.Unmarshal(body, &logs)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling logs: %v", err)
	}

	return logs, nil
}

func createAlertFromLog(oneLog LogEntry) models.PostableAlerts {
	logTime, err := strfmt.ParseDateTime(oneLog["time"].(string))
	if err != nil {
		logTime = strfmt.DateTime(time.Now())
	}
	postableAlert := models.PostableAlert{
		StartsAt: logTime,
		Annotations: map[string]string{
			"summary":     "ERROR in log",
			"description": "Log level with ERROR in log",
		},
		Alert: models.Alert{
			Labels: map[string]string{
				"severity":  "warning",
				"alertname": "ErrorLog",
				"time":      oneLog["time"].(string),
				"log":       oneLog["log"].(string),
				"namespace": oneLog["kubernetes"].(map[string]interface{})["namespace_name"].(string),
				"pod":       oneLog["kubernetes"].(map[string]interface{})["pod_name"].(string),
			},
		},
	}
	alerts := append(models.PostableAlerts{}, &postableAlert)

	return alerts
}

func sendAlerts(alerts models.PostableAlerts) error {
	cr := clientruntime.New(alertManagerHost, alertManagerBasePath, []string{alertManagerScheme})
	alertManagerClient := alert.New(cr, strfmt.Default)

	response, err := alertManagerClient.PostAlerts(alert.NewPostAlertsParams().WithAlerts(alerts))
	if err != nil {
		return fmt.Errorf("Error posting alerts: %v", err)
	}

	if response != nil && response.IsSuccess() {
		log.Printf("Alerts posted successfully.")
	}

	return nil
}

func receiveLog(w http.ResponseWriter, r *http.Request) {
	logs, err := parseLogs(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// As the log length is long, let's send alert one by one
	for _, oneLog := range logs {
		log.Printf("Log: %v\n", oneLog)

		alerts := createAlertFromLog(oneLog)

		err = sendAlerts(alerts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func parseAlertManagerURL() {
	alertManagerURL := os.Getenv("ALERTMANAGER_URL")
	u, err := url.Parse(alertManagerURL)
	if err != nil {
		log.Fatal("Can't parse ALERTMANAGER_URL: ", err)
	}

	alertManagerHost = u.Host
	alertManagerBasePath = u.Path
	alertManagerScheme = u.Scheme
}

func sentTestAlert() {
	os.Setenv("ALERTMANAGER_URL", "http://localhost:9698/alertmanager/api/v2/")
	parseAlertManagerURL()

	oneLog := LogEntry{
		"time": strfmt.DateTime(time.Now()).String(),
		"log":  "test",
		"kubernetes": map[string]any{
			"namespace_name": "test-namespace",
			"pod_name":       "test-pod",
		},
	}
	alerts := createAlertFromLog(oneLog)
	err := sendAlerts(alerts)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func main() {
	//sentTestAlert()

	parseAlertManagerURL()
	http.HandleFunc("/", receiveLog)
	http.HandleFunc("/health", healthCheck)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	} else {
		log.Print("Listening on port 8080")
	}
}
